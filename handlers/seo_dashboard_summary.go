package handlers

import (
	"log"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/services"
)

// ═══════════════════════════════════════════════════
// GET /api/seo/dashboard/summary?site_url=...&range=30
//
// Dashboard SEO metrikleri — seo_metrics_cache tablosundan
// Gerçek GSC verileriyle: organic_traffic, total_keywords, first_page_keywords
//
// Trend = ((Current - Previous) / Previous) * 100
// Current period: son N gün  |  Previous period: ondan önceki N gün
// ═══════════════════════════════════════════════════

// ── Response yapıları ──

type SEOMetricItem struct {
	Value      float64 `json:"value"`
	Trend      float64 `json:"trend"`
	IsPositive bool    `json:"is_positive"`
}

type DashboardSEOResponse struct {
	OrganicTraffic    SEOMetricItem `json:"organic_traffic"`
	TotalKeywords     SEOMetricItem `json:"total_keywords"`
	FirstPageKeywords SEOMetricItem `json:"first_page_keywords"`
	PeriodLabel       string        `json:"period_label"`
	PeriodStart       string        `json:"period_start"`
	PeriodEnd         string        `json:"period_end"`
	HasData           bool          `json:"has_data"`
	ActiveSiteURL     string        `json:"active_site_url"`
	AvailableSites    []SiteOption  `json:"available_sites"`
}

type SiteOption struct {
	SiteURL     string `json:"site_url"`
	DisplayName string `json:"display_name"`
}

