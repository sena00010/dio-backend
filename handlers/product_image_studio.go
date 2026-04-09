package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

type productStudioJob struct {
	JobID      string    `json:"job_id"`
	StudioID   uint      `json:"studio_id"`
	Mode       string    `json:"mode,omitempty"`
	Status     string    `json:"status"`
	Total      int       `json:"total"`
	Completed  int32     `json:"completed"`
	StartedAt  time.Time `json:"started_at"`
	LastUpdate time.Time `json:"last_update"`
}

var (
	productStudioJobs   = make(map[string]*productStudioJob)
	productStudioJobsMu sync.RWMutex
)

type productStudioModule struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Icon        string `json:"icon"`
	Route       string `json:"route"`
	Enabled     bool   `json:"enabled"`
	Active      bool   `json:"active"`
	Badge       string `json:"badge,omitempty"`
	Description string `json:"description"`
}

func hasReferenceAnalysisProvider() bool {
	return strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) != "" || strings.TrimSpace(os.Getenv("GEMINI_API_KEY")) != ""
}

func shouldRefreshStudioReference(studio models.ProductImageStudio) bool {
	summary := strings.ToLower(strings.TrimSpace(studio.SourceAnalysis))
	if summary == "" {
		return true
	}

	legacySignals := []string{
		"a product from the ",
		"category photographed in a casual source image",
		"giyim & moda category",
		"ayakkabi category",
		"ev dekorasyon category",
		"elektronik category",
	}

	for _, signal := range legacySignals {
		if strings.Contains(summary, signal) {
			return true
		}
	}

	return false
}

func refreshStudioReferenceIfNeeded(studio *models.ProductImageStudio) {
	if studio == nil || !shouldRefreshStudioReference(*studio) || !hasReferenceAnalysisProvider() {
		return
	}

	referenceSummary := strings.TrimSpace(utils.DescribeProductImageReference(studio.SourceImageURL, studio.Category))
	if referenceSummary == "" || strings.EqualFold(referenceSummary, strings.TrimSpace(studio.SourceAnalysis)) {
		return
	}

	studio.SourceAnalysis = referenceSummary
	database.DB.Model(studio).Update("source_analysis", referenceSummary)
}

func GetProductStudioSidebar(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"modules": []productStudioModule{
			{
				Key:         "product-image-studio",
				Label:       "Urun Gorseli Studyosu",
				Icon:        "🖼️",
				Route:       "/creative/product-image-studio",
				Enabled:     true,
				Active:      true,
				Description: "AI ile urun gorselini pazar yeri ve sosyal medya varyantlarina donusturur.",
			},
			{
				Key:         "ad-studio",
				Label:       "Reklam Studyosu",
				Icon:        "📣",
				Route:       "/creative/ad-studio",
				Enabled:     true,
				Active:      false,
				Badge:       "YENI",
				Description: "Tek gorselden banner, story ve feed reklam kreatifleri uretir.",
			},
		},
	})
}

