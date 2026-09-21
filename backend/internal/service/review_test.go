package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/config"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/constants"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/dto"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/model"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/repository"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newReviewFixture(t *testing.T, dsn string) (*gorm.DB, EmissionSampleService, ComplianceDecisionService, ReviewService) {
	t.Helper()
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
	security := NewSecurityService(repository.NewSecurityRepository(db), config.Config{})
	reviewRepo := repository.NewReviewRepository(db)
	sampleService := NewEmissionSampleService(repository.NewEmissionSampleRepository(db), reviewRepo, security)
	decisionService := NewComplianceDecisionService(repository.NewComplianceDecisionRepository(db), security)
	reviewService := NewReviewService(reviewRepo)
	return db, sampleService, decisionService, reviewService
}

func createVerifiedSample(t *testing.T, ctx context.Context, svc EmissionSampleService, code, unit string, sampledAt time.Time) model.EmissionSample {
	t.Helper()
	sample, err := svc.Create(ctx, dto.CreateEmissionSample{
		Code: code, Name: "Stack sample " + code, Facility: "Train A", Owner: "operator",
		Category: "emissions", RiskLevel: "high", MetricValue: 40, MetricUnit: "ppm",
		EffectiveAt: sampledAt, Evidence: "lab report", UnitCode: unit,
	}, "operator", "req-"+code)
	if err != nil {
		t.Fatalf("create sample %s: %v", code, err)
	}
	for _, target := range []string{"testing", "verified"} {
		sample, err = svc.Transition(ctx, sample.ID, dto.TransitionRequest{
			Status: target, ExpectedVersion: sample.Version, Reason: "advance sample for " + target,
		}, "operator", "req-"+code+"-"+target)
		if err != nil {
			t.Fatalf("transition sample %s to %s: %v", code, target, err)
		}
	}
	return sample
}

func createAcceptedDecision(t *testing.T, ctx context.Context, svc ComplianceDecisionService, code, unit, sampleCode string, effectiveAt time.Time) model.ComplianceDecision {
	t.Helper()
	decision, err := svc.Create(ctx, dto.CreateComplianceDecision{
		Code: code, Name: "Decision " + code, Facility: "Train A", Owner: "operator",
		Category: "emissions", RiskLevel: "high", MetricValue: 40, MetricUnit: "ppm",
		EffectiveAt: effectiveAt, Evidence: "cites " + sampleCode, UnitCode: unit, SampleCode: sampleCode,
	}, "operator", "req-"+code+"-create")
	if err != nil {
		t.Fatalf("create decision: %v", err)
	}
	decision, err = svc.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "review", ExpectedVersion: decision.Version, Reason: "ready for review",
	}, "operator", model.RoleOperator, "req-"+code+"-review")
	if err != nil {
		t.Fatalf("submit review: %v", err)
	}
	decision, err = svc.Transition(ctx, decision.ID, dto.TransitionRequest{
		Status: "accepted", ExpectedVersion: decision.Version, Reason: "accept based on " + sampleCode,
	}, "reviewer", model.RoleReviewer, "req-"+code+"-accept")
	if err != nil {
		t.Fatalf("accept decision: %v", err)
	}
	return decision
}

