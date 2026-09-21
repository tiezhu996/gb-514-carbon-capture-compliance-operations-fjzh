package service

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/config"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/dto"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/model"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var fixtureDBCounter uint64

type rollbackFixture struct {
	db               *gorm.DB
	sampleSvc        EmissionSampleService
	decisionSvc      ComplianceDecisionService
	decisionRepo     repository.ComplianceDecisionRepository
	securityRepo     repository.SecurityRepository
	originalSampleID uint
	goodSampleID     uint
	otherUnitID      uint
	earlySampleID    uint
	decisionID       uint
}

func newRollbackFixture(t *testing.T) rollbackFixture {
	t.Helper()
	dsn := "file:rollback-flow-" + time.Now().Format("150405.000000") +
		"-" + strconv.FormatUint(atomic.AddUint64(&fixtureDBCounter, 1), 10) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.EmissionSample{}, &model.ComplianceDecision{},
		&model.DecisionRevision{}, &model.DecisionRollback{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	decisionRepo := repository.NewComplianceDecisionRepository(db)
	sampleRepo := repository.NewEmissionSampleRepository(db)
	invalidationRepo := repository.NewInvalidationRepository(db)
	securityRepo := repository.NewSecurityRepository(db)
	security := NewSecurityService(securityRepo, config.Config{})
	sampleSvc := NewEmissionSampleService(sampleRepo, decisionRepo, invalidationRepo, security)
	decisionSvc := NewComplianceDecisionService(decisionRepo, sampleSvc, security)
	ctx := context.Background()

	// Original verified sample underpinning the decision.
	original := createSample(t, ctx, sampleSvc, "ES-ORIG", "装置A", time.Now().UTC().Add(-2*time.Hour), "verified")
	// Same unit, verified, sampled after any invalidation happening "now".
	good := createSample(t, ctx, sampleSvc, "ES-GOOD", "装置A", time.Now().UTC().Add(2*time.Hour), "verified")
	// Different unit -> must be rejected.
	otherUnit := createSample(t, ctx, sampleSvc, "ES-OTHER", "装置B", time.Now().UTC().Add(2*time.Hour), "verified")
	// Same unit but sampled before invalidation -> must be rejected.
	early := createSample(t, ctx, sampleSvc, "ES-EARLY", "装置A", time.Now().UTC().Add(-24*time.Hour), "verified")

	originalID := original.ID
	decision, err := decisionSvc.Create(ctx, dto.CreateComplianceDecision{
		Code: "CD-RB", Name: "Rollback decision", Facility: "装置A", Owner: "operator",
		Category: "emissions", RiskLevel: "high", MetricValue: 42, MetricUnit: "ppm",
		EffectiveAt: time.Now().UTC(), Evidence: "evidence rests on ES-ORIG", RelatedCode: "PR-RB",
		SampleID: &originalID,
	}, "operator", "req-create")
	if err != nil {
		t.Fatalf("create decision: %v", err)
	}
	decision = transitionDecision(t, ctx, decisionSvc, decision.ID, decision.Version, "review", model.RoleOperator, "operator", "req-review")
	decision = transitionDecision(t, ctx, decisionSvc, decision.ID, decision.Version, "accepted", model.RoleReviewer, "reviewer", "req-accept")
	if decision.Status != "accepted" {
		t.Fatalf("setup: expected accepted, got %s", decision.Status)
	}

	return rollbackFixture{
		db: db, sampleSvc: sampleSvc, decisionSvc: decisionSvc, decisionRepo: decisionRepo,
		securityRepo: securityRepo, originalSampleID: original.ID, goodSampleID: good.ID,
		otherUnitID: otherUnit.ID, earlySampleID: early.ID, decisionID: decision.ID,
	}
}

func createSample(t *testing.T, ctx context.Context, svc EmissionSampleService, code, facility string, sampledAt time.Time, status string) model.EmissionSample {
	t.Helper()
	sample, err := svc.Create(ctx, dto.CreateEmissionSample{
		Code: code, Name: "样本 " + code, Facility: facility, Owner: "operator",
		Category: "emissions", RiskLevel: "high", MetricValue: 10, MetricUnit: "ppm",
		EffectiveAt: sampledAt, Evidence: "lab result",
	}, "operator", "req-sample-create-"+code)
	if err != nil {
		t.Fatalf("create sample %s: %v", code, err)
	}
	currentStatus := "collected"
	for _, next := range []string{"testing", status} {
		if currentStatus == status {
			break
		}
		sample, err = svc.Transition(ctx, sample.ID, dto.TransitionRequest{
			Status: next, ExpectedVersion: sample.Version, Reason: "advance sample state for fixture",
		}, "operator", "req-sample-state-"+code)
		if err != nil {
			t.Fatalf("transition sample %s to %s: %v", code, next, err)
		}
		currentStatus = next
	}
	return sample
}

func transitionDecision(t *testing.T, ctx context.Context, svc ComplianceDecisionService, id, version uint, status, role, actor, requestID string) model.ComplianceDecision {
	t.Helper()
	decision, err := svc.Transition(ctx, id, dto.TransitionRequest{
		Status: status, ExpectedVersion: version, Reason: "fixture transition " + status,
	}, actor, role, requestID)
	if err != nil {
		t.Fatalf("transition decision %d to %s: %v", id, status, err)
	}
	return decision
}

// TestSampleInvalidationRollsBackAcceptedDecisionAndPreservesHistory covers the
// core chain: invalidation cascades to the accepted decision, original
// conclusion/evidence/revisions survive, the rollback reason is retained and
// the decision no longer takes effect.
func TestSampleInvalidationRollsBackAcceptedDecisionAndPreservesHistory(t *testing.T) {
	fixture := newRollbackFixture(t)
	ctx := context.Background()

	original, err := fixture.sampleSvc.Get(ctx, fixture.originalSampleID)
	if err != nil {
		t.Fatalf("load original sample: %v", err)
	}
	invalidated, err := fixture.sampleSvc.Transition(ctx, original.ID, dto.TransitionRequest{
		Status: "invalid", ExpectedVersion: original.Version, Reason: "分析仪校准失效，样本作废",
	}, "operator", "req-invalidate")
	if err != nil {
		t.Fatalf("invalidate sample: %v", err)
	}
	if invalidated.Status != "invalid" || invalidated.InvalidatedReason != "分析仪校准失效，样本作废" || invalidated.InvalidatedAt == nil {
		t.Fatalf("invalidation context not retained: %+v", invalidated)
	}

	decision, err := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	if err != nil {
		t.Fatalf("load decision: %v", err)
	}
	if decision.Status != "review" {
		t.Fatalf("accepted decision must be rolled back to review, got %s", decision.Status)
	}
	if len(decision.Rollbacks) != 1 {
		t.Fatalf("expected one rollback record, got %d", len(decision.Rollbacks))
	}
	rollback := decision.Rollbacks[0]
	if rollback.FromState != "accepted" || rollback.InvalidatedSampleCode != "ES-ORIG" ||
		rollback.InvalidationReason != "分析仪校准失效，样本作废" || rollback.ResolvedAt != nil {
		t.Fatalf("rollback link content wrong: %+v", rollback)
	}
	// Original conclusion and evidence snapshots remain append-only: draft,
	// review, accepted versions are all still present before the rollback rev.
	if len(decision.Revisions) != 4 {
		t.Fatalf("expected 4 revisions after rollback (draft/review/accepted/rollback), got %d", len(decision.Revisions))
	}
	acceptedRevision := decision.Revisions[2]
	if acceptedRevision.State != "accepted" || acceptedRevision.Actor != "reviewer" || acceptedRevision.Evidence == "" {
		t.Fatalf("original accepted conclusion must be preserved: %+v", acceptedRevision)
	}
	rollbackRevision := decision.Revisions[3]
	if rollbackRevision.State != "review" || rollbackRevision.Actor != "system" {
		t.Fatalf("rollback revision context wrong: %+v", rollbackRevision)
	}
}

// TestRepeatedInvalidationChangesNothing ensures a second invalidation attempt
// fails and leaves sample, decision versions and audit count untouched.
func TestRepeatedInvalidationChangesNothing(t *testing.T) {
	fixture := newRollbackFixture(t)
	ctx := context.Background()
	original, _ := fixture.sampleSvc.Get(ctx, fixture.originalSampleID)
	if _, err := fixture.sampleSvc.Transition(ctx, original.ID, dto.TransitionRequest{
		Status: "invalid", ExpectedVersion: original.Version, Reason: "第一次作废",
	}, "operator", "req-invalidate-1"); err != nil {
		t.Fatalf("first invalidation: %v", err)
	}

	decisionAfterFirst, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	var auditCountFirst int64
	if err := fixture.db.Model(&model.AuditLog{}).Count(&auditCountFirst).Error; err != nil {
		t.Fatalf("count audits: %v", err)
	}

	_, err := fixture.sampleSvc.Transition(ctx, original.ID, dto.TransitionRequest{
		Status: "invalid", ExpectedVersion: original.Version, Reason: "重复作废",
	}, "operator", "req-invalidate-2")
	if !errors.Is(err, ErrSampleAlreadyInvalidated) {
		t.Fatalf("expected ErrSampleAlreadyInvalidated, got %v", err)
	}

	invalidSample, _ := fixture.sampleSvc.Get(ctx, fixture.originalSampleID)
	if invalidSample.InvalidatedReason != "第一次作废" {
		t.Fatalf("invalidation reason must not be overwritten, got %q", invalidSample.InvalidatedReason)
	}
	decisionAfterSecond, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	if decisionAfterSecond.Version != decisionAfterFirst.Version ||
		len(decisionAfterSecond.Revisions) != len(decisionAfterFirst.Revisions) ||
		len(decisionAfterSecond.Rollbacks) != len(decisionAfterFirst.Rollbacks) {
		t.Fatalf("repeat invalidation must not touch the decision: before=%+v after=%+v", decisionAfterFirst, decisionAfterSecond)
	}
	var auditCountSecond int64
	_ = fixture.db.Model(&model.AuditLog{}).Count(&auditCountSecond).Error
	if auditCountSecond != auditCountFirst {
		t.Fatalf("repeat invalidation must not append audit rows: %d vs %d", auditCountFirst, auditCountSecond)
	}
}

// TestOperatorCannotFinalizeRolledBackDecision keeps the reviewer boundary.
func TestOperatorCannotFinalizeRolledBackDecision(t *testing.T) {
	fixture := rollbackToReview(t)
	ctx := context.Background()
	decision, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	_, err := fixture.decisionSvc.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "accepted", ExpectedVersion: decision.Version,
		Reason: "operator attempted final review", ReplacementSampleID: fixture.goodSampleID,
	}, "operator", model.RoleOperator, "req-operator-final")
	if !errors.Is(err, ErrReviewerRequired) {
		t.Fatalf("expected ErrReviewerRequired, got %v", err)
	}
	after, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	if after.Status != "review" || len(after.Rollbacks) != 1 || after.Rollbacks[0].ResolvedAt != nil {
		t.Fatalf("failed operator final review must change nothing: %+v", after)
	}
}

