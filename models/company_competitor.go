package models

import "time"

// CompanyCompetitor - Rakip firmalar
type CompanyCompetitor struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CompanyID uint      `gorm:"index;not null" json:"company_id"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	Website   string    `gorm:"size:255" json:"website"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Company *Company `gorm:"foreignKey:CompanyID" json:"-"`
}

func (CompanyCompetitor) TableName() string {
	return "company_competitors"
}
