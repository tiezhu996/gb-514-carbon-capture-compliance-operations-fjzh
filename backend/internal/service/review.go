package service

import (
	"context"
	"strings"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/dto"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/model"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/repository"
)

// ReviewService governs the 作废复核 bounded context: finalizing a decision
// that was rolled back because its emission sample was voided. Only reviewers
// or admins may perform the final review, and only with a verified substitute
// from the same unit sampled after the void timestamp.
type ReviewService interface {
	GetRollback(context.Context, uint) (model.ComplianceDecision, error)
	Finalize(context.Context, uint, dto.FinalizeRollbackRequest, string, string, string) (model.ComplianceDecision, error)
}

type reviewService struct {
	review repository.ReviewRepository
}

func NewReviewService(review repository.ReviewRepository) ReviewService {
	return &reviewService{review: review}
}

func (s *reviewService) GetRollback(ctx context.Context, decisionID uint) (model.ComplianceDecision, error) {
	return s.review.LoadDecisionForRollback(ctx, decisionID)
}

func (s *reviewService) Finalize(ctx context.Context, decisionID uint, input dto.FinalizeRollbackRequest, actor, role, requestID string) (model.ComplianceDecision, error) {
	if role != model.RoleReviewer && role != model.RoleAdmin {
		return model.ComplianceDecision{}, ErrReviewerRequired
	}
	reason := strings.TrimSpace(input.Reason)
	if input.SubstituteSampleID == 0 || reason == "" {
		return model.ComplianceDecision{}, ErrInvalidInput
	}

	// Validate the rollback chain and substitute before opening the write
	// transaction so malformed requests never touch records or audit logs.
	decision, err := s.review.LoadDecisionForRollback(ctx, decisionID)
	if err != nil {
		return model.ComplianceDecision{}, err
	}
	if decision.Rollback == nil {
		return model.ComplianceDecision{}, ErrRollbackNotFound
	}
	if decision.Rollback.FinalizedAt != nil {
		return model.ComplianceDecision{}, ErrRollbackFinalized
	}
	finalState := decision.Rollback.PreviousState
	if finalState != "accepted" && finalState != "escalated" {
		finalState = "accepted"
	}

	// The repository re-validates everything inside the transaction; the
	// concurrency claim there guarantees a single winning final review.
	finalized, err := s.review.FinalizeRollback(ctx, decisionID, input.SubstituteSampleID, finalState, reason, actor, requestID)
	if err != nil {
		return model.ComplianceDecision{}, err
	}
	return finalized, nil
}
