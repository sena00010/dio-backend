package handlers

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// GET /api/protected/pending-contents?company_id=X
// Henüz takvime eklenmemiş AI fikirleri ve postlar
// ──────────────────────────────────────────────

func GetPendingContents(c *gin.Context) {
	userIDVal, _ := c.Get("userID")
	userID := userIDVal.(uint)

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id parametresi gerekli"})
		return
	}

	// Sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyIDStr, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu markaya erişim yetkiniz yok"})
		return
	}

	type PendingItem struct {
		ID            uint   `json:"id"`
		Type          string `json:"type"` // "idea" or "post"
		Title         string `json:"title"`
		Content       string `json:"content"`
		Platform      string `json:"platform"`
		Format        string `json:"format"`
		ImageURL      string `json:"image_url"`
		SuggestedTime string `json:"suggested_time"`
		CreatedAt     string `json:"created_at"`
	}

	var items []PendingItem

	// ── Posts: henüz schedule'ı olmayanlar ve görseli olanlar ──
	var posts []models.Post
	database.DB.
		Where("company_id = ? AND image_url != '' AND image_url IS NOT NULL AND id NOT IN (SELECT post_id FROM ai_schedules WHERE post_id IS NOT NULL AND company_id = ?)", company.ID, company.ID).
		Order("created_at DESC").
		Limit(30).
		Find(&posts)

	for _, post := range posts {
		format := utils.PlatformAspectRatio(post.Platform) + " Feed"
		items = append(items, PendingItem{
			ID:            post.ID,
			Type:          "post",
			Title:         post.Title,
			Content:       post.Content,
			Platform:      post.Platform,
			Format:        format,
			ImageURL:      post.ImageURL,
			SuggestedTime: "19:00",
			CreatedAt:     post.CreatedAt.Format(time.RFC3339),
		})
	}

	// ── AI Ideas: sadece Post'a dönüştürülmemiş ve görseli olanlar ──
	var ideas []models.AiIdea
	database.DB.
		Where("company_id = ? AND image_url != '' AND image_url IS NOT NULL AND id NOT IN (SELECT idea_id FROM ai_schedules WHERE idea_id IS NOT NULL AND company_id = ?)", company.ID, company.ID).
		Order("created_at DESC").
		Limit(20).
		Find(&ideas)

	// Zaten Post olarak kaydedilmiş başlıkları belirle (duplike kontrolü)
	postTitles := make(map[string]bool)
	for _, p := range posts {
		postTitles[p.Title] = true
	}

	for _, idea := range ideas {
		// Eğer aynı başlıklı bir Post zaten varsa, duplike ekleme
		if postTitles[idea.GeneratedTitle] {
			continue
		}
		format := utils.PlatformAspectRatio(idea.Platform) + " Feed"
		items = append(items, PendingItem{
			ID:            idea.ID,
			Type:          "idea",
			Title:         idea.GeneratedTitle,
			Content:       idea.GeneratedDescription,
			Platform:      idea.Platform,
			Format:        format,
			ImageURL:      idea.ImageURL,
			SuggestedTime: "19:00",
			CreatedAt:     idea.CreatedAt.Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"items": items,
		"count": len(items),
	})
}

// ──────────────────────────────────────────────
// GET /api/protected/schedules?company_id=X&month=4&year=2026
// ──────────────────────────────────────────────

