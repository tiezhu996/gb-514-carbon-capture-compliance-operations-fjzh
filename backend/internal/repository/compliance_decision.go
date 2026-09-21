package repository

import (
	"context"
	"time"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/constants"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/dto"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/model"
	"gorm.io/gorm"
)

// ComplianceDecisionRepository owns all persistence operations for 合规决定.
type ComplianceDecisionRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.ComplianceDecision], error)
	Get(context.Context, uint) (model.ComplianceDecision, error)
	CreateWithRevision(context.Context, *model.ComplianceDecision, *model.DecisionRevision) error
	UpdateWithRevision(context.Context, uint, uint, *model.ComplianceDecision, *model.DecisionRevision) error
	// FinalizeRollback performs the final transition of a rolled-back decision
	// and resolves its open rollback in one optimistic-locked transaction.
	FinalizeRollback(context.Context, uint, uint, *model.ComplianceDecision, *model.DecisionRevision, uint, string, string, time.Time) error
	OpenRollback(context.Context, uint) (*model.DecisionRollback, error)
	CountRollbacks(context.Context, uint) (uint, error)
	ListCascadeCandidates(ctx context.Context, sampleID uint) ([]model.ComplianceDecision, error)
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
}

type complianceDecisionRepository struct {
	store *Store[model.ComplianceDecision]
	db    *gorm.DB
}

func NewComplianceDecisionRepository(db *gorm.DB) ComplianceDecisionRepository {
	return &complianceDecisionRepository{store: NewStore[model.ComplianceDecision](db), db: db}
}

func (r *complianceDecisionRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.ComplianceDecision], error) {
	page, err := r.store.List(ctx, q)
	if err != nil || len(page.Items) == 0 {
		return page, err
	}
	ids := make([]uint, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	var revisions []model.DecisionRevision
	if err := r.db.WithContext(ctx).Where("compliance_decision_id IN ?", ids).
		Order("version ASC").Find(&revisions).Error; err != nil {
		return Page[model.ComplianceDecision]{}, err
	}
	byDecision := make(map[uint][]model.DecisionRevision)
	for _, revision := range revisions {
		byDecision[revision.ComplianceDecisionID] = append(byDecision[revision.ComplianceDecisionID], revision)
	}
	var rollbacks []model.DecisionRollback
	if err := r.db.WithContext(ctx).Where("compliance_decision_id IN ?", ids).
		Order("chain_order ASC").Find(&rollbacks).Error; err != nil {
		return Page[model.ComplianceDecision]{}, err
	}
	rollbackByDecision := make(map[uint][]model.DecisionRollback)
	for _, rollback := range rollbacks {
		rollbackByDecision[rollback.ComplianceDecisionID] = append(rollbackByDecision[rollback.ComplianceDecisionID], rollback)
	}
	r.fillReadModels(ctx, page.Items, byDecision, rollbackByDecision)
	return page, nil
}

func (r *complianceDecisionRepository) Get(ctx context.Context, id uint) (model.ComplianceDecision, error) {
	var item model.ComplianceDecision
	err := r.db.WithContext(ctx).Preload("Revisions", func(db *gorm.DB) *gorm.DB {
		return db.Order("version ASC")
	}).Preload("Rollbacks", func(db *gorm.DB) *gorm.DB {
		return db.Order("chain_order ASC")
	}).First(&item, id).Error
	if err != nil {
		return item, err
	}
	revisionByDecision := map[uint][]model.DecisionRevision{id: item.Revisions}
	rollbackByDecision := map[uint][]model.DecisionRollback{id: item.Rollbacks}
	items := []model.ComplianceDecision{item}
	r.fillReadModels(ctx, items, revisionByDecision, rollbackByDecision)
	return items[0], nil
}

// fillReadModels resolves sample codes and replacement codes for decisions and
// their rollback chain so the API payload carries the rollback link directly.
func (r *complianceDecisionRepository) fillReadModels(
	ctx context.Context,
	items []model.ComplianceDecision,
	revisionByDecision map[uint][]model.DecisionRevision,
	rollbackByDecision map[uint][]model.DecisionRollback,
) {
	sampleIDs := make(map[uint]struct{})
	for _, item := range items {
		if item.SampleID != nil {
			sampleIDs[*item.SampleID] = struct{}{}
		}
		for _, rollback := range rollbackByDecision[item.ID] {
			sampleIDs[rollback.InvalidatedSampleID] = struct{}{}
			if rollback.ReplacementSampleID != nil {
				sampleIDs[*rollback.ReplacementSampleID] = struct{}{}
			}
		}
	}
	sampleCodes := make(map[uint]string)
	if len(sampleIDs) > 0 {
		ids := make([]uint, 0, len(sampleIDs))
		for id := range sampleIDs {
			ids = append(ids, id)
		}
		type codeRow struct {
			ID   uint
			Code string
		}
		var rows []codeRow
		_ = r.db.WithContext(ctx).Model(&model.EmissionSample{}).
			Select("id, code").Where("id IN ?", ids).Scan(&rows).Error
		for _, row := range rows {
			sampleCodes[row.ID] = row.Code
		}
	}
	for index := range items {
		items[index].Revisions = revisionByDecision[items[index].ID]
		rollbacks := rollbackByDecision[items[index].ID]
		for rollbackIndex := range rollbacks {
			if rollbacks[rollbackIndex].ReplacementSampleID != nil {
				rollbacks[rollbackIndex].ReplacementSampleCode = sampleCodes[*rollbacks[rollbackIndex].ReplacementSampleID]
			}
		}
		items[index].Rollbacks = rollbacks
		if items[index].SampleID != nil {
			items[index].SampleCode = sampleCodes[*items[index].SampleID]
		}
		// The most recent resolved replacement, if any, is the sample the
		// decision currently rests on.
		for i := len(rollbacks) - 1; i >= 0; i-- {
			if rollbacks[i].ReplacementSampleID != nil {
				items[index].ReplacementSampleCode = rollbacks[i].ReplacementSampleCode
				break
			}
		}
	}
}

func (r *complianceDecisionRepository) CreateWithRevision(ctx context.Context, item *model.ComplianceDecision, revision *model.DecisionRevision) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Revisions", "Rollbacks").Create(item).Error; err != nil {
			return err
		}
		revision.ComplianceDecisionID = item.ID
		return tx.Create(revision).Error
	})
}

