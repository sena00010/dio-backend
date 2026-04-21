package utils

import (
	"bytes"
	"log"
	"sync"
	"text/template"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

var (
	promptCache      = make(map[string]string)
	promptCacheMutex sync.RWMutex
	promptCacheTTLs  = make(map[string]time.Time)
)

const promptCacheDuration = 1 * time.Minute

// formatMap is a helper function to format maps for text/template if needed (though map keys are accessed via .Key naturally)
// Can be extended with custom template functions later
var funcMap = template.FuncMap{}

// GetRenderedPrompt fetches the prompt content from cache/db and renders it using text/template
func GetRenderedPrompt(promptKey string, data map[string]interface{}) (string, error) {
	content := getPromptContentFromCache(promptKey)

	// Fallback to error message if missing
	if content == "" {
		log.Printf("⚠️ Missing prompt definition for key: %s (Check DB)", promptKey)
		return "", nil // or return error, but "" avoids crash
	}

	tmpl, err := template.New(promptKey).Funcs(funcMap).Parse(content)
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

func getPromptContentFromCache(promptKey string) string {
	promptCacheMutex.RLock()
	cachedContent, found := promptCache[promptKey]
	expiry, hasExpiry := promptCacheTTLs[promptKey]
	promptCacheMutex.RUnlock()

	// Cache valid check
	if found && hasExpiry && time.Now().Before(expiry) {
		return cachedContent
	}

	// Not in cache or expired, fetch from DB
	var prompt models.SystemPrompt
	err := database.DB.Where("prompt_key = ?", promptKey).First(&prompt).Error
	if err != nil {
		return ""
	}

	// Refresh cache
	promptCacheMutex.Lock()
	promptCache[promptKey] = prompt.Content
	promptCacheTTLs[promptKey] = time.Now().Add(promptCacheDuration)
	promptCacheMutex.Unlock()

	return prompt.Content
}

// ClearPromptCache can be called from admin routes when a prompt is explicitly updated
func ClearPromptCache(promptKey string) {
	promptCacheMutex.Lock()
	defer promptCacheMutex.Unlock()
	delete(promptCache, promptKey)
	delete(promptCacheTTLs, promptKey)
	log.Printf("🔄 Prompt cache cleared for: %s", promptKey)
}
