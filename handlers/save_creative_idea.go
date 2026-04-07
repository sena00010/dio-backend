package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ──────────────────────────────────────────────
// POST /api/protected/save-creative-idea
// Tek bir kreatif varyantı ai_ideas tablosuna kaydet
// ──────────────────────────────────────────────

type SaveCreativeIdeaRequest struct {
	CompanyID         uint     `json:"company_id" binding:"required"`
	Topic             string   `json:"topic"`
	Platform          string   `json:"platform"`
	CampaignType      string   `json:"campaign_type"`
	VariantName       string   `json:"variant_name"`
	Headline          string   `json:"headline"`
	SubHeadline       string   `json:"sub_headline"`
	Caption           string   `json:"caption"`
	CTA               string   `json:"cta"`
	Hashtags          []string `json:"hashtags"`
	Format            string   `json:"format"`
	VisualDescription string   `json:"visual_description"`
	ColorPalette      []string `json:"color_palette"`
	DesignNotes       string   `json:"design_notes"`
	AIScore           int      `json:"ai_score"`
	ImageURL          string   `json:"image_url"`
}

func SaveCreativeIdea(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req SaveCreativeIdeaRequest
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

	// Hashtag birleştir
	hashtagStr := ""
	if len(req.Hashtags) > 0 {
		cleanTags := make([]string, 0, len(req.Hashtags))
		for _, h := range req.Hashtags {
			h = strings.TrimSpace(h)
			if h == "" { continue }
			if !strings.HasPrefix(h, "#") { h = "#" + h }
			cleanTags = append(cleanTags, h)
		}
		hashtagStr = strings.Join(cleanTags, " ")
	}

	// Color palette JSON
	colorJSON, _ := json.Marshal(req.ColorPalette)

	idea := models.AiIdea{
		CompanyID:             req.CompanyID,
		UserInputTopic:        req.Topic,
		Platform:              req.Platform,
		CampaignType:          req.CampaignType,
		GeneratedTitle:        req.Headline,
		GeneratedDescription:  req.Caption,
		GeneratedHashtags:     hashtagStr,
		ImageSuggestionPrompt: req.VisualDescription,
		SuggestedLayout:       req.Format,
		SuggestedFont:         "",
		ContentItems:          string(colorJSON),
		DesignLayers:          req.DesignNotes,
		ImageURL:              req.ImageURL,
		IsSelected:            false,
	}

	if err := database.DB.Create(&idea).Error; err != nil {
		log.Printf("❌ Kreatif fikir kaydetme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Kaydedilemedi: " + err.Error()})
		return
	}

	// ── Post tablosuna da "draft" olarak ekle ──
	post := models.Post{
		CompanyID:    req.CompanyID,
		Title:        req.Headline,
		Content:      req.Caption,
		Platform:     req.Platform,
		Hashtags:     hashtagStr,
		ImagePrompt:  req.VisualDescription,
		ImageURL:     req.ImageURL,
		ContentItems: string(colorJSON),
		DesignLayers: req.DesignNotes,
		Status:       "draft",
	}
	var postID uint
	if err := database.DB.Create(&post).Error; err != nil {
		log.Printf("⚠️ Post taslağı oluşturma hatası: %v", err)
	} else {
		postID = post.ID
		log.Printf("📝 Post taslağı oluşturuldu: post_id=%d, idea_id=%d", post.ID, idea.ID)
	}

	log.Printf("✅ Kreatif fikir kaydedildi: id=%d, company=%d, headline=%s", idea.ID, req.CompanyID, req.Headline)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"idea_id": idea.ID,
		"post_id": postID,
		"message": "Kreatif fikir kaydedildi",
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/save-creative-ideas-batch
// Tüm kreatif varyantları toplu olarak ai_ideas tablosuna kaydet (Taslak Kaydet)
// ──────────────────────────────────────────────

