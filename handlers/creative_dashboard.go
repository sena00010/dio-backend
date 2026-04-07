package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// In-memory cache for creative dashboard data
// ──────────────────────────────────────────────

type dashboardCacheEntry struct {
	Topics        []utils.TopicSuggestion `json:"topics"`
	TrendRadar    json.RawMessage         `json:"trend_radar"`
	CompanyName   string                  `json:"company_name"`
	GeneratedAt   string                  `json:"generated_at"`
	CachedAt      time.Time               `json:"-"`
}

var (
	dashboardCache     = make(map[uint]*dashboardCacheEntry)
	dashboardCacheLock sync.RWMutex
	cacheTTL           = 15 * time.Minute // Cache 15 dakika geçerli
)

func getCachedDashboard(companyID uint) *dashboardCacheEntry {
	dashboardCacheLock.RLock()
	defer dashboardCacheLock.RUnlock()
	entry, ok := dashboardCache[companyID]
	if !ok {
		return nil
	}
	if time.Since(entry.CachedAt) > cacheTTL {
		return nil // Expired
	}
	return entry
}

func setCachedDashboard(companyID uint, entry *dashboardCacheEntry) {
	dashboardCacheLock.Lock()
	defer dashboardCacheLock.Unlock()
	entry.CachedAt = time.Now()
	dashboardCache[companyID] = entry
}

// ──────────────────────────────────────────────
// POST /api/protected/creative-dashboard
// Combined endpoint: topics + trends in parallel
// ──────────────────────────────────────────────

func CreativeDashboard(c *gin.Context) {
	start := time.Now()

	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req struct {
		CompanyID uint `json:"company_id" binding:"required"`
		Force     bool `json:"force"` // Cache'i bypass et
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Şirket erişimi kontrol et
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	// Cache kontrol — force değilse cache'den dön
	if !req.Force {
		if cached := getCachedDashboard(req.CompanyID); cached != nil {
			log.Printf("⚡ Cache hit — creative dashboard (company: %s, %dms)", company.Name, time.Since(start).Milliseconds())
			c.JSON(http.StatusOK, gin.H{
				"suggestions":    cached.Topics,
				"trend_data":     cached.TrendRadar,
				"company":        cached.CompanyName,
				"generated_at":   cached.GeneratedAt,
				"cached":         true,
				"response_ms":    time.Since(start).Milliseconds(),
			})
			return
		}
	}

	// ── Paralel AI çağrıları ──
	var (
		topics     []utils.TopicSuggestion
		trendRaw   string
		topicsErr  error
		trendsErr  error
		wg         sync.WaitGroup
	)

	wg.Add(2)

	// Goroutine 1: Konu önerileri
	go func() {
		defer wg.Done()
		topics, topicsErr = utils.GenerateTopicSuggestions(company)
		if topicsErr != nil {
			log.Printf("⚠️ Konu önerileri hatası: %v", topicsErr)
		}
	}()

	// Goroutine 2: Trend radar
	go func() {
		defer wg.Done()
		trendRaw, trendsErr = utils.GenerateTrendRadar(company)
		if trendsErr != nil {
			log.Printf("⚠️ Trend radar hatası: %v", trendsErr)
		}
	}()

	wg.Wait()

	elapsed := time.Since(start)
	log.Printf("✅ Creative dashboard hazır — %dms (company: %s)", elapsed.Milliseconds(), company.Name)

	// Trend verisini parse et
	var trendData json.RawMessage
	if trendRaw != "" {
		if err := json.Unmarshal([]byte(trendRaw), &trendData); err != nil {
			trendData = json.RawMessage(`{"trend_radar":[],"competitor_gaps":[]}`)
		}
	} else {
		trendData = json.RawMessage(`{"trend_radar":[],"competitor_gaps":[]}`)
	}

	// Hata durumu — en az biri başarılı olmalı
	if topicsErr != nil && trendsErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "AI verileri üretilemedi",
		})
		return
	}

	// Başarılı — cache'e yaz
	generatedAt := time.Now().Format("2006-01-02T15:04:05Z07:00")
	if topics == nil {
		topics = []utils.TopicSuggestion{}
	}

	setCachedDashboard(req.CompanyID, &dashboardCacheEntry{
		Topics:      topics,
		TrendRadar:  trendData,
		CompanyName: company.Name,
		GeneratedAt: generatedAt,
	})

	c.JSON(http.StatusOK, gin.H{
		"suggestions":  topics,
		"trend_data":   trendData,
		"company":      company.Name,
		"generated_at": generatedAt,
		"cached":       false,
		"response_ms":  elapsed.Milliseconds(),
	})
}
