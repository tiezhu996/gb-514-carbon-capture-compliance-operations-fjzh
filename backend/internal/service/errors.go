package service

import (
	"errors"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/repository"
)

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrReviewerRequired  = errors.New("reviewer or admin role is required for this decision")
	ErrDecisionLocked    = errors.New("compliance decision fields are locked after review begins")

	// Rollback-flow errors are produced by the review repository transactions.
	// They are re-exported here so handlers depend on a single service surface.
	ErrSampleAlreadyVoided = repository.ErrSampleAlreadyVoided
	ErrRollbackFinalized   = repository.ErrRollbackFinalized
	ErrSubstituteInvalid   = repository.ErrSubstituteInvalid

	// ErrVoidViaTransition forces invalidation through the dedicated void flow
	// so rollback handling and the void audit trail cannot be bypassed.
	ErrVoidViaTransition = errors.New("void an emission sample through the dedicated invalidation endpoint")
	// ErrRollbackNotFound means the decision has no open rollback chain.
	ErrRollbackNotFound = errors.New("no open rollback review exists for this compliance decision")
	// ErrRollbackReviewOnly stops generic transitions touching the rollback
	// review state, which is governed solely by the void/finalize flow.
	ErrRollbackReviewOnly = errors.New("rollback review must be resolved with a verified substitute sample by a reviewer")
)
