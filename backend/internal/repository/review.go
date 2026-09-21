package repository

import (
	"context"
	"time"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/constants"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/model"
	"gorm.io/gorm"
)

// VoidResult reports what the atomic invalidation changed. Affected holds the
// final decisions that were rolled back because they cited the voided sample.
type VoidResult struct {
	Sample   model.EmissionSample
	Affected []model.ComplianceDecision
}

// ReviewRepository owns the 作废复核 bounded context: invalidating a sample
// together with rolling back the decisions that cite it, and completing a
// rollback with a substitute sample. Every method runs one database
// transaction so concurrent requests converge on exactly one outcome and a
// failure leaves records, revisions and audit logs untouched.
type ReviewRepository interface {
	// VoidSample marks the sample invalid and rolls back every
	// accepted/escalated decision referencing it, atomically.
	VoidSample(ctx context.Context, sampleID uint, reason, actor, requestID string) (VoidResult, error)
	// FinalizeRollback completes the rollback for one decision using the
	// substitute sample. The caller passes the validated substitute.
	FinalizeRollback(ctx context.Context, decisionID, substituteID uint, finalState, reason, actor, requestID string) (model.ComplianceDecision, error)
	// OpenRollback returns the rollback chain for a decision.
	OpenRollback(ctx context.Context, decisionID uint) (model.DecisionRollback, error)
	// AffectedDecisions returns the rollback projection shown on a sample page.
	AffectedDecisions(ctx context.Context, sampleID uint) ([]model.AffectedDecision, error)
	// LoadDecisionForRollback reads the decision with its rollback linkage.
	LoadDecisionForRollback(ctx context.Context, decisionID uint) (model.ComplianceDecision, error)
}

type reviewRepository struct{ db *gorm.DB }

func NewReviewRepository(db *gorm.DB) ReviewRepository {
	return &reviewRepository{db: db}
}

func (r *reviewRepository) VoidSample(ctx context.Context, sampleID uint, reason, actor, requestID string) (VoidResult, error) {
	result := VoidResult{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sample model.EmissionSample
		if err := tx.First(&sample, sampleID).Error; err != nil {
			return err
		}
		if sample.Status == "invalid" || sample.VoidedAt != nil {
			return ErrSampleAlreadyVoided
		}

		voidedAt := time.Now().UTC()
		beforeVersion := sample.Version
		update := tx.Model(&model.EmissionSample{}).
			Where("id = ? AND status <> ? AND version = ? AND voided_at IS NULL", sampleID, "invalid", beforeVersion).
			Updates(map[string]any{
				"status": "invalid", "version": gorm.Expr("version + 1"), "updated_at": voidedAt,
				"voided_at": voidedAt, "voided_reason": reason, "voided_by": actor,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			// A concurrent request invalidated the sample first.
			return ErrSampleAlreadyVoided
		}
		if err := tx.Create(&model.AuditLog{
			Actor: actor, RequestID: requestID, Action: "void", EntityType: "EmissionSample",
			EntityID: sampleID, BeforeState: sample.Status, AfterState: "invalid",
			Detail: "voided 排放样本: " + reason, CreatedAt: voidedAt,
		}).Error; err != nil {
			return err
		}

		var decisions []model.ComplianceDecision
		if err := tx.Where("sample_code = ? AND status IN ?", sample.Code, constants.FinalDecisionStates).
			Find(&decisions).Error; err != nil {
			return err
		}
		for i := range decisions {
			decision := decisions[i]
			previousVersion := decision.Version
			rolledAt := time.Now().UTC()
			claim := tx.Model(&model.ComplianceDecision{}).
				Where("id = ? AND version = ? AND status IN ?", decision.ID, previousVersion, constants.FinalDecisionStates).
				Updates(map[string]any{
					"status":  string(constants.DecisionStateReviewRequired),
					"version": gorm.Expr("version + 1"), "updated_at": rolledAt,
				})
			if claim.Error != nil {
				return claim.Error
			}
			if claim.RowsAffected == 0 {
				// Another transition won the decision concurrently; the void
				// stays effective but this decision is no longer final.
				continue
			}
			rollback := model.DecisionRollback{
				ComplianceDecisionID: decision.ID, VoidedSampleID: sample.ID,
				VoidedSampleCode: sample.Code, PreviousState: decision.Status,
				PreviousVersion: previousVersion, VoidReason: reason, VoidedAt: voidedAt,
				RolledBackAt: rolledAt, RolledBackBy: actor, CreatedAt: rolledAt, UpdatedAt: rolledAt,
			}
			if err := tx.Create(&rollback).Error; err != nil {
				return err
			}
			revision := model.DecisionRevision{
				ComplianceDecisionID: decision.ID, Version: previousVersion + 1,
				State: string(constants.DecisionStateReviewRequired), Evidence: decision.Evidence,
				Reason: "rolled back because cited sample " + sample.Code + " was voided: " + reason,
				Actor:  actor, RequestID: requestID, CreatedAt: rolledAt,
			}
			if err := tx.Create(&revision).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.AuditLog{
				Actor: actor, RequestID: requestID, Action: "rollback", EntityType: "ComplianceDecision",
				EntityID: decision.ID, BeforeState: decision.Status,
				AfterState: string(constants.DecisionStateReviewRequired),
				Detail:     "rolled back 合规决定; cited sample " + sample.Code + " voided: " + reason,
				CreatedAt:  rolledAt,
			}).Error; err != nil {
				return err
			}
			decisions[i].Status = string(constants.DecisionStateReviewRequired)
			decisions[i].Version = previousVersion + 1
		}

		if err := tx.First(&result.Sample, sampleID).Error; err != nil {
			return err
		}
		result.Affected = decisions
		return nil
	})
	return result, err
}

