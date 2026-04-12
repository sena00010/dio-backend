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
// GET /api/actions/run-engine?projectId=X
// Manuel tetikleyici — Action Engine'i çalıştırır
// ═══════════════════════════════════════════════════

func ActionRunEngine(c *gin.Context) {
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

	// Proje sahiplik kontrolü
	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı veya yetkiniz yok"})
		return
	}

	// Engine'i çalıştır
	if err := services.RunActionEngine(uint(projectID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Action Engine hatası: " + err.Error()})
		return
	}

	// Güncel görev sayısını dön
	var count int64
	database.DB.Model(&models.SeoActionTask{}).Where("project_id = ? AND status = 'open'", projectID).Count(&count)

	c.JSON(http.StatusOK, gin.H{
		"message":    "Action Engine başarıyla çalıştı",
		"project_id": projectID,
		"total_open": count,
	})
}

// ═══════════════════════════════════════════════════
// GET /api/actions/list?projectId=X
// Aksiyonlar listesi — Frontend'in veri kaynağı
// ═══════════════════════════════════════════════════

func ActionList(c *gin.Context) {
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

	// Proje sahiplik kontrolü
	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	// Görevleri çek (status = open olanlar üstte, sonra severity sırasıyla)
	var tasks []models.SeoActionTask
	database.DB.
		Where("project_id = ?", projectID).
		Order("FIELD(status, 'open', 'in_progress', 'done', 'dismissed'), FIELD(severity, 'critical', 'high', 'medium', 'low'), potential_gain DESC").
		Find(&tasks)

	// Kategorilere göre grupla
	summary := map[string]int{
		"total":            0,
		"critical":         0,
		"ctr_opportunity":  0,
		"growth":           0,
		"technical":        0,
	}

	totalPotentialGain := 0.0
	criticalGain := 0.0
	ctrGain := 0.0
	growthGain := 0.0

	for _, t := range tasks {
		if t.Status != "open" {
			continue
		}
		summary["total"]++
		switch t.Category {
		case "critical":
			summary["critical"]++
			criticalGain += t.PotentialGain
		case "ctr_opportunity":
			summary["ctr_opportunity"]++
			ctrGain += t.PotentialGain
		case "growth":
			summary["growth"]++
			growthGain += t.PotentialGain
		case "technical":
			summary["technical"]++
		}
		totalPotentialGain += t.PotentialGain
	}

	c.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
		"summary": gin.H{
			"total":            summary["total"],
			"critical":         summary["critical"],
			"ctr_opportunity":  summary["ctr_opportunity"],
			"growth":           summary["growth"],
			"total_gain":       totalPotentialGain,
			"critical_gain":    criticalGain,
			"ctr_gain":         ctrGain,
			"growth_gain":      growthGain,
		},
	})
}

// ═══════════════════════════════════════════════════
// PATCH /api/actions/:id/status
// Görev durumunu güncelle (done, dismissed, in_progress)
// ═══════════════════════════════════════════════════

func ActionUpdateStatus(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	taskID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz task_id"})
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status gerekli"})
		return
	}

	// Geçerli status değerleri
	validStatuses := map[string]bool{"open": true, "in_progress": true, "done": true, "dismissed": true}
	if !validStatuses[body.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz status: open, in_progress, done, dismissed"})
		return
	}

	// Görev kontrolü (şirket sahipliği)
	var task models.SeoActionTask
	if err := database.DB.
		Joins("JOIN seo_projects ON seo_projects.id = seo_action_tasks.project_id").
		Where("seo_action_tasks.id = ? AND seo_projects.company_id = ?", taskID, cid).
		First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Görev bulunamadı"})
		return
	}

	task.Status = body.Status
	database.DB.Save(&task)

	c.JSON(http.StatusOK, gin.H{"message": "Durum güncellendi", "task_id": task.ID, "status": task.Status})
}