func TestSampleVoidRollsBackFinalDecisionAndReviewerFinalizesWithSubstitute(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:rollback-%d?mode=memory&cache=shared", time.Now().UnixNano())
	_, samples, decisions, reviews := newReviewFixture(t, dsn)

	originalTime := time.Now().UTC().Add(-2 * time.Hour)
	sample := createVerifiedSample(t, ctx, samples, "ES-ORIG", "CU-A", originalTime)
	decision := createAcceptedDecision(t, ctx, decisions, "CD-RB", "CU-A", "ES-ORIG", originalTime)
	originalRevisions := len(decision.Revisions)

	// Void the cited sample: the accepted decision must roll back to
	// review_required, preserving prior conclusion/evidence and the reason.
	voided, err := samples.Void(ctx, sample.ID, dto.VoidSampleRequest{Reason: "sampler calibration failed"}, "operator", "req-void")
	if err != nil {
		t.Fatalf("void sample: %v", err)
	}
	if voided.Status != "invalid" || voided.VoidedAt == nil || voided.VoidedReason != "sampler calibration failed" {
		t.Fatalf("sample void metadata not frozen: %+v", voided)
	}
	if len(voided.AffectedDecisions) != 1 || voided.AffectedDecisions[0].DecisionCode != "CD-RB" ||
		voided.AffectedDecisions[0].PreviousState != "accepted" {
		t.Fatalf("affected decisions projection wrong: %+v", voided.AffectedDecisions)
	}

	rolledBack, err := reviews.GetRollback(ctx, decision.ID)
	if err != nil {
		t.Fatalf("load rollback: %v", err)
	}
	if rolledBack.Status != string(constants.DecisionStateReviewRequired) {
		t.Fatalf("expected review_required, got %s", rolledBack.Status)
	}
	if rolledBack.Rollback == nil || rolledBack.Rollback.PreviousState != "accepted" ||
		rolledBack.Rollback.VoidedSampleCode != "ES-ORIG" || rolledBack.Rollback.VoidReason != "sampler calibration failed" {
		t.Fatalf("rollback chain did not preserve original conclusion and void reason: %+v", rolledBack.Rollback)
	}
	if len(rolledBack.Revisions) != originalRevisions+1 {
		t.Fatalf("rollback revision not appended: %d revisions", len(rolledBack.Revisions))
	}
	if rolledBack.Revisions[0].Evidence == "" || rolledBack.Revisions[0].State != "draft" {
		t.Fatalf("original conclusion/evidence must be retained: %+v", rolledBack.Revisions)
	}

	// Operator cannot finalize.
	_, err = reviews.Finalize(ctx, decision.ID, dto.FinalizeRollbackRequest{
		SubstituteSampleID: sample.ID, Reason: "operator tries to finalize",
	}, "operator", model.RoleOperator, "req-op-finalize")
	if !errors.Is(err, ErrReviewerRequired) {
		t.Fatalf("operator must not finalize, got %v", err)
	}

	// A substitute from a different unit is rejected.
	other := createVerifiedSample(t, ctx, samples, "ES-OTHER", "CU-B", time.Now().UTC())
	_, err = reviews.Finalize(ctx, decision.ID, dto.FinalizeRollbackRequest{
		SubstituteSampleID: other.ID, Reason: "wrong unit substitute",
	}, "reviewer", model.RoleReviewer, "req-wrong-unit")
	if !errors.Is(err, ErrSubstituteInvalid) {
		t.Fatalf("different-unit substitute must be rejected, got %v", err)
	}

	// A same-unit sample sampled BEFORE the void time is rejected.
	tooEarly := createVerifiedSample(t, ctx, samples, "ES-EARLY", "CU-A", originalTime.Add(-time.Hour))
	_, err = reviews.Finalize(ctx, decision.ID, dto.FinalizeRollbackRequest{
		SubstituteSampleID: tooEarly.ID, Reason: "sample predates void",
	}, "reviewer", model.RoleReviewer, "req-early")
	if !errors.Is(err, ErrSubstituteInvalid) {
		t.Fatalf("pre-void substitute must be rejected, got %v", err)
	}

	// A verified, same-unit substitute sampled after the void time is accepted.
	substitute := createVerifiedSample(t, ctx, samples, "ES-SUB", "CU-A", voided.VoidedAt.Add(time.Minute))
	finalized, err := reviews.Finalize(ctx, decision.ID, dto.FinalizeRollbackRequest{
		SubstituteSampleID: substitute.ID, Reason: "substitute confirms compliance",
	}, "reviewer", model.RoleReviewer, "req-finalize")
	if err != nil {
		t.Fatalf("reviewer finalize: %v", err)
	}
	if finalized.Status != "accepted" || finalized.SampleCode != "ES-SUB" {
		t.Fatalf("decision should be restored to accepted with substitute, got %s / %s", finalized.Status, finalized.SampleCode)
	}
	if finalized.Rollback == nil || finalized.Rollback.SubstituteCode != "ES-SUB" || finalized.Rollback.FinalizedAt == nil ||
		finalized.Rollback.FinalizedBy != "reviewer" {
		t.Fatalf("finalized rollback chain incomplete: %+v", finalized.Rollback)
	}
	if len(finalized.Revisions) != originalRevisions+2 || finalized.Revisions[len(finalized.Revisions)-1].Actor != "reviewer" {
		t.Fatalf("final review revision missing: %+v", finalized.Revisions)
	}

	// Replacing the substitute after final review is rejected.
	alt := createVerifiedSample(t, ctx, samples, "ES-ALT", "CU-A", voided.VoidedAt.Add(2*time.Minute))
	_, err = reviews.Finalize(ctx, decision.ID, dto.FinalizeRollbackRequest{
		SubstituteSampleID: alt.ID, Reason: "try to swap substitute",
	}, "reviewer", model.RoleReviewer, "req-swap")
	if !errors.Is(err, ErrRollbackFinalized) {
		t.Fatalf("substitute replacement must be rejected, got %v", err)
	}
}

