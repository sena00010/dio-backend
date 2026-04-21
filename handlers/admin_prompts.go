package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// Admin Panel — System Prompts Management
// ──────────────────────────────────────────────

// GET /api/admin/prompts — Tüm promptları listele (sidebar için)
func AdminListPrompts(c *gin.Context) {
	var prompts []models.SystemPrompt
	query := database.DB.Order("category ASC, title ASC")

	// Opsiyonel kategori filtresi
	if category := c.Query("category"); category != "" {
		query = query.Where("category = ?", category)
	}

	if err := query.Find(&prompts).Error; err != nil {
		log.Printf("❌ Prompt listeleme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Promptlar yüklenemedi"})
		return
	}

	// Sidebar için kategori bazlı gruplama
	categories := make(map[string][]gin.H)
	for _, p := range prompts {
		categories[p.Category] = append(categories[p.Category], gin.H{
			"id":          p.ID,
			"prompt_key":  p.PromptKey,
			"title":       p.Title,
			"description": p.Description,
			"content":     p.Content,
			"variables":   p.Variables,
			"is_active":   p.IsActive,
			"ai_provider": p.AiProvider,
			"ai_model":    p.AiModel,
			"updated_at":  p.UpdatedAt,
			"updated_by":  p.UpdatedBy,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
		"total":      len(prompts),
	})
}

// GET /api/admin/prompts/:key — Tek bir prompt'un detayını getir
func AdminGetPrompt(c *gin.Context) {
	key := c.Param("key")

	var prompt models.SystemPrompt
	if err := database.DB.Where("prompt_key = ?", key).First(&prompt).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Prompt bulunamadı: " + key})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"prompt": prompt,
	})
}

// PUT /api/admin/prompts/:key — Prompt'u güncelle
func AdminUpdatePrompt(c *gin.Context) {
	key := c.Param("key")
	adminEmail, _ := c.Get("adminEmail")

	var prompt models.SystemPrompt
	if err := database.DB.Where("prompt_key = ?", key).First(&prompt).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Prompt bulunamadı: " + key})
		return
	}

	var req struct {
		Content     *string `json:"content"`
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Variables   *string `json:"variables"`
		AiProvider  *string `json:"ai_provider"`
		AiModel     *string `json:"ai_model"`
		IsActive    *bool   `json:"is_active"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Partial update
	if req.Content != nil {
		prompt.Content = *req.Content
	}
	if req.Title != nil {
		prompt.Title = *req.Title
	}
	if req.Description != nil {
		prompt.Description = *req.Description
	}
	if req.Variables != nil {
		prompt.Variables = *req.Variables
	}
	if req.AiProvider != nil {
		prompt.AiProvider = *req.AiProvider
	}
	if req.AiModel != nil {
		prompt.AiModel = *req.AiModel
	}
	if req.IsActive != nil {
		prompt.IsActive = *req.IsActive
	}

	// Kim güncelledi
	if email, ok := adminEmail.(string); ok {
		prompt.UpdatedBy = email
	}

	if err := database.DB.Save(&prompt).Error; err != nil {
		log.Printf("❌ Prompt güncelleme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Prompt güncellenemedi"})
		return
	}

	// YENİ: Cache'i temizle, böylece anında yeni prompt yüklenir.
	utils.ClearPromptCache(prompt.PromptKey)

	log.Printf("✅ Prompt güncellendi: %s (by: %v)", key, adminEmail)
	c.JSON(http.StatusOK, gin.H{
		"message": "Prompt başarıyla güncellendi",
		"prompt":  prompt,
	})
}

// GET /api/admin/prompts/categories — Mevcut kategorileri listele
func AdminListCategories(c *gin.Context) {
	var categories []string
	database.DB.Model(&models.SystemPrompt{}).Distinct("category").Pluck("category", &categories)

	c.JSON(http.StatusOK, gin.H{
		"categories": categories,
	})
}

// POST /api/admin/prompts — Yeni prompt oluştur (opsiyonel)
func AdminCreatePrompt(c *gin.Context) {
	adminEmail, _ := c.Get("adminEmail")

	var req struct {
		PromptKey   string `json:"prompt_key" binding:"required"`
		Category    string `json:"category" binding:"required"`
		Title       string `json:"title" binding:"required"`
		Description string `json:"description"`
		Content     string `json:"content" binding:"required"`
		Variables   string `json:"variables"`
		AiProvider  string `json:"ai_provider"`
		AiModel     string `json:"ai_model"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Aynı key var mı kontrol
	var existing models.SystemPrompt
	if err := database.DB.Where("prompt_key = ?", req.PromptKey).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Bu prompt_key zaten mevcut: " + req.PromptKey})
		return
	}

	updatedBy := ""
	if email, ok := adminEmail.(string); ok {
		updatedBy = email
	}

	prompt := models.SystemPrompt{
		PromptKey:   req.PromptKey,
		Category:    req.Category,
		Title:       req.Title,
		Description: req.Description,
		Content:     req.Content,
		Variables:   req.Variables,
		AiProvider:  req.AiProvider,
		AiModel:     req.AiModel,
		IsActive:    true,
		UpdatedBy:   updatedBy,
	}

	if err := database.DB.Create(&prompt).Error; err != nil {
		log.Printf("❌ Prompt oluşturma hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Prompt oluşturulamadı"})
		return
	}

	log.Printf("✅ Yeni prompt oluşturuldu: %s (by: %v)", req.PromptKey, adminEmail)
	c.JSON(http.StatusCreated, gin.H{
		"message": "Prompt başarıyla oluşturuldu",
		"prompt":  prompt,
	})
}

// POST /api/admin/prompts/translate — Promptu İngilizceye çevir
func AdminTranslatePrompt(c *gin.Context) {
	var req struct {
		Content string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "OPENAI_API_KEY bulunamadı"})
		return
	}

	systemPrompt := `You are an expert translator specializing in technical marketing AI prompts. Translate the following text from Turkish to English. 
CRITICAL RULES:
1. Do NOT translate any curly brace variables like {{.variableName}}. Keep them EXACTLY the same.
2. Maintain the tone and instructional nature of the text.
3. Only output the translated text, no conversational padding.`

	reqBody := map[string]interface{}{
		"model":       utils.GetAiModel("gpt_fast_general", "gpt-4o-mini"),
		"temperature": 0.3,
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": req.Content},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Çeviri servisine ulaşılamadı"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("OpenAI API error: %d", resp.StatusCode)})
		return
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Çeviri sonucu ayrıştırılamadı"})
		return
	}

	if len(result.Choices) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Çeviri sonucu boş"})
		return
	}

	translated := strings.TrimSpace(result.Choices[0].Message.Content)
	c.JSON(http.StatusOK, gin.H{"translated_content": translated})
}

