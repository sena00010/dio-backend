package models

import "time"

// Post — ileride detaylandırılacak
type Post struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CompanyID uint      `gorm:"index;not null" json:"company_id"`
	Title     string    `gorm:"size:255" json:"title"`
	Content   string    `gorm:"type:text" json:"content"`
	Platform    string    `gorm:"size:50" json:"platform"`
	Hashtags    string    `gorm:"type:text" json:"hashtags,omitempty"`
	ImagePrompt     string    `gorm:"type:text" json:"image_prompt,omitempty"`
	ImageURL        string    `gorm:"size:500" json:"image_url,omitempty"`
	SuggestedLayout string    `gorm:"size:50" json:"suggested_layout,omitempty"`
	SuggestedFont   string    `gorm:"size:50" json:"suggested_font,omitempty"`
	ContentItems    string    `gorm:"type:text" json:"content_items,omitempty"`
	DesignLayers    string    `gorm:"type:text" json:"design_layers,omitempty"`
	LogoPosition    string    `gorm:"type:text" json:"logo_position,omitempty"`
	Status          string    `gorm:"size:20;default:'draft'" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
