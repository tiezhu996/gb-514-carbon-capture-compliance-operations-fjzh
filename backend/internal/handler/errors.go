package handler

import (
	"errors"
	"net/http"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/repository"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/service"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/util"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		util.Fail(c, http.StatusNotFound, "not_found", "record was not found")
	case errors.Is(err, repository.ErrVersionConflict):
		util.Fail(c, http.StatusConflict, "version_conflict", "record changed; refresh and retry")
	case errors.Is(err, service.ErrInvalidTransition), errors.Is(err, service.ErrInvalidInput),
		errors.Is(err, service.ErrReviewerRequired), errors.Is(err, service.ErrDecisionLocked),
		errors.Is(err, service.ErrSampleAlreadyInvalidated), errors.Is(err, service.ErrSampleReferenceInvalid),
		errors.Is(err, service.ErrReplacementRequired), errors.Is(err, service.ErrReplacementNotVerified),
		errors.Is(err, service.ErrReplacementFacilityMismatch),
		errors.Is(err, service.ErrReplacementSampledBeforeInvalidation),
		errors.Is(err, service.ErrReplacementNotAllowed), errors.Is(err, service.ErrReplacementFinalOnly),
		errors.Is(err, service.ErrReplacementLocked):
		util.Fail(c, http.StatusUnprocessableEntity, "business_rule", err.Error())
	default:
		_ = c.Error(err)
		util.Fail(c, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}