func SeoDashboardSummary(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)

	// ── Entegrasyon kontrolü ──
	var integration models.SeoIntegration
	if err := database.DB.Where("company_id = ? AND provider = ?", cid, "google").First(&integration).Error; err != nil {
		c.JSON(http.StatusOK, DashboardSEOResponse{HasData: false, PeriodLabel: "Son 30 Gün"})
		return
	}

	// ── Site listesi ──
	var projects []models.SeoProject
	database.DB.Where("company_id = ? AND is_active = ?", cid, true).Find(&projects)

	var availableSites []SiteOption
	for _, p := range projects {
		availableSites = append(availableSites, SiteOption{SiteURL: p.SiteURL, DisplayName: p.DisplayName})
	}

	// ── Aktif site URL ──
	siteURL := c.Query("site_url")
	if siteURL == "" {
		if len(projects) > 0 {
			siteURL = projects[0].SiteURL
		} else {
			siteURL = integration.SiteURL
		}
	}

	// ── Range parametresi (gün sayısı) ──
	rangeDays := 30
	rangeParam := c.DefaultQuery("range", "30")
	switch rangeParam {
	case "7":
		rangeDays = 7
	case "14":
		rangeDays = 14
	case "60":
		rangeDays = 60
	case "90":
		rangeDays = 90
	}

	periodLabel := "Son " + rangeParam + " Gün"

	// ── Cache'de veri var mı? ──
	var totalRows int64
	database.DB.Table("seo_metrics_caches").
		Where("company_id = ? AND site_url = ?", cid, siteURL).
		Count(&totalRows)

	if totalRows == 0 {
		log.Printf("📊 Dashboard: Cache boş (site=%s), arka planda 90 gün fetch başlatılıyor", siteURL)
		go func() {
			if err := services.FetchAndCacheGSCMetricsForSite(cid, siteURL); err != nil {
				log.Printf("❌ Auto-fetch hatası: %v", err)
			}
		}()
		c.JSON(http.StatusOK, DashboardSEOResponse{
			HasData:        false,
			PeriodLabel:    periodLabel,
			ActiveSiteURL:  siteURL,
			AvailableSites: availableSites,
		})
		return
	}

	// ═══════════════════════════════════════════
	// Tarih aralıkları hesapla
	// Current: (today - rangeDays) → (today - 1)
	// Previous: (today - 2*rangeDays) → (today - rangeDays - 1)
	// ═══════════════════════════════════════════
	now := time.Now()
	currentEnd := now.AddDate(0, 0, -1)                      // dün
	currentStart := currentEnd.AddDate(0, 0, -(rangeDays - 1)) // son N gün
	previousEnd := currentStart.AddDate(0, 0, -1)              // current'ın bir gün öncesi
	previousStart := previousEnd.AddDate(0, 0, -(rangeDays - 1))

	curStartStr := currentStart.Format("2006-01-02")
	curEndStr := currentEnd.Format("2006-01-02")
	prevStartStr := previousStart.Format("2006-01-02")
	prevEndStr := previousEnd.Format("2006-01-02")

	// ═══════════════════════════════════════════
	// 1) ORGANIC TRAFFIC: metric_type='page' → SUM(clicks)
	// ═══════════════════════════════════════════
	var organicCurrent, organicPrev float64
	database.DB.Table("seo_metrics_caches").
		Where("company_id = ? AND site_url = ? AND metric_type = ? AND record_date BETWEEN ? AND ?",
			cid, siteURL, "page", curStartStr, curEndStr).
		Select("COALESCE(SUM(clicks), 0)").Row().Scan(&organicCurrent)

	database.DB.Table("seo_metrics_caches").
		Where("company_id = ? AND site_url = ? AND metric_type = ? AND record_date BETWEEN ? AND ?",
			cid, siteURL, "page", prevStartStr, prevEndStr).
		Select("COALESCE(SUM(clicks), 0)").Row().Scan(&organicPrev)

	// ═══════════════════════════════════════════
	// 2) TOTAL KEYWORDS: metric_type='query' → COUNT(DISTINCT page_url)
	// ═══════════════════════════════════════════
	var keywordsCurrent, keywordsPrev float64
	database.DB.Table("seo_metrics_caches").
		Where("company_id = ? AND site_url = ? AND metric_type = ? AND record_date BETWEEN ? AND ?",
			cid, siteURL, "query", curStartStr, curEndStr).
		Select("COUNT(DISTINCT page_url)").Row().Scan(&keywordsCurrent)

	database.DB.Table("seo_metrics_caches").
		Where("company_id = ? AND site_url = ? AND metric_type = ? AND record_date BETWEEN ? AND ?",
			cid, siteURL, "query", prevStartStr, prevEndStr).
		Select("COUNT(DISTINCT page_url)").Row().Scan(&keywordsPrev)

	// ═══════════════════════════════════════════
	// 3) FIRST PAGE KEYWORDS: metric_type='query' AND position <= 10
	//    Her query'nin ORTALAMA pozisyonu <= 10 olanları say
	// ═══════════════════════════════════════════
	var firstPageCurrent, firstPagePrev float64

	// Current dönem: query bazında ortalama pozisyon <= 10 olan distinct query sayısı
	database.DB.Table("seo_metrics_caches").
		Where("company_id = ? AND site_url = ? AND metric_type = ? AND record_date BETWEEN ? AND ?",
			cid, siteURL, "query", curStartStr, curEndStr).
		Select("COUNT(DISTINCT CASE WHEN position <= 10.0 THEN page_url END)").
		Row().Scan(&firstPageCurrent)

	database.DB.Table("seo_metrics_caches").
		Where("company_id = ? AND site_url = ? AND metric_type = ? AND record_date BETWEEN ? AND ?",
			cid, siteURL, "query", prevStartStr, prevEndStr).
		Select("COUNT(DISTINCT CASE WHEN position <= 10.0 THEN page_url END)").
		Row().Scan(&firstPagePrev)

	// ═══════════════════════════════════════════
	// TREND HESAPLAMA: ((Current - Previous) / Previous) * 100
	// ═══════════════════════════════════════════
	calcTrend := func(current, previous float64) SEOMetricItem {
		trend := 0.0
		if previous > 0 {
			trend = ((current - previous) / previous) * 100
			trend = math.Round(trend*10) / 10 // 1 ondalık
		} else if current > 0 {
			trend = 100.0 // önceki dönem 0 ama şimdi var → %100 artış
		}
		return SEOMetricItem{
			Value:      current,
			Trend:      trend,
			IsPositive: trend >= 0,
		}
	}

	c.JSON(http.StatusOK, DashboardSEOResponse{
		OrganicTraffic:    calcTrend(organicCurrent, organicPrev),
		TotalKeywords:     calcTrend(keywordsCurrent, keywordsPrev),
		FirstPageKeywords: calcTrend(firstPageCurrent, firstPagePrev),
		PeriodLabel:       periodLabel,
		PeriodStart:       curStartStr,
		PeriodEnd:         curEndStr,
		HasData:           true,
		ActiveSiteURL:     siteURL,
		AvailableSites:    availableSites,
	})
}

// ═══════════════════════════════════════════════════
// POST /api/seo/dashboard/sync-cache?site_url=...
// Manuel GSC veri senkronizasyonu
// ═══════════════════════════════════════════════════

func SeoDashboardSyncCache(c *gin.Context) {
	companyID, _ := c.Get("companyID")
	cid := companyID.(uint)
	siteURL := c.Query("site_url")

	if siteURL != "" {
		log.Printf("🔄 Manuel sync (tek site): company_id=%d, site=%s", cid, siteURL)
		if err := services.FetchAndCacheGSCMetricsForSite(cid, siteURL); err != nil {
			log.Printf("❌ Sync hatası: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		log.Printf("🔄 Manuel sync (tüm siteler): company_id=%d", cid)
		if err := services.FetchAndCacheGSCMetrics(cid); err != nil {
			log.Printf("❌ Sync hatası: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "GSC verileri başarıyla senkronize edildi"})
}
