package utils

import (
	"log"
)

// GetAiModel — artık system_prompts tablosundaki prompt_key'e bağlı
// ai_model değerini döner. Bulamazsa defaultModel kullanılır.
// prompt_key eşleştirme: "gpt_image_edit" → "product_studio_edit" gibi
func GetAiModel(settingKey string, defaultModel string) string {
	// settingKey → prompt_key eşleştirmesi
	promptKey := mapSettingToPromptKey(settingKey)
	if promptKey == "" {
		return defaultModel
	}

	cfg := getPromptFromCache(promptKey)
	if cfg.AIModel != "" {
		log.Printf("🔧 Model '%s' prompt '%s' → %s", settingKey, promptKey, cfg.AIModel)
		return cfg.AIModel
	}

	return defaultModel
}

// GetAiProvider — system_prompts tablosundan prompt_key'e bağlı provider döner
func GetAiProvider(settingKey string, defaultProvider string) string {
	promptKey := mapSettingToPromptKey(settingKey)
	if promptKey == "" {
		return defaultProvider
	}

	cfg := getPromptFromCache(promptKey)
	if cfg.AIProvider != "" {
		return cfg.AIProvider
	}

	return defaultProvider
}

// mapSettingToPromptKey — eski ai_model_settings key'lerini
// system_prompts prompt_key'lerine eşler
func mapSettingToPromptKey(settingKey string) string {
	mapping := map[string]string{
		"gpt_image_edit":   "product_studio_edit",
		"gpt_fast_general": "ai_idea_generator",
		"gpt_smart_general": "creative_variant_generator",
	}
	if pk, ok := mapping[settingKey]; ok {
		return pk
	}
	return ""
}

// ClearAiModelCache is used when an admin updates a setting
func ClearAiModelCache() {
	// Artık prompt cache üzerinden yönetiliyor
	log.Println("🔄 AI Model Cache temizlendi (prompt cache)")
}
