package handlers

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/services"
)

// ═══════════════════════════════════════════════════
// GET /api/seo/projects
// Kullanıcının GSC projelerini (site'ları) döner + senkronize eder
// ═══════════════════════════════════════════════════

func SeoListProjects(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	// Entegrasyon kontrolü
	var integration models.SeoIntegration
	if err := database.DB.Where("company_id = ? AND provider = ?", cid, "google").First(&integration).Error; err != nil {
		c.JSON(http.StatusOK, gin.H{"projects": []interface{}{}, "connected": false})
		return
	}

	// GSC'den site listesini senkronize et
	projects, err := services.SyncGSCSites(cid, &integration)
	if err != nil {
		log.Printf("⚠️ GSC site sync hatası: %v", err)
		// DB'deki mevcut projeleri dön
		database.DB.Where("company_id = ?", cid).Find(&projects)
	}

	c.JSON(http.StatusOK, gin.H{
		"projects":  projects,
		"connected": true,
	})
}

// ═══════════════════════════════════════════════════
// POST /api/seo/projects/:id/sync
// Belirli bir proje için GSC verilerini çeker ve DB'ye yazar
// ═══════════════════════════════════════════════════

func SeoSyncProject(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz project_id"})
		return
	}

	// Proje kontrolü
	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	// Entegrasyon
	var integration models.SeoIntegration
	if err := database.DB.Where("company_id = ? AND provider = ?", cid, "google").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "GSC entegrasyonu yok"})
		return
	}

	// Period parametresi
	period := c.DefaultQuery("period", "28d")
	days := 28
	switch period {
	case "7d":
		days = 7
	case "90d":
		days = 90
	case "12m":
		days = 365
	}

	log.Printf("🔄 SYNC BAŞLIYOR: project_id=%d, site=%s, period=%s", project.ID, project.SiteURL, period)

	// GSC servisi oluştur ve veri çek
	gscSvc, err := services.NewGSCService(&project, &integration)
	if err != nil {
		log.Printf("❌ GSC servis oluşturulamadı: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "GSC servisi oluşturulamadı: " + err.Error()})
		return
	}

	if err := gscSvc.SyncAllData(period, days); err != nil {
		log.Printf("❌ SyncAllData hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Veri senkronizasyon hatası: " + err.Error()})
		return
	}

	// ══════════════════════════════════════════
	// DOĞRULAMA: Snapshot gerçekten yazıldı mı?
	// ══════════════════════════════════════════
	var snap models.SeoProjectSnapshot
	snapExists := database.DB.Where("project_id = ? AND period_label = ?", projectID, period).First(&snap).Error == nil

	if !snapExists {
		log.Printf("⚠️ Snapshot yazılamamış! Günlük veriden force-create yapılıyor...")
		// Günlük verilerden snapshot oluştur
		var dailyRows []models.SeoPerformanceDaily
		database.DB.Where("project_id = ?", projectID).Order("date ASC").Find(&dailyRows)

		if len(dailyRows) > days {
			dailyRows = dailyRows[len(dailyRows)-days:]
		}

		if len(dailyRows) > 0 {
			var tc, ti, tPos float64
			for _, d := range dailyRows {
				tc += d.Clicks
				ti += d.Impressions
				tPos += d.Position
			}
			cnt := float64(len(dailyRows))
			avgCTR := 0.0
			if ti > 0 {
				avgCTR = tc / ti
			}
			avgPos := 0.0
			if cnt > 0 {
				avgPos = tPos / cnt
			}

			snapDate := time.Now().Format("2006-01-02")
			forceSnap := models.SeoProjectSnapshot{
				ProjectID:        uint(projectID),
				PeriodLabel:      period,
				SnapshotDate:     &snapDate,
				StartDate:        dailyRows[0].Date,
				EndDate:          dailyRows[len(dailyRows)-1].Date,
				TotalClicks:      tc,
				TotalImpressions: ti,
				AvgCTR:           avgCTR,
				AvgPosition:      avgPos,
			}

			if err := database.DB.Create(&forceSnap).Error; err != nil {
				log.Printf("❌ Force snapshot CREATE hatası: %v", err)
			} else {
				log.Printf("✅ Force snapshot oluşturuldu: clicks=%.0f, impressions=%.0f", tc, ti)
				snapExists = true
			}
		}
	}

	// Discovery metrics kontrolü
	var disc models.SeoDiscoveryMetrics
	discExists := database.DB.Where("project_id = ? AND period_label = ?", projectID, period).First(&disc).Error == nil

	log.Printf("🔄 SYNC TAMAMLANDI: project_id=%d, snapshot=%v, discovery=%v", project.ID, snapExists, discExists)

	// AI Summary üret (arka planda)
	go func() {
		_ = gscSvc.GenerateAISummary(project.ID, period)
	}()

	c.JSON(http.StatusOK, gin.H{
		"message":          "Senkronizasyon tamamlandı",
		"project_id":       project.ID,
		"period":           period,
		"snapshot_saved":   snapExists,
		"discovery_saved":  discExists,
	})
}

