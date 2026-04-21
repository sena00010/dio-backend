package models

import "time"

// AdminUser — Admin panele erişebilen kullanıcılar
type AdminUser struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Email        string    `gorm:"size:255;uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	FullName     string    `gorm:"size:200;not null" json:"full_name"`
	Role         string    `gorm:"size:50;not null;default:'admin'" json:"role"` // admin, super_admin
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