func UpdateProductImageStudio(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	studioID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || studioID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gecersiz studio id"})
		return
	}

	var studio models.ProductImageStudio
	if err := database.DB.First(&studio, studioID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "studio bulunamadi"})
		return
	}

	if _, ok := ownedCompany(c, userID, studio.CompanyID); !ok {
		return
	}

	var req struct {
		Title         *string `json:"title"`
		Category      *string `json:"category"`
		SourceSummary *string `json:"source_summary"`
		ActiveTab     *string `json:"active_tab"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if req.Title != nil {
		updates["title"] = strings.TrimSpace(*req.Title)
	}
	if req.Category != nil {
		category := strings.TrimSpace(*req.Category)
		if category == "Otomatik Algila" || strings.EqualFold(category, "auto") {
			category = ""
		}
		updates["category"] = category
	}
	if req.SourceSummary != nil {
		updates["source_analysis"] = strings.TrimSpace(*req.SourceSummary)
	}
	if req.ActiveTab != nil {
		updates["active_tab"] = strings.TrimSpace(*req.ActiveTab)
	}

	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{"studio": buildStudioResponse(studio, studio.Variants)})
		return
	}

	if err := database.DB.Model(&studio).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "studio guncellenemedi"})
		return
	}

	if err := database.DB.Preload("Variants").Preload("AdVariants").Preload("SelectedVariant").Preload("SelectedAdVariant").First(&studio, studio.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "guncel studio getirilemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Studio ayarlari guncellendi",
		"studio":  buildStudioResponse(studio, studio.Variants),
	})
}

func AnalyzeProductImageStudio(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	studioID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || studioID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gecersiz studio id"})
		return
	}

	var studio models.ProductImageStudio
	if err := database.DB.Preload("Variants").Preload("AdVariants").Preload("SelectedVariant").Preload("SelectedAdVariant").First(&studio, studioID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "studio bulunamadi"})
		return
	}

	if _, ok := ownedCompany(c, userID, studio.CompanyID); !ok {
		return
	}

	referenceSummary := utils.DescribeProductImageReference(studio.SourceImageURL, studio.Category)
	studio.SourceAnalysis = referenceSummary
	if err := database.DB.Model(&studio).Update("source_analysis", referenceSummary).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "analiz kaydedilemedi"})
		return
	}

	if err := database.DB.Preload("Variants").Preload("AdVariants").Preload("SelectedVariant").Preload("SelectedAdVariant").First(&studio, studio.ID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "guncel studio getirilemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "AI analiz tamamlandi",
		"studio":  buildStudioResponse(studio, studio.Variants),
	})
}

func ListProductImageStudios(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	companyID, err := strconv.ParseUint(c.Query("company_id"), 10, 64)
	if err != nil || companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}

	if _, ok := ownedCompany(c, userID, uint(companyID)); !ok {
		return
	}

	var studios []models.ProductImageStudio
	if err := database.DB.
		Preload("Variants").
		Preload("AdVariants").
		Preload("SelectedVariant").
		Preload("SelectedAdVariant").
		Where("company_id = ?", uint(companyID)).
		Order("updated_at desc").
		Limit(12).
		Find(&studios).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "studio listesi getirilemedi"})
		return
	}

	for i := range studios {
		refreshStudioReferenceIfNeeded(&studios[i])
	}

	result := make([]gin.H, 0, len(studios))
	for _, studio := range studios {
		result = append(result, buildStudioResponse(studio, studio.Variants))
	}

	c.JSON(http.StatusOK, gin.H{"studios": result})
}

func UploadProductImageStudioSource(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	companyID, err := strconv.ParseUint(c.PostForm("company_id"), 10, 64)
	if err != nil || companyID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}

	company, ok := ownedCompany(c, userID, uint(companyID))
	if !ok {
		return
	}

	file, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "image dosyasi gerekli"})
		return
	}

	uploadDir := "./assets/uploads/product-studio"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload klasoru olusturulamadi"})
		return
	}

	ext := filepath.Ext(file.Filename)
	if ext == "" {
		ext = ".png"
	}

	fileName := fmt.Sprintf("product_%d%s", time.Now().UnixNano(), ext)
	filePath := filepath.Join(uploadDir, fileName)
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "dosya kaydedilemedi"})
		return
	}

	title := c.PostForm("title")
	if title == "" {
		title = "Urun Fotografi"
	}
	category := c.PostForm("category")
	if category == "Otomatik Algila" || category == "auto" {
		category = ""
	}
	slotGroup := c.PostForm("slot_group")
	if slotGroup == "" {
		slotGroup = "2-4-ek"
	}

	studio := models.ProductImageStudio{
		CompanyID:      company.ID,
		Title:          title,
		SourceImageURL: "/uploads/product-studio/" + fileName,
		SourceFileName: file.Filename,
		Category:       category,
		SlotGroup:      slotGroup,
		ActiveTab:      "urun-gorseli-studyosu",
		Status:         "uploaded",
	}

	if hasReferenceAnalysisProvider() {
		studio.SourceAnalysis = utils.DescribeProductImageReference(studio.SourceImageURL, studio.Category)
	}

	if err := database.DB.Create(&studio).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "studio kaydi olusturulamadi"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Kaynak urun gorseli yuklendi",
		"studio":  buildStudioResponse(studio, nil),
	})
}

func GenerateProductImageStudio(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	studioID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || studioID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gecersiz studio id"})
		return
	}

	var studio models.ProductImageStudio
	if err := database.DB.Preload("Variants").First(&studio, studioID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "studio bulunamadi"})
		return
	}

	company, ok := ownedCompany(c, userID, studio.CompanyID)
	if !ok {
		return
	}

	var req struct {
		Force bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req)

	if req.Force {
		database.DB.Where("studio_id = ?", studio.ID).Delete(&models.ProductImageVariant{})
		studio.Variants = nil
	}

	presets := utils.DefaultProductStudioPresets()
	if len(studio.Variants) == 0 {
		for i, preset := range presets {
			variant := models.ProductImageVariant{
				StudioID:      studio.ID,
				VariantKey:    preset.Key,
				Name:          preset.Name,
				Platform:      preset.Platform,
				FormatLabel:   fmt.Sprintf("%dx%d", preset.Width, preset.Height),
				Width:         preset.Width,
				Height:        preset.Height,
				Background:    preset.Background,
				Score:         preset.Score,
				Status:        "pending",
				PreviewURL:    studio.SourceImageURL,
				BadgeLabel:    preset.BadgeLabel,
				BadgeTone:     preset.BadgeTone,
				DesignNotes:   preset.DesignNotes,
				IsRecommended: preset.IsRecommended,
				SortOrder:     i,
			}
			database.DB.Create(&variant)
			studio.Variants = append(studio.Variants, variant)
		}
	}

	studio.Status = "processing"
	database.DB.Save(&studio)

	jobID := generateJobID()
	job := &productStudioJob{
		JobID:      jobID,
		StudioID:   studio.ID,
		Mode:       "product",
		Status:     "processing",
		Total:      len(studio.Variants),
		StartedAt:  time.Now(),
		LastUpdate: time.Now(),
	}

	productStudioJobsMu.Lock()
	productStudioJobs[jobID] = job
	productStudioJobsMu.Unlock()

	go runProductStudioGeneration(job, company, studio)

	c.JSON(http.StatusOK, gin.H{
		"job_id":    jobID,
		"studio_id": studio.ID,
		"mode":      "product",
		"status":    "processing",
		"total":     len(studio.Variants),
		"message":   "Urun gorseli studyosu varyantlari uretiliyor",
	})
}

func GenerateProductImageAdStudio(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	studioID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || studioID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gecersiz studio id"})
		return
	}

	var studio models.ProductImageStudio
	if err := database.DB.Preload("AdVariants").First(&studio, studioID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "studio bulunamadi"})
		return
	}

	company, ok := ownedCompany(c, userID, studio.CompanyID)
	if !ok {
		return
	}

	var req struct {
		Force          bool     `json:"force"`
		Formats        []string `json:"formats"`
		TextMode       string   `json:"text_mode"`
		ManualHeadline string   `json:"manual_headline"`
		ManualSub      string   `json:"manual_sub_headline"`
		ManualCTA      string   `json:"manual_cta"`
	}
	_ = c.ShouldBindJSON(&req)

	if req.Force {
		database.DB.Where("studio_id = ?", studio.ID).Delete(&models.ProductImageAdVariant{})
		studio.AdVariants = nil
	}

	presets := utils.DefaultAdStudioPresets()
	selectedFormats := make(map[string]bool)
	for _, key := range req.Formats {
		selectedFormats[key] = true
	}

	if len(studio.AdVariants) == 0 {
		order := 0
		for _, preset := range presets {
			if len(selectedFormats) > 0 && !selectedFormats[preset.Key] {
				continue
			}

			copyPayload := utils.GenerateAdCopy(company, studio, preset)
			textMode := req.TextMode
			if textMode == "" {
				textMode = "ai"
			}
			if textMode == "manual" {
				if req.ManualHeadline != "" {
					copyPayload.Headline = req.ManualHeadline
				}
				if req.ManualSub != "" {
					copyPayload.SubHeadline = req.ManualSub
				}
				if req.ManualCTA != "" {
					copyPayload.CTA = req.ManualCTA
				}
			}

			meta, _ := json.Marshal(copyPayload)
			variant := models.ProductImageAdVariant{
				StudioID:      studio.ID,
				VariantKey:    preset.Key,
				FormatKey:     preset.Key,
				Name:          preset.Name,
				Platform:      preset.Platform,
				Width:         preset.Width,
				Height:        preset.Height,
				Score:         preset.Score,
				Status:        "pending",
				PreviewURL:    studio.SourceImageURL,
				BadgeLabel:    preset.BadgeLabel,
				BadgeTone:     preset.BadgeTone,
				Headline:      copyPayload.Headline,
				SubHeadline:   copyPayload.SubHeadline,
				CTA:           copyPayload.CTA,
				TextMode:      textMode,
				TextAlign:     preset.TextAlign,
				TextPosition:  preset.TextPosition,
				FormatLabel:   preset.FormatLabel,
				DesignNotes:   preset.DesignNotes,
				IsRecommended: preset.IsRecommended,
				SortOrder:     order,
				MetaJSON:      string(meta),
			}
			order++
			database.DB.Create(&variant)
			studio.AdVariants = append(studio.AdVariants, variant)
		}
	}

	jobID := generateJobID()
	job := &productStudioJob{
		JobID:      jobID,
		StudioID:   studio.ID,
		Mode:       "ad",
		Status:     "processing",
		Total:      len(studio.AdVariants),
		StartedAt:  time.Now(),
		LastUpdate: time.Now(),
	}

	productStudioJobsMu.Lock()
	productStudioJobs[jobID] = job
	productStudioJobsMu.Unlock()

	go runProductAdStudioGeneration(job, company, studio)

	c.JSON(http.StatusOK, gin.H{
		"job_id":    jobID,
		"studio_id": studio.ID,
		"mode":      "ad",
		"status":    "processing",
		"total":     len(studio.AdVariants),
		"message":   "Reklam studyosu varyantlari uretiliyor",
	})
}

func GetProductImageStudioJob(c *gin.Context) {
	jobID := c.Param("jobID")
	productStudioJobsMu.RLock()
	job, ok := productStudioJobs[jobID]
	productStudioJobsMu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "job bulunamadi"})
		return
	}

	var studio models.ProductImageStudio
	if err := database.DB.Preload("Variants").Preload("AdVariants").Preload("SelectedVariant").Preload("SelectedAdVariant").First(&studio, job.StudioID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "studio bulunamadi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"job": gin.H{
			"job_id":      job.JobID,
			"studio_id":   job.StudioID,
			"mode":        job.Mode,
			"status":      job.Status,
			"total":       job.Total,
			"completed":   atomic.LoadInt32(&job.Completed),
			"started_at":  job.StartedAt,
			"last_update": job.LastUpdate,
		},
		"studio": buildStudioResponse(studio, studio.Variants),
	})
}

func GetProductImageStudio(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	studioID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || studioID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gecersiz studio id"})
		return
	}

	var studio models.ProductImageStudio
	if err := database.DB.Preload("Variants").Preload("AdVariants").Preload("SelectedVariant").Preload("SelectedAdVariant").First(&studio, studioID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "studio bulunamadi"})
		return
	}

	if _, ok := ownedCompany(c, userID, studio.CompanyID); !ok {
		return
	}

	refreshStudioReferenceIfNeeded(&studio)

	c.JSON(http.StatusOK, gin.H{
		"studio": buildStudioResponse(studio, studio.Variants),
	})
}

func SelectProductImageVariant(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	variantID, err := strconv.ParseUint(c.Param("variantID"), 10, 64)
	if err != nil || variantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gecersiz varyant id"})
		return
	}

	var variant models.ProductImageVariant
	if err := database.DB.First(&variant, variantID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "varyant bulunamadi"})
		return
	}

	var studio models.ProductImageStudio
	if err := database.DB.First(&studio, variant.StudioID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "studio bulunamadi"})
		return
	}

	if _, ok := ownedCompany(c, userID, studio.CompanyID); !ok {
		return
	}

	studio.SelectedVariantID = &variant.ID
	studio.Status = "ready"
	if err := database.DB.Save(&studio).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "secim kaydedilemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":             "Varyant secimi kaydedildi",
		"selected_variant":    variant,
		"selected_variant_id": variant.ID,
	})
}

func SelectProductImageAdVariant(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	variantID, err := strconv.ParseUint(c.Param("variantID"), 10, 64)
	if err != nil || variantID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "gecersiz reklam varyant id"})
		return
	}

	var variant models.ProductImageAdVariant
	if err := database.DB.First(&variant, variantID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "reklam varyanti bulunamadi"})
		return
	}

	var studio models.ProductImageStudio
	if err := database.DB.First(&studio, variant.StudioID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "studio bulunamadi"})
		return
	}

	if _, ok := ownedCompany(c, userID, studio.CompanyID); !ok {
		return
	}

	studio.SelectedAdVariantID = &variant.ID
	studio.ActiveTab = "reklam-studyosu"
	if err := database.DB.Save(&studio).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reklam secimi kaydedilemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":                "Reklam varyanti secimi kaydedildi",
		"selected_ad_variant":    variant,
		"selected_ad_variant_id": variant.ID,
	})
}

func runProductStudioGeneration(job *productStudioJob, company models.Company, studio models.ProductImageStudio) {
	referenceSummary := studio.SourceAnalysis
	if shouldRefreshStudioReference(studio) {
		referenceSummary = utils.DescribeProductImageReference(studio.SourceImageURL, studio.Category)
		studio.SourceAnalysis = referenceSummary
		database.DB.Model(&studio).Update("source_analysis", referenceSummary)
	}

	var variants []models.ProductImageVariant
	database.DB.Where("studio_id = ?", studio.ID).Order("sort_order asc").Find(&variants)
	presets := utils.DefaultProductStudioPresets()
	presetByKey := make(map[string]utils.ProductStudioPreset, len(presets))
	for _, preset := range presets {
		presetByKey[preset.Key] = preset
	}

	for i := range variants {
		variant := variants[i]
		preset, ok := presetByKey[variant.VariantKey]
		if !ok {
			continue
		}

		concurrentSemaphore <- struct{}{}
		func(v models.ProductImageVariant, p utils.ProductStudioPreset) {
			defer func() { <-concurrentSemaphore }()

			prompt := utils.BuildProductImageEditPrompt(studio, p, referenceSummary)
			provider := "openai-image-edit"

			database.DB.Model(&v).Updates(map[string]interface{}{
				"status":   "generating",
				"prompt":   prompt,
				"provider": provider,
			})

			imageURL, err := utils.GenerateProductImageEdit(studio.SourceImageURL, prompt, p.Width, p.Height)
			update := map[string]interface{}{
				"prompt":   prompt,
				"provider": provider,
			}
			if err != nil {
				log.Printf("⚠️ Product studio image-edit basarisiz (%s), fallback devrede (kaynak gorsel ile): %v", v.Name, err)
				// Fallback: Kaynak gorseli Gemini'ye gonderip yeni tasarim uret
				fallbackPrompt := utils.BuildProductStudioPrompt(company, studio, p, referenceSummary)
				imageURL, err = utils.GenerateProductImageWithSource(studio.SourceImageURL, fallbackPrompt)
				if err != nil {
					log.Printf("❌ Fallback gorsel+prompt uretimi de basarisiz (%s): %v", v.Name, err)
					update["status"] = "ready"
					update["preview_url"] = studio.SourceImageURL
					update["design_notes"] = p.DesignNotes + " | Image-edit ve fallback uretim basarisiz oldugu icin kaynak gorsel korundu."
				} else {
					update["status"] = "ready"
					update["preview_url"] = imageURL
					update["provider"] = "openai-with-source"
					update["design_notes"] = p.DesignNotes + " | Kaynak gorsel ChatGPT (gpt-image-1) ile uretildi."
				}
				database.DB.Model(&v).Updates(update)
				atomic.AddInt32(&job.Completed, 1)
				job.LastUpdate = time.Now()
				return
			}
			update["status"] = "ready"
			update["preview_url"] = imageURL
			database.DB.Model(&v).Updates(update)

			atomic.AddInt32(&job.Completed, 1)
			job.LastUpdate = time.Now()
		}(variant, preset)
	}

	job.Status = "completed"
	job.LastUpdate = time.Now()

	var firstRecommended models.ProductImageVariant
	if err := database.DB.Where("studio_id = ? AND is_recommended = ?", studio.ID, true).Order("sort_order asc").First(&firstRecommended).Error; err == nil {
		database.DB.Model(&studio).Updates(map[string]interface{}{
			"status":              "ready",
			"selected_variant_id": firstRecommended.ID,
		})
	} else {
		database.DB.Model(&studio).Update("status", "ready")
	}
}

func runProductAdStudioGeneration(job *productStudioJob, company models.Company, studio models.ProductImageStudio) {
	referenceSummary := studio.SourceAnalysis
	if shouldRefreshStudioReference(studio) {
		referenceSummary = utils.DescribeProductImageReference(studio.SourceImageURL, studio.Category)
		studio.SourceAnalysis = referenceSummary
		database.DB.Model(&studio).Update("source_analysis", referenceSummary)
	}

	var variants []models.ProductImageAdVariant
	database.DB.Where("studio_id = ?", studio.ID).Order("sort_order asc").Find(&variants)
	presetByKey := make(map[string]utils.AdStudioPreset)
	for _, preset := range utils.DefaultAdStudioPresets() {
		presetByKey[preset.Key] = preset
	}

	for i := range variants {
		variant := variants[i]
		preset, ok := presetByKey[variant.FormatKey]
		if !ok {
			continue
		}

		concurrentSemaphore <- struct{}{}
		func(v models.ProductImageAdVariant, p utils.AdStudioPreset, idx int) {
			defer func() { <-concurrentSemaphore }()

			copyPayload := utils.AdCopyPayload{
				Headline:    v.Headline,
				SubHeadline: v.SubHeadline,
				CTA:         v.CTA,
			}
			prompt := utils.BuildAdStudioPrompt(company, studio, p, referenceSummary, copyPayload)
			provider := selectProvider(detectAvailableProviders(), idx)

			database.DB.Model(&v).Updates(map[string]interface{}{
				"status":   "generating",
				"prompt":   prompt,
				"provider": provider,
			})

			ctx := utils.BrandContext{
				BrandName:      company.Name,
				Sector:         company.Sector,
				TargetAudience: company.TargetAudience,
				BrandColors:    company.BrandColors,
				ToneOfVoice:    company.ToneOfVoice,
				Description:    company.Description,
				BrandFont:      company.BrandFont,
				Platform:       p.Platform,
				Headline:       v.Headline,
				SubText:        v.SubHeadline,
				ImagePrompt:    prompt,
				Style:          "realistic",
				TextPosition:   p.TextPosition,
			}

			imageURL, err := utils.GenerateBrandImage(ctx)
			update := map[string]interface{}{
				"prompt":   prompt,
				"provider": provider,
				"status":   "ready",
			}
			if err != nil {
				log.Printf("⚠️ Ad studio varyanti fallback'a dustu (%s): %v", v.Name, err)
				update["preview_url"] = studio.SourceImageURL
			} else {
				update["preview_url"] = imageURL
			}
			database.DB.Model(&v).Updates(update)

			atomic.AddInt32(&job.Completed, 1)
			job.LastUpdate = time.Now()
		}(variant, preset, i)
	}

	job.Status = "completed"
	job.LastUpdate = time.Now()

	var firstRecommended models.ProductImageAdVariant
	if err := database.DB.Where("studio_id = ? AND is_recommended = ?", studio.ID, true).Order("sort_order asc").First(&firstRecommended).Error; err == nil {
		database.DB.Model(&studio).Updates(map[string]interface{}{
			"selected_ad_variant_id": firstRecommended.ID,
			"active_tab":             "reklam-studyosu",
		})
	}
}

func buildStudioResponse(studio models.ProductImageStudio, variants []models.ProductImageVariant) gin.H {
	sort.SliceStable(variants, func(i, j int) bool {
		return variants[i].SortOrder < variants[j].SortOrder
	})

	adVariants := append([]models.ProductImageAdVariant(nil), studio.AdVariants...)
	sort.SliceStable(adVariants, func(i, j int) bool {
		return adVariants[i].SortOrder < adVariants[j].SortOrder
	})

	var selected *models.ProductImageVariant
	if studio.SelectedVariantID != nil {
		for i := range variants {
			if variants[i].ID == *studio.SelectedVariantID {
				selected = &variants[i]
				break
			}
		}
	}
	if selected == nil && len(variants) > 0 {
		selected = &variants[0]
	}

	var selectedAd *models.ProductImageAdVariant
	if studio.SelectedAdVariantID != nil {
		for i := range adVariants {
			if adVariants[i].ID == *studio.SelectedAdVariantID {
				selectedAd = &adVariants[i]
				break
			}
		}
	}
	if selectedAd == nil && len(adVariants) > 0 {
		selectedAd = &adVariants[0]
	}

	categories := []gin.H{
		{"key": "otomatik-algila", "label": "Otomatik Algila", "active": studio.Category == ""},
		{"key": "giyim-moda", "label": "Giyim & Moda", "active": studio.Category == "Giyim & Moda"},
		{"key": "ayakkabi", "label": "Ayakkabi", "active": studio.Category == "Ayakkabi"},
		{"key": "evcil-hayvan", "label": "Evcil Hayvan", "active": studio.Category == "Evcil Hayvan"},
		{"key": "ev-dekorasyon", "label": "Ev Dekorasyon", "active": studio.Category == "Ev Dekorasyon"},
		{"key": "elektronik", "label": "Elektronik", "active": studio.Category == "Elektronik"},
	}

	orderTabs := []gin.H{
		{"key": "1-ana", "label": "1. Ana", "active": studio.SlotGroup == "1-ana"},
		{"key": "2-4-ek", "label": "2-4. Ek", "active": studio.SlotGroup == "" || studio.SlotGroup == "2-4-ek"},
		{"key": "5-8-yasam", "label": "5-8. Yasam", "active": studio.SlotGroup == "5-8-yasam"},
	}

	return gin.H{
		"id":             studio.ID,
		"title":          studio.Title,
		"status":         studio.Status,
		"active_tab":     studio.ActiveTab,
		"source_image":   studio.SourceImageURL,
		"source_file":    studio.SourceFileName,
		"source_summary": studio.SourceAnalysis,
		"left_panel": gin.H{
			"hint":       "Karisik arka plan sorun degil, AI arka plani kaldirir ve optimize eder.",
			"order_tabs": orderTabs,
			"categories": categories,
		},
		"header": gin.H{
			"title":         "Urun Gorseli Studyosu",
			"subtitle":      "Trendyol + Hepsiburada + Sosyal Medya",
			"variant_count": len(variants),
		},
		"ad_header": gin.H{
			"title":         "Reklam Studyosu",
			"subtitle":      "Urun gorselinizden banner ve kreatif olusturun",
			"variant_count": len(adVariants),
			"formats":       utils.DefaultAdStudioPresets(),
		},
		"variants":            variants,
		"ad_variants":         adVariants,
		"selected_variant":    selected,
		"selected_ad_variant": selectedAd,
	}
}

func currentUserID(c *gin.Context) (uint, bool) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanici kimligi bulunamadi"})
		return 0, false
	}
	return userIDVal.(uint), true
}

func ownedCompany(c *gin.Context, userID uint, companyID uint) (models.Company, bool) {
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu sirkete erisim yetkiniz yok"})
		return models.Company{}, false
	}
	return company, true
}

var _ = json.Marshal