// TestFinalReviewWithoutReplacementRejected enforces the replacement sample.
func TestFinalReviewWithoutReplacementRejected(t *testing.T) {
	fixture := rollbackToReview(t)
	ctx := context.Background()
	decision, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	_, err := fixture.decisionSvc.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "accepted", ExpectedVersion: decision.Version, Reason: "trying to finalize without replacement",
	}, "reviewer", model.RoleReviewer, "req-no-replacement")
	if !errors.Is(err, ErrReplacementRequired) {
		t.Fatalf("expected ErrReplacementRequired, got %v", err)
	}
}

// TestReplacementSampleRules covers verified, same-unit and sampled-after rules.
func TestReplacementSampleRules(t *testing.T) {
	fixture := rollbackToReview(t)
	ctx := context.Background()

	cases := []struct {
		name      string
		sampleID  uint
		wantError error
	}{
		{"unverified replacement", fixture.originalSampleID, ErrReplacementNotVerified},
		{"different unit replacement", fixture.otherUnitID, ErrReplacementFacilityMismatch},
		{"sampled before invalidation", fixture.earlySampleID, ErrReplacementSampledBeforeInvalidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
			_, err := fixture.decisionSvc.Transition(ctx, decision.ID, dto.TransitionRequest{
				Status: "accepted", ExpectedVersion: decision.Version,
				Reason: "attempt invalid replacement", ReplacementSampleID: tc.sampleID,
			}, "reviewer", model.RoleReviewer, "req-bad-replacement")
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("expected %v, got %v", tc.wantError, err)
			}
		})
	}
}

