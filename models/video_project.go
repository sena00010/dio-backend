package models

import "time"

// VideoProject — içerikten otomatik üretilen kısa video (Reels/TikTok/Shorts)
type VideoProject struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	CompanyID   uint      `gorm:"index;not null" json:"company_id"`
	PostID      *uint     `gorm:"index" json:"post_id"`
	Title       string    `gorm:"size:255" json:"title"`
	Brief       string    `gorm:"type:text" json:"brief"`
	DurationSec int       `json:"duration_sec"`
	Format      string    `gorm:"size:10" json:"format"` // 9:16 | 1:1
	Voice       string    `gorm:"size:20" json:"voice"`  // kadin | erkek | yok
	Status      string    `gorm:"size:20;index" json:"status"`
	Stage       string    `gorm:"size:120" json:"stage"` // kullanıcıya gösterilen adım
	Progress    int       `json:"progress"`              // 0-100
	Script      string    `gorm:"type:text" json:"script"`
	VideoURL    string    `gorm:"size:500" json:"video_url"`
	ThumbURL    string    `gorm:"size:500" json:"thumb_url"`
	Error       string    `gorm:"size:500" json:"error"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
