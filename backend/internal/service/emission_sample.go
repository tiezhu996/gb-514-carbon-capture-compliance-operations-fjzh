package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/constants"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/dto"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/model"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/repository"
)

type EmissionSampleService interface {
	List(context.Context, dto.PageQuery) (repository.Page[model.EmissionSample], error)
	Get(context.Context, uint) (model.EmissionSample, error)
	Create(context.Context, dto.CreateEmissionSample, string, string) (model.EmissionSample, error)
	Update(context.Context, uint, dto.UpdateEmissionSample, string, string) (model.EmissionSample, error)
	Transition(context.Context, uint, dto.TransitionRequest, string, string) (model.EmissionSample, error)
	Delete(context.Context, uint, string, string) error
	StatusCounts(context.Context) (map[string]int64, error)
	// GetVerifiedSample loads a sample for replacement validation.
	GetVerifiedSample(context.Context, uint) (model.EmissionSample, error)
	// ListReplacementCandidates returns verified samples for one unit sampled
	// strictly after the cutoff.
	ListReplacementCandidates(context.Context, string, time.Time) ([]model.EmissionSample, error)
	// ValidateReference ensures an optional sample id exists and shares the
	// decision's facility.
	ValidateReference(context.Context, *uint, string) error
}

type emissionSampleService struct {
	repository    repository.EmissionSampleRepository
	decisionRepo  repository.ComplianceDecisionRepository
	invalidations repository.InvalidationRepository
	security      SecurityService
}

func NewEmissionSampleService(
	repo repository.EmissionSampleRepository,
	decisionRepo repository.ComplianceDecisionRepository,
	invalidations repository.InvalidationRepository,
	security SecurityService,
) EmissionSampleService {
	return &emissionSampleService{
		repository: repo, decisionRepo: decisionRepo, invalidations: invalidations, security: security,
	}
}

func (s *emissionSampleService) List(ctx context.Context, query dto.PageQuery) (repository.Page[model.EmissionSample], error) {
	return s.repository.List(ctx, query)
}

func (s *emissionSampleService) Get(ctx context.Context, id uint) (model.EmissionSample, error) {
	return s.repository.Get(ctx, id)
}

func (s *emissionSampleService) Create(ctx context.Context, input dto.CreateEmissionSample, actor, requestID string) (model.EmissionSample, error) {
	if err := validateEmissionSampleBusinessFields(input.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.EmissionSample{}, err
	}
	item := model.EmissionSample{
		BaseModel: model.BaseModel{
			Code: strings.ToUpper(strings.TrimSpace(input.Code)), Name: strings.TrimSpace(input.Name),
			Status: model.EmissionSampleInitialStatus, Version: 1, Description: strings.TrimSpace(input.Description),
		},
		Facility: strings.TrimSpace(input.Facility), Owner: strings.TrimSpace(input.Owner),
		Category: strings.TrimSpace(input.Category), RiskLevel: input.RiskLevel,
		MetricValue: input.MetricValue, MetricUnit: strings.TrimSpace(input.MetricUnit),
		EffectiveAt: input.EffectiveAt.UTC(), Evidence: strings.TrimSpace(input.Evidence),
		RelatedCode: strings.ToUpper(strings.TrimSpace(input.RelatedCode)),
	}
	if err := s.repository.Create(ctx, &item); err != nil {
		return model.EmissionSample{}, fmt.Errorf("create 排放样本: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "create", "EmissionSample", item.ID, "", item.Status, "created 排放样本")
	return item, nil
}

func (s *emissionSampleService) Update(ctx context.Context, id uint, input dto.UpdateEmissionSample, actor, requestID string) (model.EmissionSample, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.EmissionSample{}, err
	}
	if err := validateEmissionSampleBusinessFields(current.Code, input.Name, input.Facility, input.Owner); err != nil {
		return model.EmissionSample{}, err
	}
	current.Name = strings.TrimSpace(input.Name)
	current.Description = strings.TrimSpace(input.Description)
	current.Facility = strings.TrimSpace(input.Facility)
	current.Owner = strings.TrimSpace(input.Owner)
	current.Category = strings.TrimSpace(input.Category)
	current.RiskLevel = input.RiskLevel
	current.MetricValue = input.MetricValue
	current.MetricUnit = strings.TrimSpace(input.MetricUnit)
	current.EffectiveAt = input.EffectiveAt.UTC()
	current.Evidence = strings.TrimSpace(input.Evidence)
	current.RelatedCode = strings.ToUpper(strings.TrimSpace(input.RelatedCode))
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = time.Now().UTC()
	if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
		return model.EmissionSample{}, fmt.Errorf("update 排放样本: %w", err)
	}
	_ = s.security.Audit(ctx, actor, requestID, "update", "EmissionSample", id, current.Status, current.Status, "updated business fields")
	return s.repository.Get(ctx, id)
}

func (s *emissionSampleService) Transition(ctx context.Context, id uint, input dto.TransitionRequest, actor, requestID string) (model.EmissionSample, error) {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return model.EmissionSample{}, err
	}
	target := strings.TrimSpace(input.Status)
	// Repeated invalidation must be a no-op failure: no state change, no extra
	// revision cascade, no additional audit rows. Checked before the state
	// machine so a repeat reports the dedicated business error.
	if target == "invalid" && current.Status == "invalid" {
		return model.EmissionSample{}, ErrSampleAlreadyInvalidated
	}
	if !constants.CanTransition(constants.EmissionSampleTransitions, current.Status, target) {
		return model.EmissionSample{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current.Status, target)
	}
	if target != "invalid" {
		before := current.Status
		current.Status = target
		current.Version = input.ExpectedVersion + 1
		current.UpdatedAt = time.Now().UTC()
		if err := s.repository.Update(ctx, id, input.ExpectedVersion, &current); err != nil {
			return model.EmissionSample{}, fmt.Errorf("transition 排放样本: %w", err)
		}
		if err := s.security.Audit(ctx, actor, requestID, "transition", "EmissionSample", id, before, target, input.Reason); err != nil {
			return model.EmissionSample{}, fmt.Errorf("persist transition audit: %w", err)
		}
		return s.repository.Get(ctx, id)
	}
	return s.invalidate(ctx, current, input, actor, requestID)
}

