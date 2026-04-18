package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"
)

// ═══════════════════════════════════════════════════
// GSC Metrics Fetcher — seo_metrics_cache tablosuna
// İlk çalışmada 90 gün, sonrasında incremental çekiş
// ═══════════════════════════════════════════════════

// FetchAndCacheGSCMetrics — Şirketin TÜM GSC projeleri için veri çeker.
// Cache boşsa 90 gün (trend hesabı için), doluysa sadece dünün verisi.
func FetchAndCacheGSCMetrics(companyID uint) error {
	svc, err := createGSCService(companyID)
	if err != nil {
		return err
	}

	projects := getActiveProjects(companyID)
	if len(projects) == 0 {
		return fmt.Errorf("aktif proje bulunamadı: company_id=%d", companyID)
	}

	for _, project := range projects {
		siteURL := project.SiteURL

		// Cache'de bu site için veri var mı kontrol et
		var count int64
		database.DB.Table("seo_metrics_caches").
			Where("company_id = ? AND site_url = ?", companyID, siteURL).
			Count(&count)

		if count == 0 {
			// ── İLK ÇALIŞTIRMA: 90 gün geçmiş veri çek ──
			log.Printf("🆕 İlk çalıştırma — 90 gün geçmiş veri çekiliyor: company_id=%d, site=%s", companyID, siteURL)
			if err := fetchDateRange(svc, siteURL, companyID, 90); err != nil {
				log.Printf("  ⚠️ 90 gün fetch hatası (%s): %v", siteURL, err)
			}
		} else {
			// ── INCREMENTAL: Son 3 günü çek (gecikme payı) ──
			log.Printf("🔄 Incremental fetch — son 3 gün: company_id=%d, site=%s", companyID, siteURL)
			if err := fetchDateRange(svc, siteURL, companyID, 3); err != nil {
				log.Printf("  ⚠️ Incremental fetch hatası (%s): %v", siteURL, err)
			}
		}
	}

	return nil
}

// FetchAndCacheGSCMetricsForSite — Belirli bir site için 90 gün veri çeker.
// Dashboard sync butonundan çağrılır.
func FetchAndCacheGSCMetricsForSite(companyID uint, targetSiteURL string) error {
	svc, err := createGSCService(companyID)
	if err != nil {
		return err
	}

	// Her zaman 90 gün çek (sync butonu elle basılıyor)
	log.Printf("📊 Manuel sync — 90 gün: company_id=%d, site=%s", companyID, targetSiteURL)
	return fetchDateRange(svc, targetSiteURL, companyID, 90)
}

// fetchDateRange — Belirli gün sayısı kadar geçmişi page + query olarak çeker
func fetchDateRange(svc *searchconsole.Service, siteURL string, companyID uint, days int) error {
	endDate := time.Now().AddDate(0, 0, -1) // dün (GSC verisi 1 gün gecikir)
	startDate := endDate.AddDate(0, 0, -(days - 1))
	start := startDate.Format("2006-01-02")
	end := endDate.Format("2006-01-02")

	log.Printf("  📅 %s → %s (%d gün)", start, end, days)

	// Page dimension
	pageCount, err := fetchAndUpsertDimension(svc, siteURL, companyID, "page", start, end)
	if err != nil {
		log.Printf("    ⚠️ Page hatası: %v", err)
	} else {
		log.Printf("    ✅ Page: %d kayıt", pageCount)
	}

	// Query dimension
	queryCount, err := fetchAndUpsertDimension(svc, siteURL, companyID, "query", start, end)
	if err != nil {
		log.Printf("    ⚠️ Query hatası: %v", err)
	} else {
		log.Printf("    ✅ Query: %d kayıt", queryCount)
	}

	log.Printf("  ✅ Toplam: pages=%d, queries=%d", pageCount, queryCount)
	return nil
}

