package models

import "time"

// UserLinkedInAccount — LinkedIn Marketing API bağlantı bilgisi (user_linkedin_accounts tablosu)
type UserLinkedInAccount struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	UserID         uint       `gorm:"index;not null" json:"user_id"`
	CompanyID      uint       `gorm:"index;not null" json:"company_id"`
	AccessToken    string     `gorm:"type:text;not null" json:"-"`
	AccountURN     *string    `gorm:"column:account_urn;size:255" json:"account_urn"`
	AccountName    *string    `gorm:"size:255" json:"account_name"`
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	// Relations
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName — Mevcut veritabanı tablo adı
func (UserLinkedInAccount) TableName() string {
	return "user_linkedin_accounts"
}
