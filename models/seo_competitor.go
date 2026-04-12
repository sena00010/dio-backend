package models

import "time"

// SeoCompetitor — Rakipler tablosu
type SeoCompetitor struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID uint      `gorm:"index;not null" json:"project_id"`
	Domain    string    `gorm:"size:255;not null" json:"domain"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	IsActive  bool      `gorm:"default:true" json:"is_active"`
	CreatedAt time.Time `json:"created_at"`

	Project SeoProject `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}

func (SeoCompetitor) TableName() string {
	return "seo_competitors"
}
