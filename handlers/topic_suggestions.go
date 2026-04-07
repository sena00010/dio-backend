package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// POST /api/protected/topic-suggestions
// AI-driven creative topic suggestions for a brand
// ──────────────────────────────────────────────

type TopicSuggestionsRequest struct {
	CompanyID uint `json:"company_id" binding:"required"`
}

func GenerateTopicSuggestions(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req TopicSuggestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Kullanıcının bu şirkete erişimi var mı kontrol et
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	// AI'dan konu önerileri üret
	suggestions, err := utils.GenerateTopicSuggestions(company)
	if err != nil {
		log.Printf("❌ Konu önerisi üretme hatası (company_id: %d): %v", company.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI konu önerisi üretme başarısız: " + err.Error()})
		return
	}

	log.Printf("✅ %d konu önerisi üretildi (company: %s, sector: %s)", len(suggestions), company.Name, company.Sector)
	c.JSON(http.StatusOK, gin.H{
		"suggestions": suggestions,
		"company":     company.Name,
		"generated_at": time.Now().Format("2006-01-02T15:04:05Z07:00"),
	})
}

// ──────────────────────────────────────────────
// GET /api/protected/topic-suggestions/cached
// Cached / pre-generated suggestions (for DB-backed version)
// ──────────────────────────────────────────────

type AiTopicSuggestion struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	CompanyID        uint      `gorm:"index;not null" json:"company_id"`
	TopicTitle       string    `gorm:"size:500" json:"topic_title"`
	Category         string    `gorm:"size:50" json:"category"`
	Score            int       `json:"score"`
	AiReasoning      string    `gorm:"type:text" json:"ai_reasoning"`
	TrendData        string    `gorm:"size:255" json:"trend_data"`
	SuggestedFormats string    `gorm:"type:text" json:"suggested_formats"`
	UrgencyTag       string    `gorm:"size:100" json:"urgency_tag,omitempty"`
	DaysLeft         *int      `json:"days_left,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// ──────────────────────────────────────────────
// Trend Radar & Competitor Gaps — AI-generated
// ──────────────────────────────────────────────

func GenerateTrendRadar(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req TopicSuggestionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	trendData, err := utils.GenerateTrendRadar(company)
	if err != nil {
		log.Printf("❌ Trend radar üretme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Trend verileri üretilemedi"})
		return
	}

	// Parse the JSON response
	var result json.RawMessage
	if err := json.Unmarshal([]byte(trendData), &result); err != nil {
		// If parsing fails, wrap in a simple structure
		c.JSON(http.StatusOK, gin.H{
			"raw": trendData,
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ──────────────────────────────────────────────
// Yardımcı: Marka bağlamı oluştur (konu önerileri için)
// ──────────────────────────────────────────────

func buildTopicContext(company models.Company) string {
	ctx := fmt.Sprintf(`MARKA PROFİLİ:
- Marka Adı: %s
- Sektör: %s`, company.Name, company.Sector)

	if company.Description != "" {
		ctx += fmt.Sprintf("\n- Marka Açıklaması: %s", company.Description)
	}
	if company.TargetAudience != "" {
		ctx += fmt.Sprintf("\n- Hedef Kitle: %s", company.TargetAudience)
	}
	if company.ProductCategories != "" {
		ctx += fmt.Sprintf("\n- Ürün/Hizmet Kategorileri: %s", company.ProductCategories)
	}
	if company.CompetitiveEdge != "" {
		ctx += fmt.Sprintf("\n- Rekabet Avantajı: %s", company.CompetitiveEdge)
	}
	if company.Slogan != "" {
		ctx += fmt.Sprintf("\n- Slogan: %s", company.Slogan)
	}
	if company.ToneOfVoice != "" {
		ctx += fmt.Sprintf("\n- Marka Tonu: %s", company.ToneOfVoice)
	}
	if company.Goal != "" {
		ctx += fmt.Sprintf("\n- İş Hedefi: %s", company.Goal)
	}
	if company.Platforms != "" {
		ctx += fmt.Sprintf("\n- Aktif Platformlar: %s", company.Platforms)
	}
	if company.Website != "" {
		ctx += fmt.Sprintf("\n- Website: %s", company.Website)
	}
	if company.ContentLanguage != "" {
		ctx += fmt.Sprintf("\n- İçerik Dili: %s", company.ContentLanguage)
	}
	if company.BrandType != "" {
		ctx += fmt.Sprintf("\n- Marka Tipi: %s", company.BrandType)
	}

	return ctx
}