// TestValidReplacementFinalizesRollbackAndCannotBeChanged verifies the single
// successful outcome: chain resolved, replacement bound, later swap rejected.
func TestValidReplacementFinalizesRollbackAndCannotBeChanged(t *testing.T) {
	fixture := rollbackToReview(t)
	ctx := context.Background()
	decision, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	finalized, err := fixture.decisionSvc.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "accepted", ExpectedVersion: decision.Version,
		Reason: "replacement evidence confirms compliance", ReplacementSampleID: fixture.goodSampleID,
	}, "reviewer", model.RoleReviewer, "req-finalize")
	if err != nil {
		t.Fatalf("finalize with valid replacement: %v", err)
	}
	if finalized.Status != "accepted" {
		t.Fatalf("expected accepted, got %s", finalized.Status)
	}
	rollback := finalized.Rollbacks[0]
	if rollback.ResolvedAt == nil || rollback.ResolvedBy != "reviewer" ||
		rollback.ReplacementSampleID == nil || *rollback.ReplacementSampleID != fixture.goodSampleID {
		t.Fatalf("rollback not resolved with replacement: %+v", rollback)
	}
	if finalized.ReplacementSampleCode != "ES-GOOD" {
		t.Fatalf("expected replacement code ES-GOOD, got %q", finalized.ReplacementSampleCode)
	}

	// A later final review against a resolved rollback cannot rebind a
	// different replacement: the replacement is rejected as not allowed and
	// the single finalized result stays intact. (Two truly concurrent final
	// reviews against the same still-open rollback collapse via the
	// resolved_at IS NULL / optimistic lock, covered deterministically by
	// TestFinalizeRollbackAllowsOnlyOneWinner.)
	_, err = fixture.decisionSvc.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "escalated", ExpectedVersion: finalized.Version,
		Reason: "attempt to replace the bound sample", ReplacementSampleID: fixture.otherUnitID,
	}, "admin", model.RoleAdmin, "req-swap")
	if !errors.Is(err, ErrReplacementNotAllowed) {
		t.Fatalf("expected ErrReplacementNotAllowed after resolution, got %v", err)
	}
	again, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	if again.Status != "accepted" || *again.Rollbacks[0].ReplacementSampleID != fixture.goodSampleID {
		t.Fatalf("failed replacement swap must leave the single result intact: %+v", again)
	}
}

