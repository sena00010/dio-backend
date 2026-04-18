package models

import "time"

// UserMetaAccount — Facebook/Meta Ads API bağlantı bilgisi (user_meta_accounts tablosu)
type UserMetaAccount struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	UserID        uint       `gorm:"index;not null" json:"user_id"`
	CompanyID     uint       `gorm:"index" json:"company_id"`
	AdAccountID   *string    `gorm:"size:255" json:"ad_account_id"`
	AccessToken   string     `gorm:"type:text;not null" json:"-"`
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
	AccountName   *string    `gorm:"size:255" json:"account_name"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Relations
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName — Mevcut veritabanı tablo adı
func (UserMetaAccount) TableName() string {
	return "user_meta_accounts"
}
