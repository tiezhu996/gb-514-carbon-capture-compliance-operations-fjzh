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
	// InvalidatedReason and InvalidatedAt are set exactly once, when the sample
	// transitions into invalid. They stay populated even if a later workflow
	// re-verifies the sample, so the historical invalidation context survives.
	InvalidatedReason string     `json:"invalidatedReason" gorm:"size:500;not null;default:''"`
	InvalidatedAt     *time.Time `json:"invalidatedAt"`
	// AffectedDecisions is a read-model projection of compliance decisions that
	// reference this sample, populated by the repository layer.
	AffectedDecisions []AffectedDecision `json:"affectedDecisions" gorm:"-"`
}

func (item *EmissionSample) GetBase() *BaseModel { return &item.BaseModel }

func (item EmissionSample) TableName() string { return "emission_samples" }

var EmissionSampleInitialStatus = "collected"