// invalidate marks the sample invalid and rolls back every accepted or
// escalated decision resting on it inside a single database transaction.
func (s *emissionSampleService) invalidate(
	ctx context.Context,
	current model.EmissionSample,
	input dto.TransitionRequest,
	actor, requestID string,
) (model.EmissionSample, error) {
	now := time.Now().UTC()
	reason := strings.TrimSpace(input.Reason)

	candidates, err := s.decisionRepo.ListCascadeCandidates(ctx, current.ID)
	if err != nil {
		return model.EmissionSample{}, fmt.Errorf("load cascade decisions: %w", err)
	}

	requests := make([]repository.InvalidationRequest, 0, len(candidates))
	for _, decision := range candidates {
		// Candidates are loaded without preloaded revisions; Version already
		// tracks the latest revision number on every transition.
		previousVersions := decision.Version
		// The next chain order is derived from all existing rollback rows
		// (resolved or not), so a later invalidation of a replacement sample
		// extends the chain instead of overwriting it.
		existing, err := s.decisionRepo.CountRollbacks(ctx, decision.ID)
		if err != nil {
			return model.EmissionSample{}, fmt.Errorf("count decision rollbacks: %w", err)
		}
		chainOrder := existing + 1
		nextVersion := previousVersions + 1
		fromState := decision.Status
		decision.Status = "review"
		decision.Version = nextVersion
		decision.UpdatedAt = now
		revision := newDecisionRevision(nextVersion, "review", decision.Evidence,
			fmt.Sprintf("rolled back for review because referenced sample %s was invalidated: %s", current.Code, reason),
			"system", requestID)
		rollback := &model.DecisionRollback{
			ComplianceDecisionID:  decision.ID,
			ChainOrder:            chainOrder,
			FromState:             fromState,
			InvalidatedSampleID:   current.ID,
			InvalidatedSampleCode: current.Code,
			InvalidationReason:    reason,
			InvalidatedAt:         now,
			RolledBackAt:          now,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		audit := &model.AuditLog{
			RequestID: requestID, Actor: "system", Action: "rollback", EntityType: "ComplianceDecision",
			EntityID: decision.ID, BeforeState: fromState, AfterState: "review",
			Detail:    fmt.Sprintf("sample %s invalidated by %s: %s", current.Code, actor, reason),
			CreatedAt: now,
		}
		requests = append(requests, repository.InvalidationRequest{
			Decision: decision, FromState: fromState, ChainOrder: chainOrder,
			Revision: revision, Rollback: rollback, Audit: audit,
		})
	}

	before := current.Status
	current.Status = "invalid"
	current.InvalidatedReason = reason
	current.InvalidatedAt = &now
	current.Version = input.ExpectedVersion + 1
	current.UpdatedAt = now
	sampleAudit := &model.AuditLog{
		RequestID: requestID, Actor: actor, Action: "transition", EntityType: "EmissionSample",
		EntityID: current.ID, BeforeState: before, AfterState: "invalid", Detail: reason, CreatedAt: now,
	}
	if _, err := s.invalidations.InvalidateSampleWithCascade(ctx, &current, input.ExpectedVersion, now, requests, sampleAudit); err != nil {
		return model.EmissionSample{}, fmt.Errorf("invalidate 排放样本: %w", err)
	}
	return s.repository.Get(ctx, current.ID)
}

func (s *emissionSampleService) Delete(ctx context.Context, id uint, actor, requestID string) error {
	current, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}
	return s.security.Audit(ctx, actor, requestID, "delete", "EmissionSample", id, current.Status, "deleted", "soft deleted 排放样本")
}

func (s *emissionSampleService) StatusCounts(ctx context.Context) (map[string]int64, error) {
	return s.repository.CountByStatus(ctx)
}

func (s *emissionSampleService) GetVerifiedSample(ctx context.Context, id uint) (model.EmissionSample, error) {
	return s.repository.Get(ctx, id)
}

func (s *emissionSampleService) ListReplacementCandidates(ctx context.Context, facility string, sampledAfter time.Time) ([]model.EmissionSample, error) {
	return s.repository.ListReplacementCandidates(ctx, facility, sampledAfter)
}

func (s *emissionSampleService) ValidateReference(ctx context.Context, sampleID *uint, facility string) error {
	if sampleID == nil || *sampleID == 0 {
		return nil
	}
	sample, err := s.repository.Get(ctx, *sampleID)
	if err != nil {
		return ErrSampleReferenceInvalid
	}
	if strings.TrimSpace(sample.Facility) != strings.TrimSpace(facility) {
		return ErrSampleReferenceInvalid
	}
	return nil
}

func validateEmissionSampleBusinessFields(code, name, facility, owner string) error {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(name) == "" || strings.TrimSpace(facility) == "" || strings.TrimSpace(owner) == "" {
		return ErrInvalidInput
	}
	return nil
}