func (r *complianceDecisionRepository) UpdateWithRevision(ctx context.Context, id, version uint, item *model.ComplianceDecision, revision *model.DecisionRevision) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ComplianceDecision{}).Where("id = ? AND version = ?", id, version).
			Select("*").Omit("id", "code", "created_at", "deleted_at", "Revisions", "Rollbacks").Updates(item)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		revision.ComplianceDecisionID = id
		return tx.Create(revision).Error
	})
}

func (r *complianceDecisionRepository) FinalizeRollback(
	ctx context.Context,
	id, expectedVersion uint,
	item *model.ComplianceDecision,
	revision *model.DecisionRevision,
	replacementSampleID uint,
	resolvedBy string,
	resolutionNote string,
	resolvedAt time.Time,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.ComplianceDecision{}).Where("id = ? AND version = ?", id, expectedVersion).
			Select("*").Omit("id", "code", "created_at", "deleted_at", "Revisions", "Rollbacks").Updates(item)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrVersionConflict
		}
		revision.ComplianceDecisionID = id
		if err := tx.Create(revision).Error; err != nil {
			return err
		}
		// Resolve exactly the open rollback. The resolved_at IS NULL guard makes
		// concurrent final reviews collapse into a single successful result.
		update := tx.Model(&model.DecisionRollback{}).
			Where("compliance_decision_id = ? AND resolved_at IS NULL", id).
			Updates(map[string]any{
				"replacement_sample_id": replacementSampleID,
				"resolved_at":           resolvedAt,
				"resolved_by":           resolvedBy,
				"updated_at":            resolvedAt,
			})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return ErrVersionConflict
		}
		return nil
	})
}

// OpenRollback returns the currently unresolved rollback for a decision, or nil
// when none exists.
func (r *complianceDecisionRepository) OpenRollback(ctx context.Context, id uint) (*model.DecisionRollback, error) {
	var rollback model.DecisionRollback
	err := r.db.WithContext(ctx).
		Where("compliance_decision_id = ? AND resolved_at IS NULL", id).
		Order("chain_order DESC").First(&rollback).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rollback, nil
}

// CountRollbacks counts every rollback row (resolved or open) for a decision.
func (r *complianceDecisionRepository) CountRollbacks(ctx context.Context, id uint) (uint, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.DecisionRollback{}).
		Where("compliance_decision_id = ?", id).Count(&count).Error
	return uint(count), err
}

// ListCascadeCandidates returns the accepted/escalated decisions whose current
// evidence rests on the sample: the directly bound sample or a rollback's
// replacement sample (a resolved replacement is the current evidence basis;
// superseded replacements are already invalid and cannot be re-invalidated).
func (r *complianceDecisionRepository) ListCascadeCandidates(ctx context.Context, sampleID uint) ([]model.ComplianceDecision, error) {
	var decisions []model.ComplianceDecision
	err := r.db.WithContext(ctx).
		Where("status IN ?", []string{string(constants.DecisionStateAccepted), string(constants.DecisionStateEscalated)}).
		Where("sample_id = ? OR id IN (?)", sampleID,
			r.db.Model(&model.DecisionRollback{}).
				Select("compliance_decision_id").
				Where("replacement_sample_id = ?", sampleID),
		).Find(&decisions).Error
	return decisions, err
}

func (r *complianceDecisionRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}

func (r *complianceDecisionRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}
