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
// GET /api/seo/rankings/sync?projectId=X
// GSC'den keyword sıralama verilerini çeker ve DB'ye yazar
// ═══════════════════════════════════════════════════

func RankingsSync(c *gin.Context) {
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

	if err := services.SyncRankings(uint(projectID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Senkronizasyon hatası: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Rankings sync tamamlandı", "project_id": projectID})
}

// ═══════════════════════════════════════════════════
// GET /api/seo/rankings/list?projectId=X
// Aggregated keyword listesi — frontend'in ana veri kaynağı
// ═══════════════════════════════════════════════════

func RankingsList(c *gin.Context) {
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

	keywords, err := services.GetRankedKeywords(uint(projectID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Veri çekme hatası: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"keywords": keywords,
		"total":    len(keywords),
	})
}

// ═══════════════════════════════════════════════════
// GET /api/seo/rankings/summary?projectId=X
// 4 kartı besleyen özet metrikleri
// ═══════════════════════════════════════════════════

func RankingsSummary(c *gin.Context) {
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

	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	summary := services.GetRankingSummary(uint(projectID))

	c.JSON(http.StatusOK, summary)
}
