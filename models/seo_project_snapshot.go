package models

import "time"

// SeoProjectSnapshot — Belirli bir dönemin özet metrikleri (KPI kartları + brand/non-brand)
type SeoProjectSnapshot struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ProjectID        uint      `gorm:"index;not null" json:"project_id"`
	PeriodLabel      string    `gorm:"size:20;not null" json:"period_label"`       // "7d","28d","90d","12m"
	StartDate        string    `gorm:"size:10;not null" json:"start_date"`         // "2026-03-14"
	EndDate          string    `gorm:"size:10;not null" json:"end_date"`           // "2026-04-10"
	TotalClicks      float64   `json:"total_clicks"`
	TotalImpressions float64   `json:"total_impressions"`
	AvgCTR           float64   `json:"avg_ctr"`
	AvgPosition      float64   `json:"avg_position"`
	// Önceki dönemle karşılaştırma
	PrevClicks       float64   `json:"prev_clicks"`
	PrevImpressions  float64   `json:"prev_impressions"`
	PrevCTR          float64   `json:"prev_ctr"`
	PrevPosition     float64   `json:"prev_position"`
	// Brand / Non-Brand
	BrandClicks      float64   `json:"brand_clicks"`
	NonBrandClicks   float64   `json:"non_brand_clicks"`
	BrandCTR         float64   `json:"brand_ctr"`
	NonBrandCTR      float64   `json:"non_brand_ctr"`
	// AI Özet
	AISummaryText    string    `gorm:"type:text" json:"ai_summary_text,omitempty"`
	// Timestamps
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// Relations
	Project SeoProject `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}
