package models

import "time"

// SeoAIVisibility — AI platformlarındaki görünürlük metrikleri
type SeoAIVisibility struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	ProjectID          uint      `gorm:"uniqueIndex;not null" json:"project_id"`
	ChatGPTVisibility  float64   `json:"chatgpt_visibility"`   // % oran
	ChatGPTMentions    int       `json:"chatgpt_mentions"`
	PerplexityMentions int       `json:"perplexity_mentions"`
	GeminiMentions     int       `json:"gemini_mentions"`
	GoogleAIOverviews  int       `json:"google_ai_overviews"`  // AI Overview'da çıkma sayısı
	EEATScore          int       `json:"eeat_score"`           // 0-100
	SectorAvgVisibility float64  `json:"sector_avg_visibility"`
	// Rakip verileri (JSON)
	CompetitorData     string    `gorm:"type:text" json:"competitor_data"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	Project SeoProject `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}

func (SeoAIVisibility) TableName() string {
	return "seo_ai_visibility"
}
