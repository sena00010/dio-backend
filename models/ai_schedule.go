package models

import "time"

// AiSchedule — İçerik takvimi planlaması
type AiSchedule struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	CompanyID   uint      `gorm:"index;not null" json:"company_id"`
	PostID      *uint     `gorm:"index" json:"post_id"`
	IdeaID      *uint     `gorm:"index" json:"idea_id"`
	Platform    string    `gorm:"size:50" json:"platform"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Status      string    `gorm:"size:20;default:'planned'" json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Relations
	Post    Post    `gorm:"foreignKey:PostID" json:"post,omitempty"`
	Idea    AiIdea  `gorm:"foreignKey:IdeaID" json:"idea,omitempty"`
	Company Company `gorm:"foreignKey:CompanyID" json:"company,omitempty"`
}
