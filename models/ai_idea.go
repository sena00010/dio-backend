package models

import "time"

// AiIdea — AI ile üretilen içerik fikirleri
type AiIdea struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	CompanyID             uint      `gorm:"index;not null" json:"company_id"`
	UserInputTopic        string    `gorm:"size:500;not null" json:"user_input_topic"`
	Platform              string    `gorm:"size:50;not null" json:"platform"`
	CampaignType          string    `gorm:"size:100;not null" json:"campaign_type"`
	GeneratedTitle        string    `gorm:"size:500" json:"generated_title"`
	GeneratedDescription  string    `gorm:"type:text" json:"generated_description"`
	GeneratedHashtags     string    `gorm:"type:text" json:"generated_hashtags"`
	ImageSuggestionPrompt string    `gorm:"type:text" json:"image_suggestion_prompt"`
	SuggestedLayout       string    `gorm:"size:50" json:"suggested_layout"`
	SuggestedFont         string    `gorm:"size:50" json:"suggested_font"`
	ContentItems          string    `gorm:"type:text" json:"content_items"`
	DesignLayers          string    `gorm:"type:text" json:"design_layers"`
	ImageURL              string    `gorm:"size:500" json:"image_url"`
	IsSelected            bool      `gorm:"default:false" json:"is_selected"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}
