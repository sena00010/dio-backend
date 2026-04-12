package models

import "time"

// SeoProject — GSC'den gelen her site = 1 proje
type SeoProject struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	CompanyID       uint      `gorm:"index;not null" json:"company_id"`
	SiteURL         string    `gorm:"size:500;not null" json:"site_url"`          // sc-domain:example.com veya https://example.com/
	DomainURL       string    `gorm:"size:500" json:"domain_url"`                 // https://example.com (temiz URL)
	ProjectName     string    `gorm:"size:255" json:"project_name"`               // Proje adı (örn: "Creator TR")
	DisplayName     string    `gorm:"size:255" json:"display_name"`               // example.com (UI'da gösterilen)
	PermissionLevel string    `gorm:"size:50" json:"permission_level"`            // siteOwner, siteFullUser, etc.
	IsActive        bool      `gorm:"default:true" json:"is_active"`
	LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Relations
	Company   Company              `gorm:"foreignKey:CompanyID" json:"company,omitempty"`
	Snapshots []SeoProjectSnapshot `gorm:"foreignKey:ProjectID" json:"snapshots,omitempty"`
}