// fetchAndUpsertDimension — GSC'den date+dimension kırılımlı veri çeker, DB'ye upsert yapar
func fetchAndUpsertDimension(svc *searchconsole.Service, siteURL string, companyID uint, dimension, start, end string) (int, error) {
	req := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:  start,
		EndDate:    end,
		Dimensions: []string{"date", dimension},
		RowLimit:   25000,
	}

	resp, err := svc.Searchanalytics.Query(siteURL, req).Do()
	if err != nil {
		return 0, fmt.Errorf("%s query hatası: %v", dimension, err)
	}

	count := 0
	metricType := dimension

	for _, row := range resp.Rows {
		if len(row.Keys) < 2 {
			continue
		}

		recordDate := row.Keys[0]
		pageURL := row.Keys[1]

		var existing models.SeoMetricsCache
		result := database.DB.Where(
			"company_id = ? AND site_url = ? AND page_url = ? AND metric_type = ? AND record_date = ?",
			companyID, siteURL, pageURL, metricType, recordDate,
		).First(&existing)

		if result.Error != nil {
			database.DB.Create(&models.SeoMetricsCache{
				CompanyID:   companyID,
				SiteURL:     siteURL,
				PageURL:     pageURL,
				MetricType:  metricType,
				Clicks:      row.Clicks,
				Impressions: row.Impressions,
				CTR:         row.Ctr,
				Position:    row.Position,
				RecordDate:  recordDate,
			})
		} else {
			existing.Clicks = row.Clicks
			existing.Impressions = row.Impressions
			existing.CTR = row.Ctr
			existing.Position = row.Position
			database.DB.Save(&existing)
		}
		count++
	}

	return count, nil
}

// ═══════════════════════════════════════════════════
// Yardımcı fonksiyonlar
// ═══════════════════════════════════════════════════

func createGSCService(companyID uint) (*searchconsole.Service, error) {
	var integration models.SeoIntegration
	if err := database.DB.Where("company_id = ? AND provider = ?", companyID, "google").First(&integration).Error; err != nil {
		return nil, fmt.Errorf("GSC entegrasyonu bulunamadı: company_id=%d", companyID)
	}

	cfg := &oauth2.Config{
		ClientID:     config.Get("GOOGLE_CLIENT_ID"),
		ClientSecret: config.Get("GOOGLE_CLIENT_SECRET"),
		Scopes:       []string{"https://www.googleapis.com/auth/webmasters.readonly"},
		Endpoint:     google.Endpoint,
	}

	token := &oauth2.Token{
		AccessToken:  integration.AccessToken,
		RefreshToken: integration.RefreshToken,
		Expiry:       integration.TokenExpiry,
		TokenType:    "Bearer",
	}

	tokenSource := cfg.TokenSource(context.Background(), token)

	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("GSC token yenileme hatası: %v", err)
	}
	if newToken.AccessToken != integration.AccessToken {
		integration.AccessToken = newToken.AccessToken
		integration.TokenExpiry = newToken.Expiry
		if newToken.RefreshToken != "" {
			integration.RefreshToken = newToken.RefreshToken
		}
		database.DB.Save(&integration)
		log.Printf("🔄 GSC token yenilendi: company_id=%d", companyID)
	}

	client := oauth2.NewClient(context.Background(), tokenSource)
	svc, err := searchconsole.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("search console servis hatası: %v", err)
	}

	return svc, nil
}

func getActiveProjects(companyID uint) []models.SeoProject {
	var projects []models.SeoProject
	database.DB.Where("company_id = ? AND is_active = ?", companyID, true).Find(&projects)

	if len(projects) == 0 {
		// Fallback: integration'daki default site
		var integration models.SeoIntegration
		if err := database.DB.Where("company_id = ? AND provider = ?", companyID, "google").First(&integration).Error; err == nil && integration.SiteURL != "" {
			projects = []models.SeoProject{{SiteURL: integration.SiteURL}}
		}
	}

	return projects
}

// SyncAllCompaniesGSCMetrics — Cron job (gece 03:30)
func SyncAllCompaniesGSCMetrics() {
	var integrations []models.SeoIntegration
	database.DB.Where("provider = ?", "google").Find(&integrations)

	if len(integrations) == 0 {
		log.Println("  ℹ️ Hiç GSC entegrasyonu yok, atlanıyor")
		return
	}

	log.Printf("📊 %d şirket için GSC Metrics Cache sync başlıyor...", len(integrations))

	for _, integration := range integrations {
		if err := FetchAndCacheGSCMetrics(integration.CompanyID); err != nil {
			log.Printf("  ❌ company_id=%d: %v", integration.CompanyID, err)
		} else {
			log.Printf("  ✅ company_id=%d: sync tamamlandı", integration.CompanyID)
		}
	}

	log.Println("✅ Tüm GSC Metrics Cache sync tamamlandı")
}
