package models

import "time"

type Company struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	UserID            uint      `gorm:"index;not null" json:"user_id"`
	Name              string    `gorm:"size:255;not null" json:"name"`
	Sector            string    `gorm:"size:255" json:"sector,omitempty"`
	Goal              string    `gorm:"size:500" json:"goal,omitempty"`
	ContentLanguage   string    `gorm:"size:50" json:"content_language,omitempty"`
	Platforms         string    `gorm:"type:text" json:"platforms,omitempty"`
	Website           string    `gorm:"size:500" json:"website,omitempty"`
	LogoURL           string    `gorm:"size:500" json:"logo_url,omitempty"`
	BrandColors       string    `gorm:"size:500" json:"brand_colors,omitempty"`
	Slogan            string    `gorm:"size:500" json:"slogan,omitempty"`
	Description       string    `gorm:"type:text" json:"description,omitempty"`
	ForbiddenWords    string    `gorm:"type:text" json:"forbidden_words,omitempty"`
	TargetAudience    string    `gorm:"size:500" json:"target_audience,omitempty"`
	ProductCategories string    `gorm:"type:text" json:"product_categories,omitempty"`
	ToneOfVoice       string    `gorm:"size:100" json:"tone_of_voice,omitempty"`
	DynamicFields     *string   `gorm:"type:json" json:"dynamic_fields,omitempty"`
	BrandType         string    `gorm:"size:100" json:"brand_type,omitempty"`
	BrandFont         string    `gorm:"size:100" json:"brand_font,omitempty"`
	SocialMedia       string    `gorm:"type:text" json:"social_media,omitempty"`
	CompetitiveEdge   string    `gorm:"type:text" json:"competitive_edge,omitempty"`
	CurrentStep       int       `gorm:"default:0" json:"current_step"`
	Posts             []Post               `gorm:"foreignKey:CompanyID" json:"posts,omitempty"`
	Competitors       []CompanyCompetitor  `gorm:"foreignKey:CompanyID" json:"competitors,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}
