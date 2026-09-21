package repository

import (
	"context"
	"time"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/dto"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/model"
	"gorm.io/gorm"
)

// EmissionSampleRepository owns all persistence operations for 排放样本.
type EmissionSampleRepository interface {
	List(context.Context, dto.PageQuery) (Page[model.EmissionSample], error)
	Get(context.Context, uint) (model.EmissionSample, error)
	Create(context.Context, *model.EmissionSample) error
	Update(context.Context, uint, uint, *model.EmissionSample) error
	Delete(context.Context, uint) error
	CountByStatus(context.Context) (map[string]int64, error)
	// ListReplacementCandidates returns verified samples for one unit that
	// were sampled strictly after the cutoff (invalidation time).
	ListReplacementCandidates(ctx context.Context, facility string, sampledAfter time.Time) ([]model.EmissionSample, error)
}

type emissionSampleRepository struct {
	store *Store[model.EmissionSample]
	db    *gorm.DB
}

func NewEmissionSampleRepository(db *gorm.DB) EmissionSampleRepository {
	return &emissionSampleRepository{store: NewStore[model.EmissionSample](db), db: db}
}

func (r *emissionSampleRepository) List(ctx context.Context, q dto.PageQuery) (Page[model.EmissionSample], error) {
	page, err := r.store.List(ctx, q)
	if err != nil || len(page.Items) == 0 {
		return page, err
	}
	ids := make([]uint, 0, len(page.Items))
	for _, item := range page.Items {
		ids = append(ids, item.ID)
	}
	affected, err := r.affectedDecisions(ctx, ids)
	if err != nil {
		return Page[model.EmissionSample]{}, err
	}
	for index := range page.Items {
		page.Items[index].AffectedDecisions = affected[page.Items[index].ID]
	}
	return page, nil
}

func (r *emissionSampleRepository) Get(ctx context.Context, id uint) (model.EmissionSample, error) {
	item, err := r.store.Get(ctx, id)
	if err != nil {
		return item, err
	}
	affected, err := r.affectedDecisions(ctx, []uint{id})
	if err != nil {
		return item, err
	}
	item.AffectedDecisions = affected[id]
	return item, nil
}

func (r *emissionSampleRepository) Create(ctx context.Context, item *model.EmissionSample) error {
	return r.store.Create(ctx, item)
}

func (r *emissionSampleRepository) Update(ctx context.Context, id, version uint, item *model.EmissionSample) error {
	return r.store.Update(ctx, id, version, item)
}

func (r *emissionSampleRepository) Delete(ctx context.Context, id uint) error {
	return r.store.Delete(ctx, id)
}

func (r *emissionSampleRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	return r.store.CountByStatus(ctx)
}

func (r *emissionSampleRepository) ListReplacementCandidates(ctx context.Context, facility string, sampledAfter time.Time) ([]model.EmissionSample, error) {
	items := make([]model.EmissionSample, 0)
	err := r.db.WithContext(ctx).
		Where("facility = ? AND status = ? AND effective_at > ?", facility, "verified", sampledAfter.UTC()).
		Order("effective_at ASC").Find(&items).Error
	return items, err
}

// affectedDecisions returns, per sample id, every decision referencing the
// sample, annotated with the current rollback state of each decision. The
// latest rollback per decision is selected by maximum chain_order.
func (r *emissionSampleRepository) affectedDecisions(ctx context.Context, sampleIDs []uint) (map[uint][]model.AffectedDecision, error) {
	var rows []struct {
		SampleID            uint
		DecisionID          uint
		Code                string
		Name                string
		Status              string
		Version             uint
		RolledBackAt        *time.Time
		RollbackReason      string
		ReplacementSampleID *uint
		ResolvedAt          *time.Time
	}
	err := r.db.WithContext(ctx).Table("compliance_decisions AS d").
		Select(`emission_sample_refs.sample_id AS sample_id, d.id AS decision_id, d.code, d.name,
			d.status, d.version, rb.rolled_back_at, rb.invalidation_reason AS rollback_reason,
			rb.replacement_sample_id, rb.resolved_at`).
		Joins(`JOIN (
			SELECT id AS decision_id, sample_id FROM compliance_decisions WHERE sample_id IN ? AND deleted_at IS NULL
			UNION
			SELECT compliance_decision_id AS decision_id, replacement_sample_id AS sample_id
			FROM decision_rollbacks WHERE replacement_sample_id IS NOT NULL
		) AS emission_sample_refs ON emission_sample_refs.decision_id = d.id`, sampleIDs).
		Joins(`LEFT JOIN decision_rollbacks AS rb ON rb.compliance_decision_id = d.id
			AND rb.chain_order = (
				SELECT MAX(chain_order) FROM decision_rollbacks sub
				WHERE sub.compliance_decision_id = d.id
			)`).
		Order("d.code ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	grouped := make(map[uint][]model.AffectedDecision)
	for _, row := range rows {
		grouped[row.SampleID] = append(grouped[row.SampleID], model.AffectedDecision{
			DecisionID: row.DecisionID, Code: row.Code, Name: row.Name, Status: row.Status,
			Version: row.Version, RolledBackAt: row.RolledBackAt, RollbackReason: row.RollbackReason,
			ReplacementSampleID: row.ReplacementSampleID, ResolvedAt: row.ResolvedAt,
		})
	}
	return grouped, nil
}
