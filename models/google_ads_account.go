package models

import "time"

// UserGoogleAdsAccount — Google Ads API bağlantı bilgisi (user_google_ads_accounts tablosu)
type UserGoogleAdsAccount struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	UserID              uint       `gorm:"index;not null" json:"user_id"`
	CompanyID           uint       `gorm:"index;not null" json:"company_id"`
	AccessToken         *string    `gorm:"type:text" json:"-"`
	RefreshToken        *string    `gorm:"type:text" json:"-"`
	TokenExpiresAt      *time.Time `json:"token_expires_at,omitempty"`
	GoogleAdsCustomerID *string    `gorm:"column:google_ads_customer_id;size:50" json:"google_ads_customer_id"`
	AccountName         *string    `gorm:"size:255" json:"account_name"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`

	// Relations
	User User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

// TableName — Mevcut veritabanı tablo adı
func (UserGoogleAdsAccount) TableName() string {
	return "user_google_ads_accounts"
}
