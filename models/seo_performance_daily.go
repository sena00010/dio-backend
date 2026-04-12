package models

import "time"

// SeoPerformanceDaily — Günlük zaman serisi verisi (trend grafiği)
type SeoPerformanceDaily struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ProjectID   uint      `gorm:"not null" json:"project_id"`
	Date        string    `gorm:"size:10;not null" json:"date"` // "2026-04-10"
	Clicks      float64   `json:"clicks"`
	Impressions float64   `json:"impressions"`
	CTR         float64   `json:"ctr"`
	Position    float64   `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
