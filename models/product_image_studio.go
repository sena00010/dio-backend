package models

import "time"

type ProductImageStudio struct {
	ID                  uint                    `gorm:"primaryKey" json:"id"`
	CompanyID           uint                    `gorm:"index;not null" json:"company_id"`
	Title               string                  `gorm:"size:255" json:"title"`
	SourceImageURL      string                  `gorm:"size:500;not null" json:"source_image_url"`
	SourceFileName      string                  `gorm:"size:255" json:"source_file_name"`
	Category            string                  `gorm:"size:100" json:"category"`
	SlotGroup           string                  `gorm:"size:50;default:'2-4-ek'" json:"slot_group"`
	ActiveTab           string                  `gorm:"size:50;default:'urun-gorseli-studyosu'" json:"active_tab"`
	Status              string                  `gorm:"size:30;default:'draft'" json:"status"`
	SourceAnalysis      string                  `gorm:"type:text" json:"source_analysis,omitempty"`
	SelectedVariantID   *uint                   `json:"selected_variant_id,omitempty"`
	SelectedVariant     *ProductImageVariant    `gorm:"foreignKey:SelectedVariantID" json:"selected_variant,omitempty"`
	Variants            []ProductImageVariant   `gorm:"foreignKey:StudioID" json:"variants,omitempty"`
	SelectedAdVariantID *uint                   `json:"selected_ad_variant_id,omitempty"`
	SelectedAdVariant   *ProductImageAdVariant  `gorm:"foreignKey:SelectedAdVariantID" json:"selected_ad_variant,omitempty"`
	AdVariants          []ProductImageAdVariant `gorm:"foreignKey:StudioID" json:"ad_variants,omitempty"`
	CreatedAt           time.Time               `json:"created_at"`
	UpdatedAt           time.Time               `json:"updated_at"`
}

type ProductImageVariant struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	StudioID      uint      `gorm:"index;not null" json:"studio_id"`
	VariantKey    string    `gorm:"size:80;index" json:"variant_key"`
	Name          string    `gorm:"size:150" json:"name"`
	Platform      string    `gorm:"size:50" json:"platform"`
	FormatLabel   string    `gorm:"size:50" json:"format_label"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	Background    string    `gorm:"size:100" json:"background"`
	Score         int       `json:"score"`
	Status        string    `gorm:"size:30;default:'pending'" json:"status"`
	PreviewURL    string    `gorm:"size:500" json:"preview_url"`
	Prompt        string    `gorm:"type:text" json:"prompt,omitempty"`
	Provider      string    `gorm:"size:50" json:"provider,omitempty"`
	BadgeLabel    string    `gorm:"size:50" json:"badge_label,omitempty"`
	BadgeTone     string    `gorm:"size:30" json:"badge_tone,omitempty"`
	DesignNotes   string    `gorm:"type:text" json:"design_notes,omitempty"`
	IsRecommended bool      `gorm:"default:false" json:"is_recommended"`
	SortOrder     int       `gorm:"default:0" json:"sort_order"`
	MetaJSON      string    `gorm:"type:text" json:"meta_json,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ProductImageAdVariant struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	StudioID      uint      `gorm:"index;not null" json:"studio_id"`
	VariantKey    string    `gorm:"size:80;index" json:"variant_key"`
	Name          string    `gorm:"size:150" json:"name"`
	FormatKey     string    `gorm:"size:60;index" json:"format_key"`
	Platform      string    `gorm:"size:50" json:"platform"`
	Width         int       `json:"width"`
	Height        int       `json:"height"`
	Score         int       `json:"score"`
	Status        string    `gorm:"size:30;default:'pending'" json:"status"`
	PreviewURL    string    `gorm:"size:500" json:"preview_url"`
	Prompt        string    `gorm:"type:text" json:"prompt,omitempty"`
	Provider      string    `gorm:"size:50" json:"provider,omitempty"`
	BadgeLabel    string    `gorm:"size:50" json:"badge_label,omitempty"`
	BadgeTone     string    `gorm:"size:30" json:"badge_tone,omitempty"`
	Headline      string    `gorm:"size:255" json:"headline,omitempty"`
	SubHeadline   string    `gorm:"size:255" json:"sub_headline,omitempty"`
	CTA           string    `gorm:"size:120" json:"cta,omitempty"`
	TextMode      string    `gorm:"size:30;default:'ai'" json:"text_mode"`
	TextAlign     string    `gorm:"size:20;default:'left'" json:"text_align"`
	TextPosition  string    `gorm:"size:30;default:'bottom_left'" json:"text_position"`
	FormatLabel   string    `gorm:"size:80" json:"format_label"`
	DesignNotes   string    `gorm:"type:text" json:"design_notes,omitempty"`
	IsRecommended bool      `gorm:"default:false" json:"is_recommended"`
	SortOrder     int       `gorm:"default:0" json:"sort_order"`
	MetaJSON      string    `gorm:"type:text" json:"meta_json,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
