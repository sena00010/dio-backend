package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ListStudioBriefs — şirkete ait tüm brief'leri listeler
// GET /api/protected/studio-briefs?type=urun&studio_id=5
func ListStudioBriefs(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	company, ok := ownedCompanyFromUser(c, userID)
	if !ok {
		return
	}

	briefType := strings.TrimSpace(c.Query("type"))
	studioIDStr := strings.TrimSpace(c.Query("studio_id"))

	query := database.DB.Where("company_id = ?", company.ID).Order("is_favorite DESC, updated_at DESC")

	if briefType != "" {
		query = query.Where("brief_type = ?", briefType)
	}
	if studioIDStr != "" {
		sid, err := strconv.ParseUint(studioIDStr, 10, 64)
		if err == nil {
			query = query.Where("studio_id = ?", sid)
		}
	}

	var briefs []models.StudioBrief
	if err := query.Find(&briefs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Brief'ler getirilemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"briefs": briefs})
}

// CreateStudioBrief — yeni brief kaydeder
// POST /api/protected/studio-briefs
func CreateStudioBrief(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	company, ok := ownedCompanyFromUser(c, userID)
	if !ok {
		return
	}

	var req struct {
		Title      string `json:"title"`
		Content    string `json:"content"`
		BriefType  string `json:"brief_type"`
		Tags       string `json:"tags"`
		StudioID   *uint  `json:"studio_id"`
		IsFavorite bool   `json:"is_favorite"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Brief içeriği boş olamaz"})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Yeni Brief"
	}

	briefType := strings.TrimSpace(req.BriefType)
	if briefType == "" {
		briefType = "genel"
	}

	brief := models.StudioBrief{
		CompanyID:  company.ID,
		StudioID:   req.StudioID,
		Title:      title,
		Content:    content,
		BriefType:  briefType,
		Tags:       strings.TrimSpace(req.Tags),
		IsFavorite: req.IsFavorite,
	}

	if err := database.DB.Create(&brief).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Brief kaydedilemedi"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"brief": brief, "message": "Brief kaydedildi"})
}

// UpdateStudioBrief — mevcut brief'i günceller
// PATCH /api/protected/studio-briefs/:id
func UpdateStudioBrief(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	company, ok := ownedCompanyFromUser(c, userID)
	if !ok {
		return
	}

	briefID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || briefID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz brief ID"})
		return
	}

	var brief models.StudioBrief
	if err := database.DB.Where("id = ? AND company_id = ?", briefID, company.ID).First(&brief).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Brief bulunamadı"})
		return
	}

	var req struct {
		Title      *string `json:"title"`
		Content    *string `json:"content"`
		BriefType  *string `json:"brief_type"`
		Tags       *string `json:"tags"`
		IsFavorite *bool   `json:"is_favorite"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{"updated_at": time.Now()}
	if req.Title != nil {
		updates["title"] = strings.TrimSpace(*req.Title)
	}
	if req.Content != nil {
		updates["content"] = strings.TrimSpace(*req.Content)
	}
	if req.BriefType != nil {
		updates["brief_type"] = strings.TrimSpace(*req.BriefType)
	}
	if req.Tags != nil {
		updates["tags"] = strings.TrimSpace(*req.Tags)
	}
	if req.IsFavorite != nil {
		updates["is_favorite"] = *req.IsFavorite
	}

	if err := database.DB.Model(&brief).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Brief güncellenemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"brief": brief, "message": "Brief güncellendi"})
}

// DeleteStudioBrief — brief siler
// DELETE /api/protected/studio-briefs/:id
func DeleteStudioBrief(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	company, ok := ownedCompanyFromUser(c, userID)
	if !ok {
		return
	}

	briefID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || briefID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz brief ID"})
		return
	}

	result := database.DB.Where("id = ? AND company_id = ?", briefID, company.ID).Delete(&models.StudioBrief{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Brief silinemedi"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Brief bulunamadı"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Brief silindi"})
}

// UseStudioBrief — kullanım sayacını arttırır
// POST /api/protected/studio-briefs/:id/use
func UseStudioBrief(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	company, ok := ownedCompanyFromUser(c, userID)
	if !ok {
		return
	}

	briefID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || briefID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz brief ID"})
		return
	}

	var brief models.StudioBrief
	if err := database.DB.Where("id = ? AND company_id = ?", briefID, company.ID).First(&brief).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Brief bulunamadı"})
		return
	}

	database.DB.Model(&brief).Update("used_count", brief.UsedCount+1)

	c.JSON(http.StatusOK, gin.H{"brief": brief})
}

// ownedCompanyFromUser — kullanıcının şirketini döner (yardımcı)
func ownedCompanyFromUser(c *gin.Context, userID uint) (models.Company, bool) {
	var company models.Company
	if err := database.DB.Where("user_id = ?", userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Şirket bulunamadı"})
		return company, false
	}
	return company, true
}