type SaveCreativeIdeasBatchRequest struct {
	CompanyID    uint                      `json:"company_id" binding:"required"`
	Topic        string                    `json:"topic"`
	Platform     string                    `json:"platform"`
	CampaignType string                    `json:"campaign_type"`
	Variants     []SaveCreativeIdeaVariant `json:"variants" binding:"required"`
}

type SaveCreativeIdeaVariant struct {
	VariantName       string   `json:"variant_name"`
	Headline          string   `json:"headline"`
	SubHeadline       string   `json:"sub_headline"`
	Caption           string   `json:"caption"`
	CTA               string   `json:"cta"`
	Hashtags          []string `json:"hashtags"`
	Format            string   `json:"format"`
	VisualDescription string   `json:"visual_description"`
	ColorPalette      []string `json:"color_palette"`
	DesignNotes       string   `json:"design_notes"`
	AIScore           int      `json:"ai_score"`
	ImageURL          string   `json:"image_url"`
}

func SaveCreativeIdeasBatch(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req SaveCreativeIdeasBatchRequest
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

	var savedCount int
	var savedIds []uint
	var postIds []uint
	for _, v := range req.Variants {
		// Hashtag birleştir
		hashtagStr := ""
		if len(v.Hashtags) > 0 {
			cleanTags := make([]string, 0, len(v.Hashtags))
			for _, h := range v.Hashtags {
				h = strings.TrimSpace(h)
				if h == "" { continue }
				if !strings.HasPrefix(h, "#") { h = "#" + h }
				cleanTags = append(cleanTags, h)
			}
			hashtagStr = strings.Join(cleanTags, " ")
		}

		colorJSON, _ := json.Marshal(v.ColorPalette)

		idea := models.AiIdea{
			CompanyID:             req.CompanyID,
			UserInputTopic:        req.Topic,
			Platform:              req.Platform,
			CampaignType:          req.CampaignType,
			GeneratedTitle:        v.Headline,
			GeneratedDescription:  v.Caption,
			GeneratedHashtags:     hashtagStr,
			ImageSuggestionPrompt: v.VisualDescription,
			SuggestedLayout:       v.Format,
			SuggestedFont:         "",
			ContentItems:          string(colorJSON),
			DesignLayers:          v.DesignNotes,
			ImageURL:              v.ImageURL,
			IsSelected:            false,
		}

		if err := database.DB.Create(&idea).Error; err != nil {
			log.Printf("⚠️ Varyant kaydetme hatası (%s): %v", v.Headline, err)
			continue
		}
		savedCount++
		savedIds = append(savedIds, idea.ID)

		// ── Aynı zamanda Post tablosuna da "draft" olarak ekle ──
		// Böylece İçerik Yönetimi ve Zamanlama sayfalarında görünür
		post := models.Post{
			CompanyID:    req.CompanyID,
			Title:        v.Headline,
			Content:      v.Caption,
			Platform:     req.Platform,
			Hashtags:     hashtagStr,
			ImagePrompt:  v.VisualDescription,
			ImageURL:     v.ImageURL,
			ContentItems: string(colorJSON),
			DesignLayers: v.DesignNotes,
			Status:       "draft",
		}
		if err := database.DB.Create(&post).Error; err != nil {
			log.Printf("⚠️ Post oluşturma hatası (%s): %v", v.Headline, err)
		} else {
			postIds = append(postIds, post.ID)
			log.Printf("📝 Post taslağı oluşturuldu: post_id=%d, idea_id=%d", post.ID, idea.ID)
		}
	}

	log.Printf("✅ Toplu kreatif kayıt: %d/%d kaydedildi (company=%d, posts=%d)", savedCount, len(req.Variants), req.CompanyID, len(postIds))
	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"saved_count": savedCount,
		"total":       len(req.Variants),
		"idea_ids":    savedIds,
		"post_ids":    postIds,
		"message":     "Kreatif taslak kaydedildi",
	})
}
