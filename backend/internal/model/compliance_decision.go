package model

import "time"

// ComplianceDecision models 合规决定 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
type ComplianceDecision struct {
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
	// UnitCode is the 捕集装置 and SampleCode is the cited 排放样本. Sample
	// voiding rolls back every final decision referencing the same sample.
	UnitCode   string `json:"unitCode" gorm:"size:64;index"`
	SampleCode string `json:"sampleCode" gorm:"size:64;index"`
	// Rollback is the active rollback linkage while the decision is in
	// review_required. It stays attached after final review so the chain
	// remains readable after refresh.
	Rollback  *DecisionRollback  `json:"rollback,omitempty" gorm:"foreignKey:ComplianceDecisionID;references:ID;constraint:OnDelete:CASCADE"`
	Revisions []DecisionRevision `json:"revisions" gorm:"foreignKey:ComplianceDecisionID;constraint:OnDelete:CASCADE"`
}

func (item *ComplianceDecision) GetBase() *BaseModel { return &item.BaseModel }

func (item ComplianceDecision) TableName() string { return "compliance_decisions" }

const ComplianceDecisionInitialStatus = "draft"

// DecisionRevision is an append-only compliance snapshot. It keeps the
// evidence and request context that justified every aggregate version.
type DecisionRevision struct {
	ID                   uint      `json:"id" gorm:"primaryKey"`
	ComplianceDecisionID uint      `json:"complianceDecisionId" gorm:"uniqueIndex:idx_decision_revision_version;not null"`
	Version              uint      `json:"version" gorm:"uniqueIndex:idx_decision_revision_version;not null"`
	State                string    `json:"state" gorm:"size:40;not null"`
	Evidence             string    `json:"evidence" gorm:"size:2000;not null"`
	Reason               string    `json:"reason" gorm:"size:500;not null"`
	Actor                string    `json:"actor" gorm:"size:80;not null;index"`
	RequestID            string    `json:"requestId" gorm:"size:64;not null;index"`
	CreatedAt            time.Time `json:"createdAt" gorm:"index"`
}

// DecisionRollback records the single rollback chain of a decision that cited
// a voided sample. The original conclusion, evidence and the void reason are
// preserved here and in the append-only revisions; the conclusion stops being
// in force until a reviewer completes the chain with a substitute sample.
type DecisionRollback struct {
	ID                   uint       `json:"id" gorm:"primaryKey"`
	ComplianceDecisionID uint       `json:"complianceDecisionId" gorm:"uniqueIndex;not null"`
	VoidedSampleID       uint       `json:"voidedSampleId" gorm:"index;not null"`
	VoidedSampleCode     string     `json:"voidedSampleCode" gorm:"size:64;not null"`
	PreviousState        string     `json:"previousState" gorm:"size:40;not null"`
	PreviousVersion      uint       `json:"previousVersion" gorm:"not null"`
	VoidReason           string     `json:"voidReason" gorm:"size:500;not null"`
	VoidedAt             time.Time  `json:"voidedAt" gorm:"index;not null"`
	RolledBackAt         time.Time  `json:"rolledBackAt" gorm:"not null"`
	RolledBackBy         string     `json:"rolledBackBy" gorm:"size:80;not null"`
	SubstituteSampleID   *uint      `json:"substituteSampleId,omitempty" gorm:"index"`
	SubstituteCode       string     `json:"substituteCode,omitempty" gorm:"size:64"`
	FinalState           string     `json:"finalState,omitempty" gorm:"size:40"`
	FinalReason          string     `json:"finalReason,omitempty" gorm:"size:500"`
	FinalizedBy          string     `json:"finalizedBy,omitempty" gorm:"size:80"`
	FinalizedRequestID   string     `json:"finalizedRequestId,omitempty" gorm:"size:64"`
	FinalizedAt          *time.Time `json:"finalizedAt,omitempty" gorm:"index"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}

func (DecisionRollback) TableName() string { return "decision_rollbacks" }
