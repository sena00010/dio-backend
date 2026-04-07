package handlers

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// POST /api/protected/migrate-ideas-to-posts
// Eski ai_ideas kayıtlarını posts tablosuna "draft" olarak kopyalar
// Sadece henüz Post'a dönüştürülmemiş olanları işler (başlık eşleşmesine göre)
func MigrateIdeasToPosts(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	// Kullanıcının markalarını bul
	var companyIDs []uint
	database.DB.Model(&models.Company{}).Where("user_id = ?", userID).Pluck("id", &companyIDs)
	if len(companyIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "Marka bulunamadı", "migrated": 0})
		return
	}

	// Tüm AI ideas
	var ideas []models.AiIdea
	database.DB.Where("company_id IN ?", companyIDs).Find(&ideas)

	// Mevcut postların başlıklarını al (duplike kontrolü)
	var existingPosts []models.Post
	database.DB.Where("company_id IN ?", companyIDs).Find(&existingPosts)
	postTitles := make(map[string]map[uint]bool) // title -> companyID -> exists
	for _, p := range existingPosts {
		if postTitles[p.Title] == nil {
			postTitles[p.Title] = make(map[uint]bool)
		}
		postTitles[p.Title][p.CompanyID] = true
	}

	var migratedCount int
	for _, idea := range ideas {
		// Duplike kontrolü
		if postTitles[idea.GeneratedTitle] != nil && postTitles[idea.GeneratedTitle][idea.CompanyID] {
			continue
		}

		colorJSON := idea.ContentItems
		if colorJSON == "" {
			colorJSON = "[]"
		}
		// ContentItems geçerli JSON mi kontrol et
		var tmp interface{}
		if json.Unmarshal([]byte(colorJSON), &tmp) != nil {
			colorJSON = "[]"
		}

		post := models.Post{
			CompanyID:    idea.CompanyID,
			Title:        idea.GeneratedTitle,
			Content:      idea.GeneratedDescription,
			Platform:     idea.Platform,
			Hashtags:     idea.GeneratedHashtags,
			ImagePrompt:  idea.ImageSuggestionPrompt,
			ImageURL:     idea.ImageURL,
			ContentItems: colorJSON,
			DesignLayers: idea.DesignLayers,
			Status:       "draft",
		}
		post.CreatedAt = idea.CreatedAt // Orijinal tarihi koru

		if err := database.DB.Create(&post).Error; err != nil {
			log.Printf("⚠️ Idea→Post migration hatası (idea_id=%d): %v", idea.ID, err)
			continue
		}
		migratedCount++
	}

	log.Printf("✅ Ideas→Posts migration tamamlandı: %d/%d idea post'a dönüştürüldü", migratedCount, len(ideas))
	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"migrated": migratedCount,
		"total":    len(ideas),
		"message":  "Eski fikirler içerik yönetimine aktarıldı",
	})
}
