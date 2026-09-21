package dto

import "time"

// CreateComplianceDecision is the public write contract for 合规决定. Status is deliberately
// omitted so callers cannot bypass the service state machine.
type CreateComplianceDecision struct {
	Code        string    `json:"code" binding:"required,min=2,max=64"`
	Name        string    `json:"name" binding:"required,min=2,max=160"`
	Description string    `json:"description" binding:"max=1000"`
	Facility    string    `json:"facility" binding:"required,max=120"`
	Owner       string    `json:"owner" binding:"required,max=120"`
	Category    string    `json:"category" binding:"required,max=80"`
	RiskLevel   string    `json:"riskLevel" binding:"required,oneof=low medium high critical"`
	MetricValue float64   `json:"metricValue"`
	MetricUnit  string    `json:"metricUnit" binding:"max=24"`
	EffectiveAt time.Time `json:"effectiveAt" binding:"required"`
	Evidence    string    `json:"evidence" binding:"max=2000"`
	RelatedCode string    `json:"relatedCode" binding:"max=64"`
	UnitCode    string    `json:"unitCode" binding:"required,min=2,max=64"`
	SampleCode  string    `json:"sampleCode" binding:"max=64"`
}

type UpdateComplianceDecision struct {
	ExpectedVersion uint      `json:"expectedVersion" binding:"required"`
	Name            string    `json:"name" binding:"required,min=2,max=160"`
	Description     string    `json:"description" binding:"max=1000"`
	Facility        string    `json:"facility" binding:"required,max=120"`
	Owner           string    `json:"owner" binding:"required,max=120"`
	Category        string    `json:"category" binding:"required,max=80"`
	RiskLevel       string    `json:"riskLevel" binding:"required,oneof=low medium high critical"`
	MetricValue     float64   `json:"metricValue"`
	MetricUnit      string    `json:"metricUnit" binding:"max=24"`
	EffectiveAt     time.Time `json:"effectiveAt" binding:"required"`
	Evidence        string    `json:"evidence" binding:"max=2000"`
	RelatedCode     string    `json:"relatedCode" binding:"max=64"`
	UnitCode        string    `json:"unitCode" binding:"required,min=2,max=64"`
	SampleCode      string    `json:"sampleCode" binding:"max=64"`
}

// FinalizeRollbackRequest completes a rolled-back decision using one
// substitute emission sample. The substitute must be verified, belong to the
// same unit and have been sampled strictly after the original void timestamp.
type FinalizeRollbackRequest struct {
	SubstituteSampleID uint   `json:"substituteSampleId" binding:"required"`
	Reason             string `json:"reason" binding:"required,min=3,max=500"`
}
