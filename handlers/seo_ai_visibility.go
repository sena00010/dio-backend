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
// GET /api/seo/ai-visibility/dashboard?projectId=X
// ═══════════════════════════════════════════════════

func AIVisibilityDashboard(c *gin.Context) {
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

	dashboard, err := services.GetAIVisibilityDashboard(uint(projectID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

// ═══════════════════════════════════════════════════
// GET /api/seo/ai-visibility/sync?projectId=X
// ═══════════════════════════════════════════════════

func AIVisibilitySync(c *gin.Context) {
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

	cached, err := services.SyncAIVisibility(uint(projectID), true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "AI görünürlük verileri güncellendi", "cached": cached})
}
