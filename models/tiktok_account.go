package models

import "time"

// UserTikTokAccount — TikTok Marketing API bağlantı bilgisi (user_tiktok_accounts tablosu)
type UserTikTokAccount struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	UserID         uint       `gorm:"index;not null" json:"user_id"`
	CompanyID      uint       `gorm:"index;not null" json:"company_id"`
	AccessToken    string     `gorm:"type:text;not null" json:"-"`
	AdvertiserID   *string    `gorm:"size:100" json:"advertiser_id"`
	AccountName    *string    `gorm:"size:255" json:"account_name"`
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	// Relations
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName — Mevcut veritabanı tablo adı
func (UserTikTokAccount) TableName() string {
	return "user_tiktok_accounts"
}
