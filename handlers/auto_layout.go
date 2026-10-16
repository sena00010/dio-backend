package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

type autoLayoutRequest struct {
	ImageURL    string `json:"image_url"`
	AspectRatio string `json:"aspect_ratio"`
	Headline    string `json:"headline"`
	Description string `json:"description"`
	CTA         string `json:"cta"`
}

// POST /api/protected/posts/:id/auto-layout
// Görsele bakarak başlık / açıklama / CTA katmanlarını ve logo konumunu önerir (kaydetmez; stüdyo kaydeder).
func AutoLayoutPost(c *gin.Context) {
	post, company, ok := loadOwnedPost(c)
	if !ok {
		return
	}
	var req autoLayoutRequest
	_ = c.ShouldBindJSON(&req)
	if req.ImageURL == "" {
		req.ImageURL = post.ImageURL
	}
	if req.ImageURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Önce bir görsel seçin"})
		return
	}
	if req.Headline == "" {
		req.Headline = post.Title
	}
	if req.AspectRatio == "" {
		req.AspectRatio = utils.PlatformAspectRatio(post.Platform)
	}

	res, err := utils.AutoLayout(utils.AutoLayoutInput{
		ImageURL:    req.ImageURL,
		AspectRatio: req.AspectRatio,
		Headline:    req.Headline,
		Description: req.Description,
		CTA:         req.CTA,
		BrandColors: company.BrandColors,
		BrandFont:   company.BrandFont,
	})
	if err != nil {
		log.Printf("❌ AI yerleşim (post %d): %v", post.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI yerleşimi şu an yapılamadı, lütfen tekrar deneyin"})
		return
	}
	log.Printf("✨ AI yerleşim: post %d, %d katman", post.ID, len(res.Layers))
	c.JSON(http.StatusOK, res)
}
