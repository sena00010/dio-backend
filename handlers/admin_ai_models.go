package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// Admin Panel — AI Models Management
// ──────────────────────────────────────────────

// GET /api/admin/ai-models
func AdminListAiModels(c *gin.Context) {
	var aiModels []models.AiModelSetting
	query := database.DB.Order("category ASC, title ASC")

	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}

	if err := query.Find(&aiModels).Error; err != nil {
		log.Printf("❌ AI model listeleme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI Modelleri yüklenemedi"})
		return
	}

	categories := make(map[string][]gin.H)
	for _, m := range aiModels {
		categories[m.Category] = append(categories[m.Category], gin.H{
			"id":          m.ID,
			"setting_key": m.SettingKey,
			"title":       m.Title,
			"description": m.Description,
			"model_name":  m.ModelName,
			"provider":    m.Provider,
			"is_active":   m.IsActive,
			"updated_at":  m.UpdatedAt,
			"updated_by":  m.UpdatedBy,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
		"total":      len(aiModels),
	})
}

// GET /api/admin/ai-models/:key
func AdminGetAiModel(c *gin.Context) {
	key := c.Param("key")

	var aiModel models.AiModelSetting
	if err := database.DB.Where("setting_key = ?", key).First(&aiModel).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "AI Model ayarı bulunamadı: " + key})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ai_model": aiModel,
	})
}

// PUT /api/admin/ai-models/:key
func AdminUpdateAiModel(c *gin.Context) {
	key := c.Param("key")
	adminEmail, _ := c.Get("adminEmail")

	var aiModel models.AiModelSetting
	if err := database.DB.Where("setting_key = ?", key).First(&aiModel).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "AI Model ayarı bulunamadı: " + key})
		return
	}

	var req struct {
		ModelName   *string `json:"model_name"`
		Provider    *string `json:"provider"`
		Title       *string `json:"title"`
		Description *string `json:"description"`
		IsActive    *bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ModelName != nil {
		aiModel.ModelName = *req.ModelName
	}
	if req.Provider != nil {
		aiModel.Provider = *req.Provider
	}
	if req.Title != nil {
		aiModel.Title = *req.Title
	}
	if req.Description != nil {
		aiModel.Description = *req.Description
	}
	if req.IsActive != nil {
		aiModel.IsActive = *req.IsActive
	}

	if email, ok := adminEmail.(string); ok {
		aiModel.UpdatedBy = email
	}

	if err := database.DB.Save(&aiModel).Error; err != nil {
		log.Printf("❌ AI Model güncelleme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI Model güncellenemedi"})
		return
	}

	// Cache'i temizle, ayar anında devreye girsin
	utils.ClearAiModelCache()

	log.Printf("✅ AI Model güncellendi: %s (by: %v)", key, adminEmail)
	c.JSON(http.StatusOK, gin.H{
		"message":  "AI Model başarıyla güncellendi",
		"ai_model": aiModel,
	})
}

// POST /api/admin/ai-models/seed
func AdminSeedAiModels(c *gin.Context) {
	var count int64
	database.DB.Model(&models.AiModelSetting{}).Count(&count)
	if count > 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "AI modelleri zaten yüklenmiş",
			"count":   count,
		})
		return
	}

	seeds := []models.AiModelSetting{
		{
			SettingKey:  "gpt_fast_general",
			Category:    "Genel Kullanım",
			Title:       "Hızlı Genel Metin Üretimi (Fast)",
			Description: "Tema önerileri, ürün copy'si, reklam kopyası ve genel hızlı metin üretimi için kullanılan hızlı model.",
			ModelName:   "gpt-4o-mini",
			Provider:    "openai",
		},
		{
			SettingKey:  "gpt_smart_general",
			Category:    "Genel Kullanım",
			Title:       "Zeki Doğruluk Modeli (Smart)",
			Description: "Tasarım katmanları pozisyonlandırması ve daha zeki kararlar gerektiren kısımlarda kullanılan model.",
			ModelName:   "gpt-4o",
			Provider:    "openai",
		},
		{
			SettingKey:  "gpt_image_edit",
			Category:    "Ürün Görsel Stüdyosu",
			Title:       "Ürün Düzenleme (Image Edit API)",
			Description: "Ürün görsellerinin arka planlarına stüdyo görünümü vs koymak için kullanılan DALL-E tabanlı edit modeli.",
			ModelName:   "dall-e-2",
			Provider:    "openai",
		},
	}

	created := 0
	for _, seed := range seeds {
		if err := database.DB.Create(&seed).Error; err != nil {
			log.Printf("⚠️ Seed ai_model oluşturulamadı (%s): %v", seed.SettingKey, err)
			continue
		}
		created++
	}

	utils.ClearAiModelCache()

	log.Printf("✅ %d prompt seed olarak yüklendi (ai models)", created)
	c.JSON(http.StatusCreated, gin.H{
		"message": "AI modelleri başarıyla yüklendi",
		"created": created,
		"total":   len(seeds),
	})
}
