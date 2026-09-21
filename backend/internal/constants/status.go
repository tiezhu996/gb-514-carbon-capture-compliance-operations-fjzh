package constants

// Shared status values are mirrored in frontend/src/types/status.ts. Keeping
// the lists explicit makes state-machine drift visible during code review.

type UnitState string

const (
	UnitStateStandby UnitState = "standby"
	UnitStateRunning UnitState = "running"
	UnitStateLimited UnitState = "limited"
	UnitStateStopped UnitState = "stopped"
)

var AllUnitState = []string{"standby", "running", "limited", "stopped"}

type DecisionState string

const (
	DecisionStateDraft          DecisionState = "draft"
	DecisionStateReview         DecisionState = "review"
	DecisionStateAccepted       DecisionState = "accepted"
	DecisionStateEscalated      DecisionState = "escalated"
	DecisionStateReviewRequired DecisionState = "review_required"
)

var AllDecisionState = []string{"draft", "review", "accepted", "escalated", "review_required"}

var CaptureUnitTransitions = map[string]map[string]bool{
	"standby": {"running": true, "limited": true},
	"running": {"limited": true, "stopped": true, "standby": true},
	"limited": {"stopped": true, "running": true},
	"stopped": {"limited": true},
}

var PermitRuleTransitions = map[string]map[string]bool{
	"draft":      {"active": true, "superseded": true},
	"active":     {"superseded": true, "retired": true, "draft": true},
	"superseded": {"retired": true, "active": true},
	"retired":    {"superseded": true},
}

var EmissionSampleTransitions = map[string]map[string]bool{
	"collected": {"testing": true, "verified": true},
	"testing":   {"verified": true, "invalid": true, "collected": true},
	"verified":  {"invalid": true, "testing": true},
	"invalid":   {"verified": true},
}

// ComplianceDecisionTransitions covers the regular review workflow. The
// review_required state is entered only by the sample-void rollback flow and
// left only by a substitute-sample final review, so it deliberately has no
// edges in this graph: generic transition requests to or from it are rejected.
var ComplianceDecisionTransitions = map[string]map[string]bool{
	"draft":     {"review": true},
	"review":    {"accepted": true, "escalated": true, "draft": true},
	"accepted":  {"escalated": true, "review": true},
	"escalated": {"accepted": true},
}

// FinalDecisionStates are the decisions whose conclusion is actively in force
// and therefore must be rolled back when their emission sample is voided.
var FinalDecisionStates = []string{"accepted", "escalated"}

func CanTransition(graph map[string]map[string]bool, from, to string) bool {
	targets, exists := graph[from]
	return exists && targets[to]
}
