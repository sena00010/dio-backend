package utils

import (
	"log"
	"sync"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

var (
	aiModelCache   = make(map[string]string)
	aiModelCacheMu sync.RWMutex
	lastCacheTime  time.Time
)

const cacheTTL = 1 * time.Minute

// GetAiModel fetches the model name for a specific key.
// Performs an in-memory caching with TTL to avoid hitting the database for every single generation request.
func GetAiModel(key string, defaultModel string) string {
	now := time.Now()

	// TTL bazlı önbellekten oku
	aiModelCacheMu.RLock()
	val, exists := aiModelCache[key]
	isFresh := now.Sub(lastCacheTime) < cacheTTL
	aiModelCacheMu.RUnlock()

	if exists && isFresh {
		return val
	}

	// Veritabanından çek ("ai_model_settings" tablosu)
	var setting models.AiModelSetting
	// Veritabanı başlatılmamış olabilir (init sırasında vb.)
	if database.DB == nil {
		return defaultModel
	}

	if err := database.DB.Where("setting_key = ? AND is_active = ?", key, true).First(&setting).Error; err != nil {
		// Bulunamazsa varsayılanı dön ve kısa bir süre bunu da cache'le (sorgu yükünü azaltır)
		cacheModelSetting(key, defaultModel)
		return defaultModel
	}

	// Bulunursa değeri cache'le ve döndür
	cacheModelSetting(key, setting.ModelName)
	return setting.ModelName
}

func cacheModelSetting(key, val string) {
	aiModelCacheMu.Lock()
	defer aiModelCacheMu.Unlock()
	aiModelCache[key] = val
	// Last cache time update'i (sadece yeni eklenince bile genel olarak listeyi taze kabul edelim)
	lastCacheTime = time.Now()
}

// ClearAiModelCache is used when an admin updates a setting in the admin panel to immediately reflect the change
func ClearAiModelCache() {
	aiModelCacheMu.Lock()
	defer aiModelCacheMu.Unlock()
	aiModelCache = make(map[string]string)
	lastCacheTime = time.Time{}
	log.Println("🔄 AI Model Cache temizlendi")
}
