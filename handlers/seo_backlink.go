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
// GET /api/seo/backlinks/dashboard?projectId=X
// ═══════════════════════════════════════════════════

func BacklinkDashboard(c *gin.Context) {
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

	dashboard, err := services.GetBacklinkDashboard(uint(projectID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

// ═══════════════════════════════════════════════════
// GET /api/seo/backlinks/sync?projectId=X
// DataForSEO'dan backlink verilerini yeniden çek
// ═══════════════════════════════════════════════════

func BacklinkSync(c *gin.Context) {
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

	cached, err := services.SyncBacklinkProject(uint(projectID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if cached {
		c.JSON(http.StatusOK, gin.H{"message": "Veriler güncel (7 günden yeni)", "cached": true})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Backlink veriler güncellendi", "cached": false})
}