func ListSchedules(c *gin.Context) {
	userIDVal, _ := c.Get("userID")
	userID := userIDVal.(uint)

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id parametresi gerekli"})
		return
	}

	// Sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyIDStr, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu markaya erişim yetkiniz yok"})
		return
	}

	query := database.DB.
		Preload("Post").
		Preload("Idea").
		Where("company_id = ?", company.ID)

	// Ay/yıl filtresi
	monthStr := c.Query("month")
	yearStr := c.Query("year")
	if monthStr != "" && yearStr != "" {
		month, _ := strconv.Atoi(monthStr)
		year, _ := strconv.Atoi(yearStr)
		if month >= 1 && month <= 12 && year >= 2000 {
			start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
			end := start.AddDate(0, 1, 0)
			query = query.Where("scheduled_at >= ? AND scheduled_at < ?", start, end)
		}
	}

	var schedules []models.AiSchedule
	if err := query.Order("scheduled_at ASC").Find(&schedules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Takvim verileri alınamadı"})
		return
	}

	// İstemciye temiz veri
	type ScheduleResponse struct {
		ID          uint   `json:"id"`
		PostID      *uint  `json:"post_id"`
		IdeaID      *uint  `json:"idea_id"`
		CompanyID   uint   `json:"company_id"`
		Title       string `json:"title"`
		Content     string `json:"content"`
		Platform    string `json:"platform"`
		ImageURL    string `json:"image_url"`
		PostStatus  string `json:"post_status"`
		Status      string `json:"status"`
		ScheduledAt string `json:"scheduled_at"`
		SourceType  string `json:"source_type"` // "post" or "idea"
	}

	var result []ScheduleResponse
	for _, s := range schedules {
		r := ScheduleResponse{
			ID:          s.ID,
			PostID:      s.PostID,
			IdeaID:      s.IdeaID,
			CompanyID:   s.CompanyID,
			Platform:    s.Platform,
			Status:      s.Status,
			ScheduledAt: s.ScheduledAt.Format(time.RFC3339),
		}

		if s.PostID != nil && *s.PostID > 0 {
			r.Title = s.Post.Title
			r.Content = s.Post.Content
			r.ImageURL = s.Post.ImageURL
			r.PostStatus = s.Post.Status
			r.SourceType = "post"
		} else if s.IdeaID != nil && *s.IdeaID > 0 {
			r.Title = s.Idea.GeneratedTitle
			r.Content = s.Idea.GeneratedDescription
			r.ImageURL = s.Idea.ImageURL
			r.PostStatus = "idea"
			r.SourceType = "idea"
		}

		result = append(result, r)
	}

	c.JSON(http.StatusOK, gin.H{
		"schedules": result,
		"count":     len(result),
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/schedules — Yeni plan (multi-platform)
// ──────────────────────────────────────────────

type CreateScheduleRequest struct {
	PostID      *uint    `json:"post_id"`
	IdeaID      *uint    `json:"idea_id"`
	ScheduledAt string   `json:"scheduled_at" binding:"required"`
	Platforms   []string `json:"platforms"` // ["instagram", "tiktok"]
	Platform    string   `json:"platform"`  // Tek platform desteği (geriye uyumluluk)
}

func CreateSchedule(c *gin.Context) {
	userIDVal, _ := c.Get("userID")
	userID := userIDVal.(uint)

	var req CreateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// En az biri gerekli
	if req.PostID == nil && req.IdeaID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "post_id veya idea_id gerekli"})
		return
	}

	scheduledAt, err := time.Parse(time.RFC3339, req.ScheduledAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz tarih formatı (RFC3339 bekleniyor)"})
		return
	}

	// CompanyID belirleme
	var companyID uint
	if req.PostID != nil && *req.PostID > 0 {
		var post models.Post
		if err := database.DB.First(&post, *req.PostID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Post bulunamadı"})
			return
		}
		companyID = post.CompanyID
	} else if req.IdeaID != nil && *req.IdeaID > 0 {
		var idea models.AiIdea
		if err := database.DB.First(&idea, *req.IdeaID).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "Fikir bulunamadı"})
			return
		}
		companyID = idea.CompanyID
	}

	// Sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu içeriğe erişim yetkiniz yok"})
		return
	}

	// Platform listesini oluştur
	platforms := req.Platforms
	if len(platforms) == 0 && req.Platform != "" {
		platforms = []string{req.Platform}
	}
	if len(platforms) == 0 {
		platforms = []string{"instagram"} // Varsayılan
	}

	// Her platform için ayrı schedule oluştur
	var createdSchedules []models.AiSchedule
	for _, platform := range platforms {
		schedule := models.AiSchedule{
			CompanyID:   companyID,
			PostID:      req.PostID,
			IdeaID:      req.IdeaID,
			Platform:    platform,
			ScheduledAt: scheduledAt,
			Status:      "planned",
		}

		if err := database.DB.Create(&schedule).Error; err != nil {
			log.Printf("❌ Schedule oluşturma hatası (%s): %v", platform, err)
			continue
		}
		createdSchedules = append(createdSchedules, schedule)
	}

	if len(createdSchedules) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Plan oluşturulamadı"})
		return
	}

	log.Printf("✅ %d platform için içerik takvime eklendi: tarih=%s", len(createdSchedules), scheduledAt.Format("2006-01-02 15:04"))

	c.JSON(http.StatusCreated, gin.H{
		"message":   "İçerik takvime eklendi",
		"schedules": createdSchedules,
		"count":     len(createdSchedules),
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/schedules/update — Sürükle-bırak
// ──────────────────────────────────────────────

type UpdateScheduleRequest struct {
	ScheduleID  uint   `json:"schedule_id" binding:"required"`
	ScheduledAt string `json:"scheduled_at" binding:"required"`
}

func UpdateSchedule(c *gin.Context) {
	userIDVal, _ := c.Get("userID")
	userID := userIDVal.(uint)

	var req UpdateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scheduledAt, err := time.Parse(time.RFC3339, req.ScheduledAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz tarih formatı"})
		return
	}

	var schedule models.AiSchedule
	if err := database.DB.First(&schedule, req.ScheduleID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Plan bulunamadı"})
		return
	}

	// Sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", schedule.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu plana erişim yetkiniz yok"})
		return
	}

	schedule.ScheduledAt = scheduledAt
	if err := database.DB.Save(&schedule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Plan güncellenemedi"})
		return
	}

	log.Printf("📅 Takvim güncellendi: schedule_id=%d, yeni_tarih=%s", req.ScheduleID, scheduledAt.Format("2006-01-02 15:04"))

	c.JSON(http.StatusOK, gin.H{
		"message":  "Plan başarıyla güncellendi",
		"schedule": schedule,
	})
}

// ──────────────────────────────────────────────
// DELETE /api/protected/schedules/:id
// ──────────────────────────────────────────────

func DeleteSchedule(c *gin.Context) {
	userIDVal, _ := c.Get("userID")
	userID := userIDVal.(uint)

	scheduleID := c.Param("id")

	var schedule models.AiSchedule
	if err := database.DB.First(&schedule, scheduleID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Plan bulunamadı"})
		return
	}

	// Sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", schedule.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu plana erişim yetkiniz yok"})
		return
	}

	if err := database.DB.Delete(&schedule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Plan silinemedi"})
		return
	}

	log.Printf("🗑️ Takvim planı silindi: schedule_id=%s", scheduleID)

	c.JSON(http.StatusOK, gin.H{"message": "Plan başarıyla silindi"})
}