func (r *reviewRepository) FinalizeRollback(ctx context.Context, decisionID, substituteID uint, finalState, reason, actor, requestID string) (model.ComplianceDecision, error) {
	var decision model.ComplianceDecision
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rollback model.DecisionRollback
		if err := tx.Where("compliance_decision_id = ?", decisionID).First(&rollback).Error; err != nil {
			return err
		}
		if rollback.FinalizedAt != nil {
			return ErrRollbackFinalized
		}

		var current model.ComplianceDecision
		if err := tx.First(&current, decisionID).Error; err != nil {
			return err
		}
		if current.Status != string(constants.DecisionStateReviewRequired) {
			return ErrRollbackFinalized
		}

		var substitute model.EmissionSample
		if err := tx.First(&substitute, substituteID).Error; err != nil {
			return err
		}
		if substitute.Status != "verified" || substitute.VoidedAt != nil ||
			substitute.UnitCode != current.UnitCode ||
			!substitute.EffectiveAt.UTC().After(rollback.VoidedAt.UTC()) {
			return ErrSubstituteInvalid
		}

		previousVersion := current.Version
		finalizedAt := time.Now().UTC()
		claim := tx.Model(&model.ComplianceDecision{}).
			Where("id = ? AND version = ? AND status = ?", decisionID, previousVersion, string(constants.DecisionStateReviewRequired)).
			Updates(map[string]any{
				"status": finalState, "version": gorm.Expr("version + 1"), "updated_at": finalizedAt,
				"sample_code": substitute.Code,
			})
		if claim.Error != nil {
			return claim.Error
		}
		if claim.RowsAffected == 0 {
			// A concurrent final review won; only one outcome is permitted.
			return ErrRollbackFinalized
		}

		if err := tx.Model(&model.DecisionRollback{}).
			Where("id = ? AND finalized_at IS NULL", rollback.ID).
			Updates(map[string]any{
				"substitute_sample_id": substitute.ID, "substitute_code": substitute.Code,
				"final_state": finalState, "final_reason": reason, "finalized_by": actor,
				"finalized_request_id": requestID, "finalized_at": finalizedAt, "updated_at": finalizedAt,
			}).Error; err != nil {
			return err
		}
		revision := model.DecisionRevision{
			ComplianceDecisionID: decisionID, Version: previousVersion + 1, State: finalState,
			Evidence: current.Evidence,
			Reason:   "final review after sample rollback using verified substitute " + substitute.Code + ": " + reason,
			Actor:    actor, RequestID: requestID, CreatedAt: finalizedAt,
		}
		if err := tx.Create(&revision).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.AuditLog{
			Actor: actor, RequestID: requestID, Action: "finalize_rollback", EntityType: "ComplianceDecision",
			EntityID: decisionID, BeforeState: string(constants.DecisionStateReviewRequired), AfterState: finalState,
			Detail:    "final review with substitute " + substitute.Code + "; voided " + rollback.VoidedSampleCode + ": " + reason,
			CreatedAt: finalizedAt,
		}).Error; err != nil {
			return err
		}
		return tx.Preload("Revisions", func(db *gorm.DB) *gorm.DB { return db.Order("version ASC") }).
			Preload("Rollback").First(&decision, decisionID).Error
	})
	return decision, err
}

func (r *reviewRepository) OpenRollback(ctx context.Context, decisionID uint) (model.DecisionRollback, error) {
	var rollback model.DecisionRollback
	err := r.db.WithContext(ctx).Where("compliance_decision_id = ?", decisionID).First(&rollback).Error
	return rollback, err
}

func (r *reviewRepository) LoadDecisionForRollback(ctx context.Context, decisionID uint) (model.ComplianceDecision, error) {
	var decision model.ComplianceDecision
	err := r.db.WithContext(ctx).
		Preload("Revisions", func(db *gorm.DB) *gorm.DB { return db.Order("version ASC") }).
		Preload("Rollback").First(&decision, decisionID).Error
	return decision, err
}

func (r *reviewRepository) AffectedDecisions(ctx context.Context, sampleID uint) ([]model.AffectedDecision, error) {
	var sample model.EmissionSample
	if err := r.db.WithContext(ctx).First(&sample, sampleID).Error; err != nil {
		return nil, err
	}
	rows := make([]model.AffectedDecision, 0)
	err := r.db.WithContext(ctx).Table("decision_rollbacks AS rb").
		Select("d.id AS decision_id, d.code AS decision_code, d.status AS state, rb.previous_state, "+
			"rb.void_reason AS rollback_reason, rb.rolled_back_at, rb.substitute_sample_id, "+
			"rb.substitute_code, rb.finalized_at").
		Joins("JOIN compliance_decisions AS d ON d.id = rb.compliance_decision_id").
		Where("rb.voided_sample_id = ?", sampleID).
		Order("rb.rolled_back_at DESC, d.id DESC").
		Scan(&rows).Error
	return rows, err
}
