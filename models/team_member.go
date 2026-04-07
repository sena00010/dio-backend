package models

import "time"

// TeamMember — Bir markadaki ekip üyesi ilişkisi
type TeamMember struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CompanyID uint      `gorm:"index;not null" json:"company_id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Role      string    `gorm:"size:50;not null;default:'member'" json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relations (for Preload/JOIN)
	User    User    `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Company Company `gorm:"foreignKey:CompanyID" json:"company,omitempty"`
}
