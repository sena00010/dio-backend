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
// GET /api/seo/technical/dashboard?projectId=X
// Teknik SEO dashboard verilerini döner
// ═══════════════════════════════════════════════════

func TechnicalDashboard(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	pid := c.Query("projectId")
	if pid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "projectId gerekli"})
		return
	}
	projectID, _ := strconv.ParseUint(pid, 10, 64)

	// Sahiplik kontrolü
	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	dashboard, err := services.GetTechnicalDashboard(uint(projectID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

// ═══════════════════════════════════════════════════
// GET /api/seo/technical/sync?projectId=X
// PSI + GSC coverage verilerini yeniden çek
// ═══════════════════════════════════════════════════

func TechnicalSync(c *gin.Context) {
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

	// Arka planda çalıştır, hemen yanıt dön
	go func() {
		_ = services.SyncTechnicalHealth(uint(projectID))
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "Teknik analiz arka planda başlatıldı. Birkaç saniye sonra sayfayı yenileyin.",
		"status":  "syncing",
	})
}

// ═══════════════════════════════════════════════════
// GET /api/seo/technical/issues/details?projectId=X&taskType=Y
// Belirli bir sorun türüne ait detayları döner
// ═══════════════════════════════════════════════════

type IssueDetail struct {
	ID          uint   `json:"id"`
	TaskType    string `json:"task_type"`
	Title       string `json:"title"`
	Description string `json:"description"`
	TargetURL   string `json:"target_url"`
	Priority    string `json:"priority"`
	Status      string `json:"status"`
}

func TechnicalIssueDetails(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	pid := c.Query("projectId")
	taskType := c.Query("taskType")
	if pid == "" || taskType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "projectId ve taskType gerekli"})
		return
	}
	projectID, _ := strconv.ParseUint(pid, 10, 64)

	// Sahiplik kontrolü
	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	// taskType'a göre sorgula
	var tasks []models.SeoActionTask
	query := database.DB.Where("project_id = ? AND status IN (?, ?)", projectID, "open", "pending")

	// taskType eşleştirme — esnek arama
	switch taskType {
	case "404_error":
		query = query.Where("task_type LIKE ? OR title LIKE ?", "%404%", "%404%")
	case "cannibalization":
		query = query.Where("task_type LIKE ? OR task_type LIKE ?", "%cannibalization%", "%duplicate%")
	case "not_indexed":
		query = query.Where("task_type LIKE ? OR title LIKE ?", "%index%", "%dizin%")
	case "schema_missing":
		query = query.Where("task_type LIKE ? OR title LIKE ?", "%schema%", "%markup%")
	case "speed_issue":
		query = query.Where("task_type LIKE ? OR title LIKE ?", "%speed%", "%hız%")
	default:
		query = query.Where("task_type = ?", taskType)
	}

	query.Order("severity ASC, created_at DESC").Limit(50).Find(&tasks)

	// Detay listesi oluştur
	details := make([]IssueDetail, 0, len(tasks))
	for _, t := range tasks {
		targetURL := ""
		if url, ok := t.ActionData["url"].(string); ok {
			targetURL = url
		} else if url, ok := t.ActionData["page_url"].(string); ok {
			targetURL = url
		} else if url, ok := t.ActionData["target_url"].(string); ok {
			targetURL = url
		}

		details = append(details, IssueDetail{
			ID:          t.ID,
			TaskType:    t.TaskType,
			Title:       t.Title,
			Description: t.Description,
			TargetURL:   targetURL,
			Priority:    t.Severity,
			Status:      t.Status,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"issues": details,
		"total":  len(details),
		"task_type": taskType,
	})
}
