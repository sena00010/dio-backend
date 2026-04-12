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
// GET /api/seo/keywords/dashboard?projectId=X
// Kelime Intelligence — Brand/NonBrand, Intent, Rising Commercial, AI Banner
// ═══════════════════════════════════════════════════

func KeywordDashboard(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	projectIDStr := c.Query("projectId")
	if projectIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "projectId gerekli"})
		return
	}
	projectID, err := strconv.ParseUint(projectIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz projectId"})
		return
	}

	// Sahiplik kontrolü
	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	dashboard, err := services.GetKeywordDashboard(uint(projectID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Keyword Intelligence hatası: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}