func TestRepeatedVoidHasSingleOutcomeAndLeavesRecordsUntouched(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:void-once-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, samples, decisions, _ := newReviewFixture(t, dsn)

	sample := createVerifiedSample(t, ctx, samples, "ES-VO", "CU-A", time.Now().UTC().Add(-time.Hour))
	decision := createAcceptedDecision(t, ctx, decisions, "CD-VO", "CU-A", "ES-VO", time.Now().UTC().Add(-time.Hour))

	if _, err := samples.Void(ctx, sample.ID, dto.VoidSampleRequest{Reason: "first void"}, "operator", "req-void-1"); err != nil {
		t.Fatalf("first void: %v", err)
	}

	var versionAfterFirst uint
	var auditsAfterFirst int64
	if err := db.Model(&model.ComplianceDecision{}).Select("version").Where("id = ?", decision.ID).Scan(&versionAfterFirst).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.AuditLog{}).Count(&auditsAfterFirst).Error; err != nil {
		t.Fatal(err)
	}

	// Repeated void is rejected and changes neither the decision nor audit log.
	_, err := samples.Void(ctx, sample.ID, dto.VoidSampleRequest{Reason: "second void"}, "operator", "req-void-2")
	if !errors.Is(err, ErrSampleAlreadyVoided) {
		t.Fatalf("repeated void must be rejected, got %v", err)
	}
	var versionNow uint
	var auditsNow int64
	if err := db.Model(&model.ComplianceDecision{}).Select("version").Where("id = ?", decision.ID).Scan(&versionNow).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.AuditLog{}).Count(&auditsNow).Error; err != nil {
		t.Fatal(err)
	}
	if versionNow != versionAfterFirst || auditsNow != auditsAfterFirst {
		t.Fatalf("failed second void mutated records: version %d->%d, audits %d->%d",
			versionAfterFirst, versionNow, auditsAfterFirst, auditsNow)
	}

	// A generic transition to invalid is also blocked.
	_, err = samples.Transition(ctx, sample.ID, dto.TransitionRequest{
		Status: "invalid", ExpectedVersion: sample.Version + 1, Reason: "bypass void endpoint",
	}, "operator", "req-bypass")
	if !errors.Is(err, ErrVoidViaTransition) {
		t.Fatalf("generic invalid transition must be blocked, got %v", err)
	}
}

func TestConcurrentFinalizeProducesSingleWinner(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("file:concurrent-%d?mode=memory&cache=shared", time.Now().UnixNano())
	_, samples, decisions, reviews := newReviewFixture(t, dsn)

	sample := createVerifiedSample(t, ctx, samples, "ES-CC", "CU-A", time.Now().UTC().Add(-time.Hour))
	decision := createAcceptedDecision(t, ctx, decisions, "CD-CC", "CU-A", "ES-CC", time.Now().UTC().Add(-time.Hour))
	voided, err := samples.Void(ctx, sample.ID, dto.VoidSampleRequest{Reason: "void before concurrent finalize"}, "operator", "req-void-cc")
	if err != nil {
		t.Fatalf("void: %v", err)
	}
	subA := createVerifiedSample(t, ctx, samples, "ES-CCA", "CU-A", voided.VoidedAt.Add(time.Minute))
	subB := createVerifiedSample(t, ctx, samples, "ES-CCB", "CU-A", voided.VoidedAt.Add(2*time.Minute))

	const workers = 8
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sub := subA.ID
			if i%2 == 0 {
				sub = subB.ID
			}
			_, ferr := reviews.Finalize(ctx, decision.ID, dto.FinalizeRollbackRequest{
				SubstituteSampleID: sub, Reason: "concurrent final review",
			}, "reviewer", model.RoleReviewer, fmt.Sprintf("req-finalize-%d", i))
			results <- ferr
		}(i)
	}
	wg.Wait()
	close(results)

	winners, losers := 0, 0
	for ferr := range results {
		switch {
		case ferr == nil:
			winners++
		case errors.Is(ferr, ErrRollbackFinalized):
			losers++
		default:
			t.Fatalf("unexpected concurrent result: %v", ferr)
		}
	}
	if winners != 1 || losers != workers-1 {
		t.Fatalf("expected exactly one winner, got %d winners and %d losers", winners, losers)
	}

	finalized, err := reviews.GetRollback(ctx, decision.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if finalized.Status != "accepted" || finalized.Rollback == nil || finalized.Rollback.FinalizedAt == nil {
		t.Fatalf("decision not finalized after single winner: %+v", finalized)
	}
}
