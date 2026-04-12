package models

import (
	"time"

	"gorm.io/datatypes"
)

// SeoContentBrief — LLM'den üretilen SEO içerik brief'i
type SeoContentBrief struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	ProjectID       uint           `gorm:"column:project_id;not null;index" json:"project_id"`
	Title           string         `gorm:"column:title;size:500;not null" json:"title"`
	OpportunityType string         `gorm:"column:opportunity_type;size:50" json:"opportunity_type"` // blog, category, product, guide
	TargetKeywords  datatypes.JSON `gorm:"column:target_keywords;type:json" json:"target_keywords"` // ["kelime1","kelime2"]
	AiBriefSteps    datatypes.JSON `gorm:"column:ai_brief_steps;type:json" json:"ai_brief_steps"`   // [{step,content}]
	AiSummary       string         `gorm:"column:ai_summary;type:text" json:"ai_summary"`
	Status          string         `gorm:"column:status;size:20;default:draft" json:"status"` // draft, in_progress, published
	Position        float64        `gorm:"column:position" json:"position"`
	Impressions     float64        `gorm:"column:impressions" json:"impressions"`
	CTR             float64        `gorm:"column:ctr" json:"ctr"`
	EstimatedClicks int            `gorm:"column:estimated_clicks" json:"estimated_clicks"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// TableName — GORM tablo adı
func (SeoContentBrief) TableName() string {
	return "seo_content_briefs"
}
