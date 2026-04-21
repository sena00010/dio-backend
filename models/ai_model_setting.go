package models

import "time"

// AiModelSetting — Admin panelden yönetilen AI model konfigürasyonları
type AiModelSetting struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	SettingKey  string    `gorm:"size:100;uniqueIndex;not null" json:"setting_key"` // ör: "ad_studio_copy", "product_image_edit"
	Category    string    `gorm:"size:100;not null" json:"category"`                // ör: "Reklam Stüdyosu"
	Title       string    `gorm:"size:255;not null" json:"title"`                   // ör: "Reklam Kopya Üretim Modeli"
	Description string    `gorm:"type:text" json:"description"`                     // ör: "Reklam Stüdyosunda copy yazarken kullanılan OpenAI modeli"
	ModelName   string    `gorm:"size:100;not null" json:"model_name"`              // ör: "gpt-4o-mini", "gpt-4o", "gpt-image-1"
	Provider    string    `gorm:"size:50;default:'openai'" json:"provider"`         // ör: "openai", "gemini"
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	UpdatedBy   string    `gorm:"size:255" json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
