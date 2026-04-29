package utils

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// PromptConfig — prompt içeriği + hangi model/provider ile çalışacağı
type PromptConfig struct {
	Content    string
	AIProvider string // "openai" veya "gemini"
	AIModel    string // "gpt-4o-mini", "dall-e-2", vb.
}

var (
	promptCache      = make(map[string]PromptConfig)
	promptCacheMutex sync.RWMutex
	promptCacheTTLs  = make(map[string]time.Time)
)

const promptCacheDuration = 1 * time.Minute

var funcMap = template.FuncMap{}

// GetRenderedPrompt fetches the prompt content from cache/db and renders it using text/template
func GetRenderedPrompt(promptKey string, data map[string]interface{}) (string, error) {
	cfg := getPromptFromCache(promptKey)

	if cfg.Content == "" {
		log.Printf("⚠️ Missing prompt definition for key: %s (Check DB)", promptKey)
		return "", nil
	}

	tmpl, err := template.New(promptKey).Funcs(funcMap).Parse(cfg.Content)
	if err != nil {
		log.Printf("❌ Failed to parse template for prompt %s: %v", promptKey, err)
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("❌ Failed to render template for prompt %s: %v", promptKey, err)
		return "", err
	}

	return buf.String(), nil
}

// GetPromptWithModel — prompt içeriğini render edip, model bilgisini de döner.
// Dönen PromptConfig.AIProvider/AIModel boşsa → ai_model_settings'ten fallback kullan.
func GetPromptWithModel(promptKey string, data map[string]interface{}) (PromptConfig, error) {
	cfg := getPromptFromCache(promptKey)

	if cfg.Content == "" {
		return PromptConfig{}, nil
	}

	tmpl, err := template.New(promptKey).Funcs(funcMap).Parse(cfg.Content)
	if err != nil {
		return PromptConfig{}, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return PromptConfig{}, err
	}

	return PromptConfig{
		Content:    buf.String(),
		AIProvider: cfg.AIProvider,
		AIModel:    cfg.AIModel,
	}, nil
}

func getPromptFromCache(promptKey string) PromptConfig {
	promptCacheMutex.RLock()
	cachedCfg, found := promptCache[promptKey]
	expiry, hasExpiry := promptCacheTTLs[promptKey]
	promptCacheMutex.RUnlock()

	if found && hasExpiry && time.Now().Before(expiry) {
		return cachedCfg
	}

	// DB'den çek
	var prompt models.SystemPrompt
	err := database.DB.Where("prompt_key = ?", promptKey).First(&prompt).Error
	if err != nil {
		return PromptConfig{}
	}

	cfg := PromptConfig{
		Content:    prompt.Content,
		AIProvider: strings.TrimSpace(prompt.AiProvider),
		AIModel:    strings.TrimSpace(prompt.AiModel),
	}

	promptCacheMutex.Lock()
	promptCache[promptKey] = cfg
	promptCacheTTLs[promptKey] = time.Now().Add(promptCacheDuration)
	promptCacheMutex.Unlock()

	return cfg
}

// ClearPromptCache can be called from admin routes when a prompt is explicitly updated
func ClearPromptCache(promptKey string) {
	promptCacheMutex.Lock()
	defer promptCacheMutex.Unlock()
	delete(promptCache, promptKey)
	delete(promptCacheTTLs, promptKey)
	log.Printf("🔄 Prompt cache cleared for: %s", promptKey)
}
