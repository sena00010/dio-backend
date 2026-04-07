package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// POST /api/onboarding/research
// Web sitesini AI ile tarayıp marka bilgilerini çıkart
// ──────────────────────────────────────────────

type BrandResearchRequest struct {
	WebURL       string `json:"web_url"`
	SocialHandle string `json:"social_handle"`
}

func BrandResearch(c *gin.Context) {
	var req BrandResearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek formatı"})
		return
	}

	if req.WebURL == "" && req.SocialHandle == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "En az bir kaynak belirtmelisiniz (web sitesi veya sosyal medya)"})
		return
	}

	searchURL := req.WebURL
	if searchURL == "" {
		searchURL = "https://www.instagram.com/" + req.SocialHandle
	}

	log.Printf("🔍 Marka araştırma başlatıldı: %s", searchURL)

	// AI araştırma, logo çekme ve renk çekme paralel çalışsın
	type aiResult struct {
		data *utils.BrandResearchResult
		err  error
	}
	type logoResult struct {
		url string
		err error
	}

	aiCh := make(chan aiResult, 1)
	logoCh := make(chan logoResult, 1)
	colorCh := make(chan []string, 1)

	// AI araştırma goroutine
	go func() {
		result, err := utils.ResearchBrandFromWebsite(searchURL)
		aiCh <- aiResult{data: result, err: err}
	}()

	// Logo URL bulma goroutine (indirmez, sadece URL bulur)
	go func() {
		url, err := utils.ExtractLogoURL(searchURL)
		logoCh <- logoResult{url: url, err: err}
	}()

	// Renk çekme goroutine (CSS/inline style'lardan)
	go func() {
		colors := utils.ExtractBrandColors(searchURL)
		colorCh <- colors
	}()

	// Sonuçları bekle
	aiRes := <-aiCh
	logoRes := <-logoCh
	scrapedColors := <-colorCh

	if aiRes.err != nil {
		log.Printf("❌ Marka araştırma hatası: %v", aiRes.err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Marka araştırma başarısız oldu: " + aiRes.err.Error(),
		})
		return
	}

	result := aiRes.data

	// Logo başarılıysa result'a ekle
	if logoRes.err != nil {
		log.Printf("⚠️ Logo çekme başarısız (kullanıcı yükleyecek): %v", logoRes.err)
	} else {
		result.LogoURL = logoRes.url
		log.Printf("✅ Logo otomatik çekildi: %s", logoRes.url)
	}

	// Web scraping'den renkler geldiyse, AI'ın renklerini override et
	if len(scrapedColors) > 0 {
		result.BrandColors = scrapedColors
		log.Printf("🎨 Renkler web scraping'den alındı: %v", scrapedColors)
	} else if len(result.BrandColors) == 0 {
		log.Printf("⚠️ Renk bulunamadı — varsayılan kalacak")
	}

	log.Printf("✅ Marka araştırma tamamlandı: %s (sektör: %s, renkler: %v)", result.Name, result.Sector, result.BrandColors)

	c.JSON(http.StatusOK, gin.H{
		"message": "Marka bilgileri başarıyla çıkartıldı",
		"result":  result,
	})
}
