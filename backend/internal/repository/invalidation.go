package repository

import (
	"context"
	"time"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/model"
	"gorm.io/gorm"
)

// RollbackResult describes one decision that was rolled back by a sample
// invalidation. It feeds the service-level audit trail.
type RollbackResult struct {
	DecisionID uint
	Code       string
	FromState  string
}

// InvalidationRequest is the fully prepared rollback payload for one decision.
type InvalidationRequest struct {
	Decision    model.ComplianceDecision
	NextVersion uint
	FromState   string
	ChainOrder  uint
	Revision    *model.DecisionRevision
	Rollback    *model.DecisionRollback
	Audit       *model.AuditLog
}

// InvalidationRepository coordinates the cross-aggregate write performed when
// an emission sample is invalidated: the sample itself plus every accepted or
// escalated decision resting on it is rolled back in one transaction.
type InvalidationRepository interface {
	InvalidateSampleWithCascade(
		ctx context.Context,
		sample *model.EmissionSample,
		expectedVersion uint,
		invalidatedAt time.Time,
		rollbacks []InvalidationRequest,
		sampleAudit *model.AuditLog,
	) ([]RollbackResult, error)
}

type invalidationRepository struct{ db *gorm.DB }

func NewInvalidationRepository(db *gorm.DB) InvalidationRepository {
	return &invalidationRepository{db: db}
}

func (r *invalidationRepository) InvalidateSampleWithCascade(
	ctx context.Context,
	sample *model.EmissionSample,
	expectedVersion uint,
	invalidatedAt time.Time,
	rollbacks []InvalidationRequest,
	sampleAudit *model.AuditLog,
) ([]RollbackResult, error) {
	results := make([]RollbackResult, 0, len(rollbacks))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Optimistic lock on the sample. Any concurrent write forces the loser
		// to retry with a fresh version, so duplicate invalidations cannot both
		// succeed.
		update := tx.Model(&model.EmissionSample{}).
			Where("id = ? AND version = ?", sample.ID, expectedVersion).
			Select("*").Omit("id", "code", "created_at", "deleted_at", "affected_decisions").
			Updates(sample)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return ErrVersionConflict
		}
		if sampleAudit != nil {
			if err := tx.Create(sampleAudit).Error; err != nil {
				return err
			}
		}
		for _, request := range rollbacks {
			decision := request.Decision
			result := tx.Model(&model.ComplianceDecision{}).
				Where("id = ? AND version = ?", decision.ID, decision.Version-1).
				Updates(map[string]any{
					"status":     decision.Status,
					"version":    decision.Version,
					"updated_at": decision.UpdatedAt,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrVersionConflict
			}
			request.Revision.ComplianceDecisionID = decision.ID
			if err := tx.Create(request.Revision).Error; err != nil {
				return err
			}
			if err := tx.Create(request.Rollback).Error; err != nil {
				return err
			}
			if request.Audit != nil {
				if err := tx.Create(request.Audit).Error; err != nil {
					return err
				}
			}
			results = append(results, RollbackResult{
				DecisionID: decision.ID, Code: decision.Code, FromState: request.FromState,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
