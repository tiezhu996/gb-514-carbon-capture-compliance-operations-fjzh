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
	// SampleID references the 排放样本 whose evidence underpins the decision.
	// Decisions without a sample keep the legacy behaviour unchanged.
	SampleID *uint `json:"sampleId" gorm:"index"`
	// SampleCode is a denormalised read-model value populated by the repository.
	SampleCode string `json:"sampleCode" gorm:"-"`
	// ReplacementSampleCode mirrors the code of the bound replacement sample
	// once a rolled-back decision is finalised again.
	ReplacementSampleCode string             `json:"replacementSampleCode" gorm:"-"`
	Revisions             []DecisionRevision `json:"revisions" gorm:"foreignKey:ComplianceDecisionID;constraint:OnDelete:CASCADE"`
	Rollbacks             []DecisionRollback `json:"rollbacks" gorm:"foreignKey:ComplianceDecisionID;constraint:OnDelete:CASCADE"`
}

func (item *ComplianceDecision) GetBase() *BaseModel { return &item.BaseModel }

func (item ComplianceDecision) TableName() string { return "compliance_decisions" }

var ComplianceDecisionInitialStatus = "draft"

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

// DecisionRollback records one rollback caused by invalidating the referenced
// emission sample. Rows are append-only: if a replacement sample is itself
// invalidated later, another row extends the rollback chain. A row without
// ResolvedAt represents the currently open rollback that blocks the decision
// from taking effect.
type DecisionRollback struct {
	ID                    uint       `json:"id" gorm:"primaryKey"`
	ComplianceDecisionID  uint       `json:"complianceDecisionId" gorm:"index:idx_decision_rollback_chain;not null"`
	ChainOrder            uint       `json:"chainOrder" gorm:"index:idx_decision_rollback_chain;not null"`
	FromState             string     `json:"fromState" gorm:"size:40;not null"`
	InvalidatedSampleID   uint       `json:"invalidatedSampleId" gorm:"not null;index"`
	InvalidatedSampleCode string     `json:"invalidatedSampleCode" gorm:"size:64;not null"`
	InvalidationReason    string     `json:"invalidationReason" gorm:"size:500;not null"`
	InvalidatedAt         time.Time  `json:"invalidatedAt" gorm:"not null"`
	RolledBackAt          time.Time  `json:"rolledBackAt" gorm:"index"`
	ReplacementSampleID   *uint      `json:"replacementSampleId"`
	ReplacementSampleCode string     `json:"replacementSampleCode" gorm:"-"`
	ResolvedAt            *time.Time `json:"resolvedAt" gorm:"index"`
	ResolvedBy            string     `json:"resolvedBy" gorm:"size:80;not null;default:''"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

func (DecisionRollback) TableName() string { return "decision_rollbacks" }

// AffectedDecision is the projection shown on a 排放样本 page: every decision
// referencing the sample, together with its current rollback context.
type AffectedDecision struct {
	DecisionID          uint       `json:"decisionId" gorm:"-"`
	Code                string     `json:"code" gorm:"-"`
	Name                string     `json:"name" gorm:"-"`
	Status              string     `json:"status" gorm:"-"`
	Version             uint       `json:"version" gorm:"-"`
	RolledBackAt        *time.Time `json:"rolledBackAt" gorm:"-"`
	RollbackReason      string     `json:"rollbackReason" gorm:"-"`
	ReplacementSampleID *uint      `json:"replacementSampleId" gorm:"-"`
	ResolvedAt          *time.Time `json:"resolvedAt" gorm:"-"`
}
