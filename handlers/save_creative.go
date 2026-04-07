package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ──────────────────────────────────────────────
// POST /api/protected/save-creative
// Kreatif varyantı Post olarak kaydet (draft)
// ──────────────────────────────────────────────

type SaveCreativeRequest struct {
	CompanyID         uint     `json:"company_id" binding:"required"`
	VariantName       string   `json:"variant_name"`
	Headline          string   `json:"headline"`
	SubHeadline       string   `json:"sub_headline"`
	Caption           string   `json:"caption"`
	CTA               string   `json:"cta"`
	Hashtags          []string `json:"hashtags"`
	Format            string   `json:"format"`
	Platform          string   `json:"platform"`
	VisualDescription string   `json:"visual_description"`
	ColorPalette      []string `json:"color_palette"`
	DesignNotes       string        `json:"design_notes"`
	AIScore           int           `json:"ai_score"`
	ImageURL          string        `json:"image_url"`
	DesignLayers      []interface{} `json:"design_layers"` // AI Vision ile belirlenen metin katmanları
}

func SaveCreative(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req SaveCreativeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
		return
	}

	// Company erişim kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu markaya erişiminiz yok"})
		return
	}

	// Hashtag'leri birleştir
	hashtagStr := ""
	if len(req.Hashtags) > 0 {
		cleanTags := make([]string, 0, len(req.Hashtags))
		for _, h := range req.Hashtags {
			h = strings.TrimSpace(h)
			if h == "" {
				continue
			}
			if !strings.HasPrefix(h, "#") {
				h = "#" + h
			}
			cleanTags = append(cleanTags, h)
		}
		hashtagStr = strings.Join(cleanTags, " ")
	}

	// Post olarak kaydet — draft status
	post := models.Post{
		CompanyID:       req.CompanyID,
		Title:           req.Headline,
		Content:         req.Caption,
		Platform:        req.Platform,
		Hashtags:        hashtagStr,
		ImagePrompt:     req.VisualDescription,
		ImageURL:        req.ImageURL,
		SuggestedLayout: req.Format,
		Status:          "draft",
	}

	// AI Vision design layers varsa ekle
	if len(req.DesignLayers) > 0 {
		layersJSON, err := json.Marshal(req.DesignLayers)
		if err == nil {
			post.DesignLayers = string(layersJSON)
		}
	}

	if err := database.DB.Create(&post).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Kaydedilemedi: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"post_id": post.ID,
		"message": "Kreatif taslak olarak kaydedildi",
	})
}
