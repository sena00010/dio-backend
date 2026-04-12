package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/services"
)

// ═══════════════════════════════════════════════════
// GET /api/seo/content/opportunities?projectId=X
// Kelimelerden otomatik içerik fırsatı listesi
// ═══════════════════════════════════════════════════

func ContentOpportunities(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	pid := c.Query("projectId")
	if pid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "projectId gerekli"})
		return
	}
	projectID, _ := strconv.ParseUint(pid, 10, 64)

	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	opps, err := services.GetContentOpportunities(uint(projectID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"opportunities": opps, "total": len(opps)})
}

// ═══════════════════════════════════════════════════
// POST /api/seo/content/generate-brief
// LLM ile content brief üret ve DB'ye kaydet
// ═══════════════════════════════════════════════════

func ContentGenerateBrief(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	var req services.GenerateBriefRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
		return
	}

	// Sahiplik kontrolü
	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", req.ProjectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	brief, err := services.GenerateContentBrief(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Brief üretim hatası: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"brief": brief, "message": "Brief başarıyla oluşturuldu"})
}

// ═══════════════════════════════════════════════════
// GET /api/seo/content/briefs?projectId=X
// Kaydedilmiş brief'leri listele
// ═══════════════════════════════════════════════════

func ContentBriefsList(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	pid := c.Query("projectId")
	if pid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "projectId gerekli"})
		return
	}
	projectID, _ := strconv.ParseUint(pid, 10, 64)

	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	briefs, err := services.GetContentBriefs(uint(projectID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"briefs": briefs, "total": len(briefs)})
}
