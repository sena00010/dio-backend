package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// POST /api/protected/generate-creatives
// ──────────────────────────────────────────────

type GenerateCreativesRequest struct {
	CompanyID    uint     `json:"company_id" binding:"required"`
	Topics       []string `json:"topics" binding:"required"`
	Emphasis     string   `json:"emphasis"`      // yenilik, fiyat, kalite, sosyal_kanit
	Platforms    []string `json:"platforms"`      // instagram, tiktok, facebook
	Formats      []string `json:"formats"`        // 1:1 Feed, 9:16 Story, 4:5 Feed, Carousel, Reels
	VariantCount int      `json:"variant_count"`  // 1-12
}

func GenerateCreatives(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req GenerateCreativesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Defaults
	if req.VariantCount < 1 || req.VariantCount > 12 {
		req.VariantCount = 6
	}
	if req.Emphasis == "" {
		req.Emphasis = "yenilik"
	}
	if len(req.Platforms) == 0 {
		req.Platforms = []string{"instagram"}
	}
	if len(req.Formats) == 0 {
		req.Formats = []string{"1:1 Feed", "9:16 Story"}
	}

	// Şirket erişim kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	// AI'dan kreatif varyantlar üret
	variants, err := utils.GenerateCreativeVariants(company, utils.CreativeConfig{
		Topics:       req.Topics,
		Emphasis:     req.Emphasis,
		Platforms:    req.Platforms,
		Formats:      req.Formats,
		VariantCount: req.VariantCount,
	})
	if err != nil {
		log.Printf("❌ Kreatif üretme hatası (company_id: %d): %v", company.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI kreatif üretme başarısız: " + err.Error()})
		return
	}

	log.Printf("✅ %d kreatif varyant üretildi (company: %s, emphasis: %s)", len(variants), company.Name, req.Emphasis)

	// Response'u JSON olarak parse et
	c.JSON(http.StatusOK, gin.H{
		"variants":      variants,
		"variant_count": len(variants),
		"company":       company.Name,
		"emphasis":      req.Emphasis,
		"topics":        req.Topics,
	})
}

// ──────────────────────────────────────────────
// Marka bağlamını oluştur (kreatif üretimi için)
// ──────────────────────────────────────────────

func BuildCreativeCompanyContext(company models.Company) string {
	ctx := fmt.Sprintf(`MARKA KİMLİĞİ:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🏢 Marka: %s
🏭 Sektör: %s`, company.Name, company.Sector)

	if company.Description != "" {
		ctx += fmt.Sprintf("\n📝 Açıklama: %s", company.Description)
	}
	if company.TargetAudience != "" {
		ctx += fmt.Sprintf("\n🎯 Hedef Kitle: %s", company.TargetAudience)
	}
	if company.ProductCategories != "" {
		ctx += fmt.Sprintf("\n📦 Ürünler: %s", company.ProductCategories)
	}
	if company.CompetitiveEdge != "" {
		ctx += fmt.Sprintf("\n⚔️ Rekabet Avantajı: %s", company.CompetitiveEdge)
	}
	if company.Slogan != "" {
		ctx += fmt.Sprintf("\n💬 Slogan: %s", company.Slogan)
	}
	if company.ToneOfVoice != "" {
		ctx += fmt.Sprintf("\n🎤 Ton: %s", company.ToneOfVoice)
	}
	if company.BrandColors != "" {
		ctx += fmt.Sprintf("\n🎨 Marka Renkleri: %s", company.BrandColors)
	}
	if company.Website != "" {
		ctx += fmt.Sprintf("\n🌐 Website: %s", company.Website)
	}
	if company.ContentLanguage != "" {
		ctx += fmt.Sprintf("\n🌍 Dil: %s", company.ContentLanguage)
	}
	if company.ForbiddenWords != "" {
		ctx += fmt.Sprintf("\n🚫 Yasak Kelimeler: %s", company.ForbiddenWords)
	}
	if company.SocialMedia != "" {
		ctx += fmt.Sprintf("\n📱 Sosyal Medya: %s", company.SocialMedia)
	}

	// Marka renkleri verisini JSON olarak parse et
	if company.BrandColors != "" {
		ctx += fmt.Sprintf("\n\n🎨 RENK PALETİ (Tasarımlarda kullanılmalı): %s", company.BrandColors)
	}

	return ctx
}

// ──────────────────────────────────────────────
// Yardımcı: AI yanıtını JSON olarak parse et
// ──────────────────────────────────────────────

func ParseCreativeVariants(raw string) ([]json.RawMessage, error) {
	var variants []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &variants); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %v", err)
	}
	return variants, nil
}
