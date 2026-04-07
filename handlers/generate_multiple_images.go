package handlers

import (
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// POST /api/protected/generate-multiple-images
// Tek bir post için birden fazla görsel üretir
// ──────────────────────────────────────────────

type GenerateMultipleImagesRequest struct {
	PostID uint   `json:"post_id" binding:"required"`
	Count  int    `json:"count"` // 1-4 arası
	Style  string `json:"style"` // realistic, illustration, 3d-render, minimalist
}

type GeneratedImageResult struct {
	URL  string `json:"url"`
	Seed int    `json:"seed"`
}

func GenerateMultipleImages(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req GenerateMultipleImagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Defaults & limits
	if req.Count < 1 || req.Count > 4 {
		req.Count = 2
	}
	if req.Style == "" {
		req.Style = "realistic"
	}

	// Post'u bul
	var post models.Post
	if err := database.DB.First(&post, req.PostID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post bulunamadı"})
		return
	}

	// Şirket sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", post.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu posta erişim yetkiniz yok"})
		return
	}

	imagePrompt := post.ImagePrompt
	if imagePrompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Görsel prompt'u bulunamadı"})
		return
	}

	// Layout
	layout := post.SuggestedLayout
	if layout == "" {
		layout = "bottom_left"
	}

	// Her görsel için farklı seed ile marka bağlamlı üretim
	var wg sync.WaitGroup
	type imageResult struct {
		index int
		url   string
		seed  int
		err   error
	}
	results := make([]imageResult, req.Count)

	for i := 0; i < req.Count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			seed := rand.Intn(999999) + idx*1000

			// Her görsel için hafif farklılık ekle
			promptVariation := imagePrompt
			variations := []string{
				"", // orijinal prompt
				" — Use slightly different lighting angle and color warmth for visual variety.",
				" — Try a different camera angle or perspective while keeping the same theme.",
				" — Add slightly different ambient elements while maintaining the core subject.",
			}
			if idx < len(variations) {
				promptVariation += variations[idx]
			}

			brandCtx := utils.BrandContext{
				BrandName:      company.Name,
				Sector:         company.Sector,
				TargetAudience: company.TargetAudience,
				BrandColors:    company.BrandColors,
				ToneOfVoice:    company.ToneOfVoice,
				LogoURL:        company.LogoURL,
				Slogan:         company.Slogan,
				Description:    company.Description,
				BrandFont:      company.BrandFont,
				Platform:       post.Platform,
				Headline:       post.Title,
				SubText:        post.Content,
				ImagePrompt:    promptVariation,
				Style:          req.Style,
				TextPosition:   layout,
			}

			localURL, err := utils.GenerateBrandImage(brandCtx)
			results[idx] = imageResult{index: idx, url: localURL, seed: seed, err: err}
		}(i)

		// Rate-limit: görselleri 2 saniye arayla başlat (API throttling)
		if i < req.Count-1 {
			time.Sleep(2 * time.Second)
		}
	}

	wg.Wait()

	// Sonuçları topla
	var images []GeneratedImageResult
	var firstURL string
	for _, r := range results {
		if r.err != nil {
			log.Printf("⚠️ Görsel %d üretilemedi: %v", r.index+1, r.err)
			continue
		}
		images = append(images, GeneratedImageResult{
			URL:  r.url,
			Seed: r.seed,
		})
		if firstURL == "" {
			firstURL = r.url
		}
	}

	if len(images) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Hiçbir görsel üretilemedi"})
		return
	}

	// İlk görseli post'a kaydet (varsayılan seçim)
	post.ImageURL = firstURL
	database.DB.Save(&post)

	log.Printf("✅ %d/%d görsel başarıyla üretildi (post_id: %d)", len(images), req.Count, post.ID)

	c.JSON(http.StatusOK, gin.H{
		"images":  images,
		"count":   len(images),
		"post_id": post.ID,
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/select-image
// Kullanıcı çoklu görselden birini seçtiğinde,
// post'un image_url'ini günceller
// ──────────────────────────────────────────────

type SelectImageRequest struct {
	PostID   uint   `json:"post_id" binding:"required"`
	ImageURL string `json:"image_url" binding:"required"`
}

func SelectImage(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req SelectImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var post models.Post
	if err := database.DB.First(&post, req.PostID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post bulunamadı"})
		return
	}

	// Sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", post.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Erişim yetkiniz yok"})
		return
	}

	post.ImageURL = req.ImageURL
	if err := database.DB.Save(&post).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Görsel kaydedilemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"post_id": post.ID,
		"message": "Görsel seçimi kaydedildi",
	})
}
