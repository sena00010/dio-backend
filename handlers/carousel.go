package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// Carousel: kapak (AI görsel) → değer slaytları → CTA (marka renginde metin slaytları)
// ──────────────────────────────────────────────

// loadOwnedPost — post'u ve sahibi olan şirketi getirir; yetki yoksa yanıtı yazıp false döner
func loadOwnedPost(c *gin.Context) (models.Post, models.Company, bool) {
	var post models.Post
	var company models.Company
	userID := c.MustGet("userID").(uint)
	id, _ := strconv.Atoi(c.Param("id"))
	if err := database.DB.First(&post, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post bulunamadı"})
		return post, company, false
	}
	if err := database.DB.Where("id = ? AND user_id = ?", post.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu posta erişim yetkiniz yok"})
		return post, company, false
	}
	return post, company, true
}

// brandPrimaryColor — marka renklerinin ilki, yoksa koyu nötr
func brandPrimaryColor(brandColors string) string {
	var colors []string
	if json.Unmarshal([]byte(brandColors), &colors) == nil {
		for _, c := range colors {
			c = strings.TrimSpace(c)
			if strings.HasPrefix(c, "#") && (len(c) == 7 || len(c) == 4) {
				return c
			}
		}
	}
	return "#1f2937"
}

func listSlides(postID uint) []models.PostSlide {
	var slides []models.PostSlide
	database.DB.Where("post_id = ?", postID).Order("position ASC").Find(&slides)
	return slides
}

type generateCarouselRequest struct {
	SlideCount int `json:"slide_count"`
}

// POST /api/protected/posts/:id/carousel — slaytları (yeniden) üretir
func GenerateCarousel(c *gin.Context) {
	post, company, ok := loadOwnedPost(c)
	if !ok {
		return
	}
	var req generateCarouselRequest
	_ = c.ShouldBindJSON(&req)
	if req.SlideCount < 4 || req.SlideCount > 8 {
		req.SlideCount = 5
	}

	// Metin ve kapak görseli paralel üretilir
	var wg sync.WaitGroup
	var texts []utils.CarouselSlideText
	var textErr, imgErr error
	var coverURL string

	wg.Add(2)
	go func() {
		defer wg.Done()
		texts, textErr = utils.GenerateCarouselText(post.Title, post.Content, post.Platform, buildCompanyContext(company), req.SlideCount)
	}()
	go func() {
		defer wg.Done()
		prompt := post.ImagePrompt
		if prompt == "" {
			prompt = post.Title
		}
		coverURL, imgErr = utils.GenerateBrandImage(utils.BrandContext{
			BrandName: company.Name, Sector: company.Sector, TargetAudience: company.TargetAudience,
			BrandColors: company.BrandColors, ToneOfVoice: company.ToneOfVoice, Slogan: company.Slogan,
			Description: company.Description, Platform: post.Platform, Headline: post.Title,
			ImagePrompt: prompt, Style: "realistic", TextPosition: "bottom_left", AspectRatio: "4:5",
		})
	}()
	wg.Wait()

	if textErr != nil {
		log.Printf("❌ Carousel metni üretilemedi (post %d): %v", post.ID, textErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Carousel metinleri üretilemedi, lütfen tekrar deneyin"})
		return
	}
	if imgErr != nil {
		// Kapak görseli olmadan da marka renginde kapak kullanılabilir
		log.Printf("⚠️ Carousel kapak görseli üretilemedi (post %d): %v", post.ID, imgErr)
	}

	primary := brandPrimaryColor(company.BrandColors)
	slides := make([]models.PostSlide, 0, len(texts))
	for i, t := range texts {
		s := models.PostSlide{PostID: post.ID, Position: i, Role: t.Role, Headline: t.Headline, Body: t.Body, BgColor: primary}
		if t.Role == "cover" {
			s.ImageURL = coverURL
		}
		slides = append(slides, s)
	}

	tx := database.DB.Begin()
	if err := tx.Where("post_id = ?", post.ID).Delete(&models.PostSlide{}).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Slaytlar kaydedilemedi"})
		return
	}
	if err := tx.Create(&slides).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Slaytlar kaydedilemedi"})
		return
	}
	if err := tx.Model(&post).Update("format", "carousel").Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Slaytlar kaydedilemedi"})
		return
	}
	tx.Commit()

	log.Printf("✅ Carousel üretildi: post %d, %d slayt (kapak görseli: %v)", post.ID, len(slides), coverURL != "")
	c.JSON(http.StatusOK, gin.H{"slides": slides, "cover_image_failed": imgErr != nil})
}

// GET /api/protected/posts/:id/carousel
func GetCarousel(c *gin.Context) {
	post, company, ok := loadOwnedPost(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"post":    post,
		"slides":  listSlides(post.ID),
		"company": gin.H{"name": company.Name, "logo_url": company.LogoURL, "brand_colors": company.BrandColors, "brand_font": company.BrandFont},
	})
}

type saveCarouselRequest struct {
	Slides []struct {
		ID          uint   `json:"id"`
		Headline    string `json:"headline"`
		Body        string `json:"body"`
		BgColor     string `json:"bg_color"`
		RenderedURL string `json:"rendered_url"`
	} `json:"slides"`
}

// PUT /api/protected/posts/:id/carousel — düzenlenen metinleri ve dışa aktarılan slayt görsellerini kaydeder
func SaveCarousel(c *gin.Context) {
	post, _, ok := loadOwnedPost(c)
	if !ok {
		return
	}
	var req saveCarouselRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	firstRendered := ""
	allRendered := len(req.Slides) > 0
	for _, s := range req.Slides {
		updates := map[string]interface{}{"headline": s.Headline, "body": s.Body, "bg_color": s.BgColor}
		if s.RenderedURL != "" {
			updates["rendered_url"] = s.RenderedURL
		} else {
			allRendered = false
		}
		// post_id koşulu başka posta ait slaytın değiştirilmesini engeller
		if err := database.DB.Model(&models.PostSlide{}).Where("id = ? AND post_id = ?", s.ID, post.ID).Updates(updates).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Slayt kaydedilemedi"})
			return
		}
		if firstRendered == "" && s.RenderedURL != "" {
			firstRendered = s.RenderedURL
		}
	}

	// Kapak, gönderinin ana görseli olur (takvim, liste ve zamanlama bunu gösterir)
	if firstRendered != "" {
		updates := map[string]interface{}{"image_url": firstRendered}
		if allRendered {
			updates["status"] = "ready"
		}
		database.DB.Model(&post).Updates(updates)
	}

	c.JSON(http.StatusOK, gin.H{"slides": listSlides(post.ID)})
}
