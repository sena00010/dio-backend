package models

import "time"

// PostSlide — carousel gönderisinin bir slaytı (kapak, değer veya CTA)
type PostSlide struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	PostID      uint      `gorm:"index;not null" json:"post_id"`
	Position    int       `gorm:"not null" json:"position"`
	Role        string    `gorm:"size:20" json:"role"` // cover | value | cta
	Headline    string    `gorm:"size:300" json:"headline"`
	Body        string    `gorm:"type:text" json:"body"`
	BgColor     string    `gorm:"size:20" json:"bg_color"`
	ImageURL    string    `gorm:"size:500" json:"image_url"`    // AI arka plan (kapak)
	RenderedURL string    `gorm:"size:500" json:"rendered_url"` // yayınlanacak 1080x1350 PNG
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