// ═══════════════════════════════════════════════════
// GET /api/seo/projects/:id/overview
// Dashboard Genel Bakış verilerini döner
// ═══════════════════════════════════════════════════

func SeoProjectOverview(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	projectID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz project_id"})
		return
	}

	var project models.SeoProject
	if err := database.DB.Where("id = ? AND company_id = ?", projectID, cid).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Proje bulunamadı"})
		return
	}

	period := c.DefaultQuery("period", "28d")

	// 1) Snapshot (KPI kartları + brand)
	var snapshot models.SeoProjectSnapshot
	snapFound := database.DB.Where("project_id = ? AND period_label = ?", projectID, period).
		Order("seo_project_snapshots.id DESC").First(&snapshot).Error == nil

	// 2) Günlük trend verisi
	var dailyRows []models.SeoPerformanceDaily
	database.DB.Where("project_id = ?", projectID).Order("date ASC").Find(&dailyRows)

	// Period'a göre filtrele
	days := 28
	switch period {
	case "7d":
		days = 7
	case "90d":
		days = 90
	case "12m":
		days = 365
	}
	if len(dailyRows) > days {
		dailyRows = dailyRows[len(dailyRows)-days:]
	}

	// Snapshot yoksa günlük verilerden hesapla (fallback)
	if !snapFound && len(dailyRows) > 0 {
		log.Printf("⚠️ Snapshot bulunamadı, günlük veriden hesaplanıyor: project=%d, period=%s, daily_rows=%d", projectID, period, len(dailyRows))
		var tc, ti, tPos float64
		for _, d := range dailyRows {
			tc += d.Clicks
			ti += d.Impressions
			tPos += d.Position
		}
		cnt := float64(len(dailyRows))
		snapshot.TotalClicks = tc
		snapshot.TotalImpressions = ti
		if ti > 0 {
			snapshot.AvgCTR = tc / ti
		}
		if cnt > 0 {
			snapshot.AvgPosition = tPos / cnt
		}

		// Snapshot'ı DB'ye de yaz ki bir sonraki yüklemede direk okunabilsin
		snapDate2 := time.Now().Format("2006-01-02")
		forceSnap := models.SeoProjectSnapshot{
			ProjectID:        uint(projectID),
			PeriodLabel:      period,
			SnapshotDate:     &snapDate2,
			StartDate:        dailyRows[0].Date,
			EndDate:          dailyRows[len(dailyRows)-1].Date,
			TotalClicks:      tc,
			TotalImpressions: ti,
			AvgCTR:           snapshot.AvgCTR,
			AvgPosition:      snapshot.AvgPosition,
		}
		if err := database.DB.Create(&forceSnap).Error; err != nil {
			log.Printf("⚠️ Overview fallback snapshot yazılamadı: %v", err)
		} else {
			log.Printf("✅ Overview fallback snapshot DB'ye yazıldı: clicks=%.0f", tc)
		}
	}

	// Hiç veri yoksa arka planda otomatik sync başlat
	if !snapFound && len(dailyRows) == 0 {
		log.Printf("🔄 Hiç veri yok, arka planda otomatik sync başlatılıyor: project=%d", projectID)
		go func() {
			var integration models.SeoIntegration
			if err := database.DB.Where("company_id = ? AND provider = ?", cid, "google").First(&integration).Error; err != nil {
				return
			}
			gscSvc, err := services.NewGSCService(&project, &integration)
			if err != nil {
				log.Printf("❌ Auto-sync GSC servis hatası: %v", err)
				return
			}
			if err := gscSvc.SyncAllData(period, days); err != nil {
				log.Printf("❌ Auto-sync hatası: %v", err)
			} else {
				log.Printf("✅ Auto-sync tamamlandı: project=%d", projectID)
			}
		}()
	}

	// Trend verisini frontend formatına çevir
	type TrendPoint struct {
		Date        string  `json:"date"`
		Clicks      float64 `json:"clicks"`
		Impressions float64 `json:"impressions"`
	}
	var trend []TrendPoint
	for _, d := range dailyRows {
		// Tarihi kısalt: "2026-04-10" → "4/10"
		shortDate := d.Date
		if len(d.Date) == 10 {
			month := d.Date[5:7]
			day := d.Date[8:10]
			// Baştaki sıfırları kaldır
			m, _ := strconv.Atoi(month)
			dd, _ := strconv.Atoi(day)
			shortDate = strconv.Itoa(m) + "/" + strconv.Itoa(dd)
		}
		trend = append(trend, TrendPoint{Date: shortDate, Clicks: d.Clicks, Impressions: d.Impressions})
	}

	// 3) Discovery Metrics (kırılımlar, tablolar)
	var discovery models.SeoDiscoveryMetrics
	database.DB.Where("project_id = ? AND period_label = ?", projectID, period).First(&discovery)

	// Delta hesaplamaları
	clickDelta := 0.0
	if snapshot.PrevClicks > 0 {
		clickDelta = ((snapshot.TotalClicks - snapshot.PrevClicks) / snapshot.PrevClicks) * 100
	}
	impressionDelta := 0.0
	if snapshot.PrevImpressions > 0 {
		impressionDelta = ((snapshot.TotalImpressions - snapshot.PrevImpressions) / snapshot.PrevImpressions) * 100
	}
	ctrDelta := (snapshot.AvgCTR - snapshot.PrevCTR) * 100
	posDelta := snapshot.PrevPosition - snapshot.AvgPosition // Pozitif = iyileşme

	c.JSON(http.StatusOK, gin.H{
		"project": gin.H{
			"id":           project.ID,
			"site_url":     project.SiteURL,
			"display_name": project.DisplayName,
			"last_sync_at": project.LastSyncAt,
		},
		"kpi": gin.H{
			"total_clicks":      snapshot.TotalClicks,
			"total_impressions": snapshot.TotalImpressions,
			"avg_ctr":           snapshot.AvgCTR,
			"avg_position":      snapshot.AvgPosition,
			"click_delta":       clickDelta,
			"impression_delta":  impressionDelta,
			"ctr_delta":         ctrDelta,
			"position_delta":    posDelta,
		},
		"brand": gin.H{
			"brand_clicks":      snapshot.BrandClicks,
			"non_brand_clicks":  snapshot.NonBrandClicks,
			"brand_ctr":         snapshot.BrandCTR,
			"non_brand_ctr":     snapshot.NonBrandCTR,
		},
		"trend":       trend,
		"ai_summary":  snapshot.AISummaryText,
		"device":      discovery.DeviceBreakdown,
		"country":     discovery.CountryBreakdown,
		"search_type": discovery.SearchTypeBreakdown,
		"top_queries": discovery.TopQueries,
		"top_pages":   discovery.TopPages,
		"ctr_opportunities": discovery.CTROpportunities,
		"rising_queries": discovery.RisingQueries,
		"falling_queries": discovery.FallingQueries,
		"period":      period,
	})
}
