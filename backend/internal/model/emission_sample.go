package model

import "time"

// EmissionSample models 排放样本 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
type EmissionSample struct {
	BaseModel
	Facility    string    `json:"facility" gorm:"size:120;index"`
	Owner       string    `json:"owner" gorm:"size:120;index"`
	Category    string    `json:"category" gorm:"size:80;index"`
	RiskLevel   string    `json:"riskLevel" gorm:"size:32;index"`
	MetricValue float64   `json:"metricValue"`
	MetricUnit  string    `json:"metricUnit" gorm:"size:24"`
	EffectiveAt time.Time `json:"effectiveAt"`
	Evidence    string    `json:"evidence" gorm:"size:2000"`
	RelatedCode string    `json:"relatedCode" gorm:"size:64;index"`
	// UnitCode identifies the 捕集装置 the sample was taken from. A substitute
	// sample for a rolled-back decision must share it.
	UnitCode string `json:"unitCode" gorm:"size:64;index"`
	// VoidedAt / VoidedReason / VoidedBy are frozen the moment the sample is
	// invalidated and never cleared afterwards, even if the sample re-enters
	// testing/verified. They are the reference time for substitute eligibility.
	VoidedAt     *time.Time `json:"voidedAt,omitempty" gorm:"index"`
	VoidedReason string     `json:"voidedReason,omitempty" gorm:"size:500"`
	VoidedBy     string     `json:"voidedBy,omitempty" gorm:"size:80;index"`
	// AffectedDecisions is populated by the service for read responses and is
	// not persisted on this aggregate.
	AffectedDecisions []AffectedDecision `json:"affectedDecisions,omitempty" gorm:"-"`
}

func (item *EmissionSample) GetBase() *BaseModel { return &item.BaseModel }

func (item EmissionSample) TableName() string { return "emission_samples" }

const EmissionSampleInitialStatus = "collected"

// AffectedDecision is the read projection shown on a sample page: every
// accepted/escalated decision that cited the sample and was rolled back.
type AffectedDecision struct {
	DecisionID         uint       `json:"decisionId" gorm:"column:decision_id"`
	DecisionCode       string     `json:"decisionCode" gorm:"column:decision_code"`
	State              string     `json:"state" gorm:"column:state"`
	PreviousState      string     `json:"previousState" gorm:"column:previous_state"`
	RollbackReason     string     `json:"rollbackReason" gorm:"column:rollback_reason"`
	RolledBackAt       time.Time  `json:"rolledBackAt" gorm:"column:rolled_back_at"`
	SubstituteSampleID *uint      `json:"substituteSampleId,omitempty" gorm:"column:substitute_sample_id"`
	SubstituteCode     string     `json:"substituteCode,omitempty" gorm:"column:substitute_code"`
	FinalizedAt        *time.Time `json:"finalizedAt,omitempty" gorm:"column:finalized_at"`
}
