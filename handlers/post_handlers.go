package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ──────────────────────────────────────────────
// POST /api/protected/upload-canvas
// multipart/form-data → dosya kaydet → URL dön
// ──────────────────────────────────────────────

func UploadCanvas(c *gin.Context) {
	_, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}

	// multipart form'dan dosyayı al
	file, err := c.FormFile("canvas")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Dosya bulunamadı: " + err.Error()})
		return
	}

	// Hedef klasörü oluştur
	uploadDir := "./assets/uploads/posts"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		log.Printf("❌ Upload klasörü oluşturulamadı: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Klasör oluşturulamadı"})
		return
	}

	// Benzersiz dosya adı
	ext := filepath.Ext(file.Filename)
	if ext == "" {
		ext = ".png"
	}
	fileName := fmt.Sprintf("canvas_%d%s", time.Now().UnixNano(), ext)
	filePath := filepath.Join(uploadDir, fileName)

	// Dosyayı kaydet
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		log.Printf("❌ Dosya kaydetme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Dosya kaydedilemedi"})
		return
	}

	// Public URL
	publicURL := "/uploads/posts/" + fileName

	log.Printf("✅ Canvas kaydedildi: %s", publicURL)
	c.JSON(http.StatusOK, gin.H{
		"message": "Görsel başarıyla yüklendi",
		"url":     publicURL,
	})
}

// ──────────────────────────────────────────────
// PATCH /api/protected/posts/:id
// Post alanlarını güncelle (image_url vb.)
// ──────────────────────────────────────────────

type UpdatePostRequest struct {
	ImageURL     *string `json:"image_url"`
	Title        *string `json:"title"`
	Content      *string `json:"content"`
	Status       *string `json:"status"`
	DesignLayers *string `json:"design_layers"` // Metin katmanları JSON
}

func UpdatePost(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	postID := c.Param("id")

	var post models.Post
	if err := database.DB.First(&post, postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post bulunamadı"})
		return
	}

	// Şirket sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", post.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu posta erişim yetkiniz yok"})
		return
	}

	var req UpdatePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Partial update
	if req.ImageURL != nil {
		post.ImageURL = *req.ImageURL
	}
	if req.Title != nil {
		post.Title = *req.Title
	}
	if req.Content != nil {
		post.Content = *req.Content
	}
	if req.Status != nil {
		post.Status = *req.Status
	}
	if req.DesignLayers != nil {
		post.DesignLayers = *req.DesignLayers
	}

	if err := database.DB.Save(&post).Error; err != nil {
		log.Printf("❌ Post güncelleme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Post güncellenemedi"})
		return
	}

	log.Printf("✅ Post güncellendi: ID=%d", post.ID)
	c.JSON(http.StatusOK, gin.H{
		"message": "Post başarıyla güncellendi",
		"post":    post,
	})
}