// TestReplacementCandidatesProjection ensures the reviewer only sees same-unit,
// verified, post-invalidation samples, and the sample page sees affected
// decisions.
func TestReplacementCandidatesAndAffectedDecisionsProjection(t *testing.T) {
	fixture := rollbackToReview(t)
	ctx := context.Background()

	candidates, err := fixture.decisionSvc.ReplacementCandidates(ctx, fixture.decisionID)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != fixture.goodSampleID {
		codes := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			codes = append(codes, candidate.Code)
		}
		t.Fatalf("only ES-GOOD should be eligible, got %v", codes)
	}

	sampleView, err := fixture.sampleSvc.Get(ctx, fixture.originalSampleID)
	if err != nil {
		t.Fatalf("load sample view: %v", err)
	}
	if len(sampleView.AffectedDecisions) != 1 || sampleView.AffectedDecisions[0].Code != "CD-RB" {
		t.Fatalf("sample page must show the affected decision, got %+v", sampleView.AffectedDecisions)
	}
	affected := sampleView.AffectedDecisions[0]
	if affected.Status != "review" || affected.RollbackReason == "" || affected.ResolvedAt != nil {
		t.Fatalf("affected decision projection wrong: %+v", affected)
	}
}

// TestInvalidatingReplacementReopensChain ensures a later invalidation of the
// replacement sample re-rolls the decision back, extending the chain while
// keeping every prior conclusion.
func TestInvalidatingReplacementReopensChain(t *testing.T) {
	fixture := rollbackToReview(t)
	ctx := context.Background()
	decision, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	if _, err := fixture.decisionSvc.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "accepted", ExpectedVersion: decision.Version,
		Reason: "finalized on replacement", ReplacementSampleID: fixture.goodSampleID,
	}, "reviewer", model.RoleReviewer, "req-finalize-1"); err != nil {
		t.Fatalf("finalize first time: %v", err)
	}
	goodSample, _ := fixture.sampleSvc.Get(ctx, fixture.goodSampleID)
	if _, err := fixture.sampleSvc.Transition(ctx, goodSample.ID, dto.TransitionRequest{
		Status: "invalid", ExpectedVersion: goodSample.Version, Reason: "替代样本同样失效",
	}, "operator", "req-invalidate-replacement"); err != nil {
		t.Fatalf("invalidate replacement: %v", err)
	}
	reopened, _ := fixture.decisionSvc.Get(ctx, fixture.decisionID)
	if reopened.Status != "review" {
		t.Fatalf("expected reopened rollback to review, got %s", reopened.Status)
	}
	if len(reopened.Rollbacks) != 2 {
		t.Fatalf("expected two chain entries, got %d", len(reopened.Rollbacks))
	}
	first, second := reopened.Rollbacks[0], reopened.Rollbacks[1]
	if first.ResolvedAt == nil || first.ChainOrder != 1 || second.ChainOrder != 2 ||
		second.InvalidatedSampleCode != "ES-GOOD" || second.ResolvedAt != nil {
		t.Fatalf("rollback chain history wrong: first=%+v second=%+v", first, second)
	}
}

func rollbackToReview(t *testing.T) rollbackFixture {
	t.Helper()
	fixture := newRollbackFixture(t)
	ctx := context.Background()
	original, _ := fixture.sampleSvc.Get(ctx, fixture.originalSampleID)
	if _, err := fixture.sampleSvc.Transition(ctx, original.ID, dto.TransitionRequest{
		Status: "invalid", ExpectedVersion: original.Version, Reason: "分析仪校准失效，样本作废",
	}, "operator", "req-invalidate"); err != nil {
		t.Fatalf("invalidate sample in helper: %v", err)
	}
	return fixture
}
