package models

import "time"

// SystemPrompt — Admin panelden yönetilen AI promptları
type SystemPrompt struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	PromptKey   string    `gorm:"size:100;uniqueIndex;not null" json:"prompt_key"`   // benzersiz anahtar (ör: "ad_studio_main")
	Category    string    `gorm:"size:100;not null" json:"category"`                 // sidebar kategorisi (ör: "Ürün Görsel Stüdyosu")
	Title       string    `gorm:"size:255;not null" json:"title"`                    // gösterim başlığı
	Description string    `gorm:"type:text" json:"description"`                      // admin için açıklama (ne işe yaradığı)
	Content     string    `gorm:"type:longtext;not null" json:"content"`             // asıl prompt metni
	Variables   string    `gorm:"type:text" json:"variables"`                        // kullanılan değişkenler listesi (JSON)
	AiProvider  string    `gorm:"size:50" json:"ai_provider"`                        // ör: "openai", "gemini"
	AiModel     string    `gorm:"size:100" json:"ai_model"`                          // ör: "gpt-4o", "gemini-1.5-pro"
	IsActive    bool      `gorm:"default:true" json:"is_active"`                     // aktif/pasif
	UpdatedBy   string    `gorm:"size:255" json:"updated_by"`                        // son güncelleyen admin email
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
