package models

import "time"

// SeoIntegration — Google Search Console (veya gelecekte başka) entegrasyonları
type SeoIntegration struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	CompanyID    uint      `gorm:"index;not null" json:"company_id"`
	Provider     string    `gorm:"size:50;not null;default:'google'" json:"provider"`
	AccessToken  string    `gorm:"type:text" json:"-"`
	RefreshToken string    `gorm:"type:text" json:"-"`
	TokenExpiry  time.Time `json:"-"`
	SiteURL      string    `gorm:"size:500" json:"site_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relations
	Company Company `gorm:"foreignKey:CompanyID" json:"company,omitempty"`
}
