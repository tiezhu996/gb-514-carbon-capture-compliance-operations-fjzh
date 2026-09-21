package service

import "errors"

var (
	ErrInvalidTransition = errors.New("requested status transition is not allowed")
	ErrInvalidInput      = errors.New("business input validation failed")
	ErrUnauthorized      = errors.New("invalid username or password")
	ErrInactiveUser      = errors.New("user account is inactive")
	ErrReviewerRequired  = errors.New("reviewer or admin role is required for this decision")
	ErrDecisionLocked    = errors.New("compliance decision fields are locked after review begins")

	// ErrSampleAlreadyInvalidated guards repeated invalidation: the request
	// fails without touching the sample, decisions or the audit trail.
	ErrSampleAlreadyInvalidated = errors.New("emission sample is already invalidated")
	// ErrSampleReferenceInvalid guards decision bindings to a missing sample
	// or one taken at a different 装置 (facility).
	ErrSampleReferenceInvalid = errors.New("referenced emission sample does not exist or belongs to another unit")
	// ErrReplacementRequired means a rolled-back decision cannot be finalised
	// without binding a replacement sample.
	ErrReplacementRequired = errors.New("a verified replacement sample is required to finalise the rolled-back decision")
	// ErrReplacementNotVerified means the replacement sample is not verified.
	ErrReplacementNotVerified = errors.New("replacement sample must be verified before final review")
	// ErrReplacementFacilityMismatch means replacement and decision refer to
	// different 装置.
	ErrReplacementFacilityMismatch = errors.New("replacement sample must come from the same unit as the decision")
	// ErrReplacementSampledBeforeInvalidation enforces that the replacement
	// was sampled after the original sample was invalidated.
	ErrReplacementSampledBeforeInvalidation = errors.New("replacement sample must be sampled after the invalidation time")
	// ErrReplacementNotAllowed rejects replacement bindings on decisions that
	// were never rolled back.
	ErrReplacementNotAllowed = errors.New("decision has no open rollback that accepts a replacement sample")
	// ErrReplacementFinalOnly rejects a replacement supplied on a non-final
	// transition while a rollback is open.
	ErrReplacementFinalOnly = errors.New("replacement sample can only be bound on final acceptance or escalation")
	// ErrReplacementLocked rejects swapping the replacement after the
	// rollback was resolved; the first final review is the single outcome.
	ErrReplacementLocked = errors.New("replacement sample is already bound and cannot be changed")
)
