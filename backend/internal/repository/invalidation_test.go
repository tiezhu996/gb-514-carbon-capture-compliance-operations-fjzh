package repository

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

var finalizeDBCounter uint64

// TestFinalizeRollbackAllowsOnlyOneWinner deterministically simulates two
// concurrent final reviews against the same open rollback using the same
// optimistic-lock version: exactly one commits, the loser gets
// ErrVersionConflict and nothing is written by the losing attempt.
func TestFinalizeRollbackAllowsOnlyOneWinner(t *testing.T) {
	dsn := "file:finalize-winner-" + time.Now().Format("150405.000000") +
		"-" + strconv.FormatUint(atomic.AddUint64(&finalizeDBCounter, 1), 10) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(
		&model.EmissionSample{}, &model.ComplianceDecision{},
		&model.DecisionRevision{}, &model.DecisionRollback{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	replacement := model.EmissionSample{
		BaseModel: model.BaseModel{Code: "ES-WIN", Name: "replacement", Status: "verified", Version: 1},
		Facility:  "装置A", EffectiveAt: now.Add(time.Hour),
	}
	if err := db.Create(&replacement).Error; err != nil {
		t.Fatalf("create replacement sample: %v", err)
	}
	decision := model.ComplianceDecision{
		BaseModel: model.BaseModel{Code: "CD-WIN", Name: "decision", Status: "review", Version: 3},
		Facility:  "装置A",
	}
	if err := db.Omit("Revisions", "Rollbacks").Create(&decision).Error; err != nil {
		t.Fatalf("create decision: %v", err)
	}
	for version := uint(1); version <= 3; version++ {
		state := "draft"
		if version >= 2 {
			state = "review"
		}
		if err := db.Create(&model.DecisionRevision{
			ComplianceDecisionID: decision.ID, Version: version, State: state,
			Evidence: "evidence", Reason: "history", Actor: "operator", RequestID: "req", CreatedAt: now,
		}).Error; err != nil {
			t.Fatalf("seed revision %d: %v", version, err)
		}
	}
	if err := db.Create(&model.DecisionRollback{
		ComplianceDecisionID: decision.ID, ChainOrder: 1, FromState: "accepted",
		InvalidatedSampleID: 999, InvalidatedSampleCode: "ES-OLD",
		InvalidationReason: "calibration", InvalidatedAt: now.Add(-time.Hour),
		RolledBackAt: now, CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create open rollback: %v", err)
	}

	repo := NewComplianceDecisionRepository(db)
	winner := model.ComplianceDecision{
		BaseModel: model.BaseModel{Status: "accepted", Version: 4, UpdatedAt: now},
	}
	winnerRevision := &model.DecisionRevision{
		Version: 4, State: "accepted", Evidence: "evidence",
		Reason: "finalized", Actor: "reviewer", RequestID: "req-winner", CreatedAt: now,
	}
	if err := repo.FinalizeRollback(ctx, decision.ID, 3, &winner, winnerRevision, replacement.ID, "reviewer", "finalized", now); err != nil {
		t.Fatalf("winning finalize must succeed: %v", err)
	}

	loser := model.ComplianceDecision{
		BaseModel: model.BaseModel{Status: "escalated", Version: 4, UpdatedAt: now},
	}
	loserRevision := &model.DecisionRevision{
		Version: 4, State: "escalated", Evidence: "evidence",
		Reason: "concurrent", Actor: "reviewer", RequestID: "req-loser", CreatedAt: now,
	}
	err = repo.FinalizeRollback(ctx, decision.ID, 3, &loser, loserRevision, replacement.ID, "reviewer", "concurrent", now)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("losing finalize must return version conflict, got %v", err)
	}

	var stored model.ComplianceDecision
	if err := db.First(&stored, decision.ID).Error; err != nil {
		t.Fatalf("reload decision: %v", err)
	}
	if stored.Status != "accepted" || stored.Version != 4 {
		t.Fatalf("winner result was not preserved: status=%s version=%d", stored.Status, stored.Version)
	}
	var revisionCount int64
	if err := db.Model(&model.DecisionRevision{}).Where("compliance_decision_id = ?", decision.ID).Count(&revisionCount).Error; err != nil {
		t.Fatalf("count revisions: %v", err)
	}
	if revisionCount != 4 {
		t.Fatalf("loser must not append a revision, got %d", revisionCount)
	}
	var rollback model.DecisionRollback
	if err := db.Where("compliance_decision_id = ?", decision.ID).First(&rollback).Error; err != nil {
		t.Fatalf("load rollback: %v", err)
	}
	if rollback.ResolvedAt == nil || rollback.ResolvedBy != "reviewer" ||
		rollback.ReplacementSampleID == nil || *rollback.ReplacementSampleID != replacement.ID {
		t.Fatalf("rollback must record the single winner only: %+v", rollback)
	}
}
