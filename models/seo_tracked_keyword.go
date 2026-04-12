package models

import "time"

// SeoTrackedKeyword — Takip edilen anahtar kelimeler
type SeoTrackedKeyword struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID uint      `gorm:"not null;index" json:"project_id"`
	Keyword   string    `gorm:"size:500;not null" json:"keyword"`
	Segment   string    `gorm:"size:20;default:'Non-Brand'" json:"segment"` // Brand, Non-Brand
	Intent    string    `gorm:"size:30;default:'informational'" json:"intent"` // informational, commercial, navigational
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SeoKeywordDailyStat — Kelime bazlı günlük GSC istatistikleri
type SeoKeywordDailyStat struct {
	ID        uint    `gorm:"primaryKey" json:"id"`
	ProjectID uint    `gorm:"not null;index" json:"project_id"`
	KeywordID uint    `gorm:"not null;index" json:"keyword_id"`
	Date      string  `gorm:"size:10;not null;index" json:"date"` // YYYY-MM-DD
	Position  float64 `json:"position"`
	Clicks    float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	CTR       float64 `json:"ctr"`

	// Composite unique: bir kelime + bir gün = tek kayıt
	// GORM'da uniqueIndex tanımı
}
