package handler

import (
	"net/http"

	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/dto"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/middleware"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/service"
	"github.com/blueship581/carbon-capture-compliance-operations/backend/internal/util"
	"github.com/gin-gonic/gin"
)

// ReviewHandler exposes the 作废复核 flow: sample invalidation with rollback
// and substitute-sample final review. Permissions mirror the existing
// operator/reviewer boundary.
type ReviewHandler struct {
	samples service.EmissionSampleService
	reviews service.ReviewService
}

func NewReviewHandler(samples service.EmissionSampleService, reviews service.ReviewService) *ReviewHandler {
	return &ReviewHandler{samples: samples, reviews: reviews}
}

func (h *ReviewHandler) Register(group *gin.RouterGroup) {
	group.POST("/samples/:id/void", middleware.RequireMinimumRole("operator"), h.voidSample)
	group.GET("/decisions/:id/rollback", middleware.RequireMinimumRole("viewer"), h.getRollback)
	group.POST("/decisions/:id/finalize", middleware.RequireMinimumRole("reviewer"), h.finalizeRollback)
}

func (h *ReviewHandler) voidSample(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.VoidSampleRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.samples.Void(c.Request.Context(), id, input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *ReviewHandler) getRollback(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	item, err := h.reviews.GetRollback(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}

func (h *ReviewHandler) finalizeRollback(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.FinalizeRollbackRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := h.reviews.Finalize(c.Request.Context(), id, input, actorFromContext(c), roleFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, item)
}
