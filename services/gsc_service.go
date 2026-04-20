package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"

	"github.com/kullaniciadiniz/dio-panel-backend/config"
)

// ═══════════════════════════════════════════════════
// GSC Service — Google Search Console veri çekme servisi
// ═══════════════════════════════════════════════════

// GSCService, GSC API ile iletişim kurar ve verileri DB'ye kaydeder
type GSCService struct {
	svc       *searchconsole.Service
	project   *models.SeoProject
	siteURL   string
	brandName string // Marka adı (Brand/Non-Brand ayrımı için)
}

// NewGSCService — Verilen proje için GSC servisini oluşturur
// Integration tokeni ile OAuth2 client üretir
func NewGSCService(project *models.SeoProject, integration *models.SeoIntegration) (*GSCService, error) {
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

	// Token yenilendiyse DB'ye geri yaz
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("token yenileme hatası: %v", err)
	}
	if newToken.AccessToken != integration.AccessToken {
		integration.AccessToken = newToken.AccessToken
		integration.TokenExpiry = newToken.Expiry
		if newToken.RefreshToken != "" {
			integration.RefreshToken = newToken.RefreshToken
		}
		database.DB.Save(integration)
		log.Printf("🔄 GSC token yenilendi: project_id=%d", project.ID)
	}

	client := oauth2.NewClient(context.Background(), tokenSource)
	svc, err := searchconsole.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("search console servis hatası: %v", err)
	}

	// Marka adını company'den al
	var company models.Company
	database.DB.First(&company, project.CompanyID)

	return &GSCService{
		svc:       svc,
		project:   project,
		siteURL:   project.SiteURL,
		brandName: strings.ToLower(company.Name),
	}, nil
}

// ═══════════════════════════════════════════════════
// SyncAllData — Tüm verileri belirtilen dönem için senkronize eder
// ═══════════════════════════════════════════════════

func (g *GSCService) SyncAllData(periodLabel string, days int) error {
	endDate := time.Now().AddDate(0, 0, -1)
	startDate := endDate.AddDate(0, 0, -(days - 1))
	start := startDate.Format("2006-01-02")
	end := endDate.Format("2006-01-02")

	log.Printf("📊 GSC Sync başlıyor: project=%d, site=%s, period=%s (%s → %s)",
		g.project.ID, g.siteURL, periodLabel, start, end)

	// Önceki dönem (karşılaştırma için)
	prevEnd := startDate.AddDate(0, 0, -1)
	prevStart := prevEnd.AddDate(0, 0, -(days - 1))
	prevStartStr := prevStart.Format("2006-01-02")
	prevEndStr := prevEnd.Format("2006-01-02")

	// İstek A: Trend verisi (date dimension)
	trendData, totalClicks, totalImpressions, avgCTR, avgPosition, err := g.fetchTrendData(start, end)
	if err != nil {
		return fmt.Errorf("İstek A hatası: %v", err)
	}
	log.Printf("  ✅ İstek A: %d günlük trend verisi, toplam %d tık", len(trendData), int(totalClicks))

	// Günlük verileri DB'ye yaz
	g.upsertDailyData(trendData)

	// Önceki dönem özet (karşılaştırma)
	_, prevClicks, prevImpressions, prevCTR, prevPosition, _ := g.fetchTrendData(prevStartStr, prevEndStr)

	// İstek B: Kelimeler ve Brand/Non-Brand
	brandClicks, nonBrandClicks, brandCTR, nonBrandCTR, topQueries, err := g.fetchQueryData(start, end)
	if err != nil {
		log.Printf("  ⚠️ İstek B hatası (devam ediliyor): %v", err)
	} else {
		log.Printf("  ✅ İstek B: brand=%d tık, non-brand=%d tık, %d sorgu", int(brandClicks), int(nonBrandClicks), len(topQueries))
	}

	// İstek C: Sayfa performansı + CTR fırsatları
	topPages, ctrOpportunities, err := g.fetchPageData(start, end)
	if err != nil {
		log.Printf("  ⚠️ İstek C hatası (devam ediliyor): %v", err)
	} else {
		log.Printf("  ✅ İstek C: %d sayfa, %d CTR fırsatı", len(topPages), len(ctrOpportunities))
	}

	// İstek D: Cihaz ve Ülke kırılımı
	deviceBreakdown, countryBreakdown, searchTypeBreakdown, err := g.fetchDimensionData(start, end)
	if err != nil {
		log.Printf("  ⚠️ İstek D hatası (devam ediliyor): %v", err)
	} else {
		log.Printf("  ✅ İstek D: %d cihaz, %d ülke, %d arama türü", len(deviceBreakdown), len(countryBreakdown), len(searchTypeBreakdown))
	}

	// 7 günlük ivme verisi (yükselen/düşen sorgular)
	risingQueries, fallingQueries := g.fetchTrendQueries(start, end, days)

	// ── Snapshot yaz ──
	g.upsertSnapshot(periodLabel, start, end, SnapshotData{
		TotalClicks: totalClicks, TotalImpressions: totalImpressions,
		AvgCTR: avgCTR, AvgPosition: avgPosition,
		PrevClicks: prevClicks, PrevImpressions: prevImpressions,
		PrevCTR: prevCTR, PrevPosition: prevPosition,
		BrandClicks: brandClicks, NonBrandClicks: nonBrandClicks,
		BrandCTR: brandCTR, NonBrandCTR: nonBrandCTR,
	})

	// ── Discovery Metrics yaz ──
	g.upsertDiscoveryMetrics(periodLabel, DiscoveryData{
		DeviceBreakdown:     deviceBreakdown,
		CountryBreakdown:    countryBreakdown,
		SearchTypeBreakdown: searchTypeBreakdown,
		TopQueries:          topQueries,
		TopPages:            topPages,
		CTROpportunities:    ctrOpportunities,
		RisingQueries:       risingQueries,
		FallingQueries:      fallingQueries,
	})

	// Son sync zamanını güncelle
	now := time.Now()
	g.project.LastSyncAt = &now
	database.DB.Save(g.project)

	log.Printf("✅ GSC Sync tamamlandı: project=%d, period=%s", g.project.ID, periodLabel)
	return nil
}

// ═══════════════════════════════════════════════════
// İstek A: Trend verisi — dimensions: ["date"]
// ═══════════════════════════════════════════════════

type DailyRow struct {
	Date        string  `json:"date"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

func (g *GSCService) fetchTrendData(start, end string) ([]DailyRow, float64, float64, float64, float64, error) {
	req := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:  start,
		EndDate:    end,
		Dimensions: []string{"date"},
		RowLimit:   25000,
	}

	resp, err := g.svc.Searchanalytics.Query(g.siteURL, req).Do()
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}

	var rows []DailyRow
	var totalClicks, totalImpressions, sumCTR, sumPosition float64

	for _, row := range resp.Rows {
		if len(row.Keys) == 0 {
			continue
		}
		d := DailyRow{
			Date:        row.Keys[0],
			Clicks:      row.Clicks,
			Impressions: row.Impressions,
			CTR:         row.Ctr,
			Position:    row.Position,
		}
		rows = append(rows, d)
		totalClicks += row.Clicks
		totalImpressions += row.Impressions
		sumCTR += row.Ctr
		sumPosition += row.Position
	}

	n := float64(len(rows))
	var avgCTR, avgPosition float64
	if n > 0 {
		avgCTR = sumCTR / n
		avgPosition = sumPosition / n
	}

	return rows, totalClicks, totalImpressions, avgCTR, avgPosition, nil
}

func (g *GSCService) upsertDailyData(rows []DailyRow) {
	for _, row := range rows {
		var existing models.SeoPerformanceDaily
		result := database.DB.Where("project_id = ? AND date = ?", g.project.ID, row.Date).First(&existing)
		if result.Error != nil {
			// Yeni kayıt
			database.DB.Create(&models.SeoPerformanceDaily{
				ProjectID: g.project.ID, Date: row.Date,
				Clicks: row.Clicks, Impressions: row.Impressions,
				CTR: row.CTR, Position: row.Position,
			})
		} else {
			// Güncelle
			existing.Clicks = row.Clicks
			existing.Impressions = row.Impressions
			existing.CTR = row.CTR
			existing.Position = row.Position
			database.DB.Save(&existing)
		}
	}
}

// ═══════════════════════════════════════════════════
// İstek B: Kelimeler — dimensions: ["query"]
// ═══════════════════════════════════════════════════

func (g *GSCService) fetchQueryData(start, end string) (float64, float64, float64, float64, []map[string]interface{}, error) {
	req := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:  start,
		EndDate:    end,
		Dimensions: []string{"query"},
		RowLimit:   5000,
	}

	resp, err := g.svc.Searchanalytics.Query(g.siteURL, req).Do()
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}

	var brandClicks, nonBrandClicks float64
	var brandImpressions, nonBrandImpressions float64
	var topQueries []map[string]interface{}

	for i, row := range resp.Rows {
		if len(row.Keys) == 0 {
			continue
		}
		query := row.Keys[0]
		isBrand := strings.Contains(strings.ToLower(query), g.brandName)

		if isBrand {
			brandClicks += row.Clicks
			brandImpressions += row.Impressions
		} else {
			nonBrandClicks += row.Clicks
			nonBrandImpressions += row.Impressions
		}

		// İlk 100 sorguyu topla (UI tablo için)
		if i < 100 {
			qType := "Non-Brand"
			if isBrand {
				qType = "Brand"
			}
			topQueries = append(topQueries, map[string]interface{}{
				"query":       query,
				"type":        qType,
				"clicks":      row.Clicks,
				"impressions": row.Impressions,
				"ctr":         math.Round(row.Ctr*1000) / 10,  // %3.1 formatı
				"pos":         math.Round(row.Position*10) / 10,
			})
		}
	}

	var brandCTR, nonBrandCTR float64
	if brandImpressions > 0 {
		brandCTR = brandClicks / brandImpressions
	}
	if nonBrandImpressions > 0 {
		nonBrandCTR = nonBrandClicks / nonBrandImpressions
	}

	return brandClicks, nonBrandClicks, brandCTR, nonBrandCTR, topQueries, nil
}

// ═══════════════════════════════════════════════════
// İstek C: Sayfa performansı — dimensions: ["page"]
// ═══════════════════════════════════════════════════

func (g *GSCService) fetchPageData(start, end string) ([]map[string]interface{}, []map[string]interface{}, error) {
	req := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:  start,
		EndDate:    end,
		Dimensions: []string{"page"},
		RowLimit:   5000,
	}

	resp, err := g.svc.Searchanalytics.Query(g.siteURL, req).Do()
	if err != nil {
		return nil, nil, err
	}

	var topPages []map[string]interface{}
	var ctrOpportunities []map[string]interface{}

	for _, row := range resp.Rows {
		if len(row.Keys) == 0 {
			continue
		}

		// URL'den alan adını çıkar, sadece path göster
		page := row.Keys[0]
		if idx := strings.Index(page, "//"); idx > 0 {
			afterProto := page[idx+2:]
			if slashIdx := strings.Index(afterProto, "/"); slashIdx >= 0 {
				page = afterProto[slashIdx:]
			}
		}

		entry := map[string]interface{}{
			"page":        page,
			"clicks":      row.Clicks,
			"impressions": row.Impressions,
			"ctr":         math.Round(row.Ctr*1000) / 10,
			"pos":         math.Round(row.Position*10) / 10,
		}

		if len(topPages) < 100 {
			topPages = append(topPages, entry)
		}

		// CTR Fırsatı: Gösterim > 5000 ve CTR < 2%
		if row.Impressions > 5000 && row.Ctr < 0.02 {
			potentialGain := row.Impressions * (0.035 - row.Ctr) // hedef %3.5 CTR
			ctrOpportunities = append(ctrOpportunities, map[string]interface{}{
				"query":  page,
				"detail": fmt.Sprintf("%.0fK göst. · CTR %%%.1f · Poz: %.1f", row.Impressions/1000, row.Ctr*100, row.Position),
				"gain":   fmt.Sprintf("+%.0f tık", potentialGain),
				"impressions": row.Impressions,
				"ctr":    row.Ctr,
				"pos":    row.Position,
			})
		}
	}

	// CTR fırsatlarını potansiyel kazanıma göre sırala
	sort.Slice(ctrOpportunities, func(i, j int) bool {
		iImp, _ := ctrOpportunities[i]["impressions"].(float64)
		jImp, _ := ctrOpportunities[j]["impressions"].(float64)
		return iImp > jImp
	})

	// En fazla 10 fırsat
	if len(ctrOpportunities) > 10 {
		ctrOpportunities = ctrOpportunities[:10]
	}

	return topPages, ctrOpportunities, nil
}

// ═══════════════════════════════════════════════════
// İstek D: Cihaz + Ülke + Arama Türü — 3 ayrı istek
// ═══════════════════════════════════════════════════

func (g *GSCService) fetchDimensionData(start, end string) ([]map[string]interface{}, []map[string]interface{}, []map[string]interface{}, error) {
	// D1: Cihaz kırılımı
	deviceResp, err := g.svc.Searchanalytics.Query(g.siteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: start, EndDate: end, Dimensions: []string{"device"},
	}).Do()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("device query hatası: %v", err)
	}

	var devices []map[string]interface{}
	var deviceTotal float64
	for _, r := range deviceResp.Rows {
		deviceTotal += r.Clicks
	}
	deviceMap := map[string]string{"MOBILE": "Mobil", "DESKTOP": "Masaüstü", "TABLET": "Tablet"}
	colorMap := map[string]string{"MOBILE": "#22C55E", "DESKTOP": "#3B82F6", "TABLET": "#A855F7"}
	for _, r := range deviceResp.Rows {
		if len(r.Keys) == 0 {
			continue
		}
		name := deviceMap[r.Keys[0]]
		if name == "" {
			name = r.Keys[0]
		}
		pct := 0.0
		if deviceTotal > 0 {
			pct = math.Round(r.Clicks / deviceTotal * 100)
		}
		devices = append(devices, map[string]interface{}{
			"name":  name,
			"value": r.Clicks,
			"pct":   pct,
			"color": colorMap[r.Keys[0]],
		})
	}

	// D2: Ülke kırılımı
	countryResp, err := g.svc.Searchanalytics.Query(g.siteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: start, EndDate: end, Dimensions: []string{"country"}, RowLimit: 10,
	}).Do()
	if err != nil {
		log.Printf("  ⚠️ country query hatası: %v", err)
	}

	var countries []map[string]interface{}
	var countryTotal float64
	if countryResp != nil {
		for _, r := range countryResp.Rows {
			countryTotal += r.Clicks
		}
		for _, r := range countryResp.Rows {
			if len(r.Keys) == 0 {
				continue
			}
			pct := 0.0
			if countryTotal > 0 {
				pct = math.Round(r.Clicks / countryTotal * 100)
			}
			countries = append(countries, map[string]interface{}{
				"code":  strings.ToUpper(r.Keys[0][:min(3, len(r.Keys[0]))]),
				"name":  countryCodeToName(r.Keys[0]),
				"value": r.Clicks,
				"pct":   pct,
			})
		}
	}

	// D3: Arama türü kırılımı
	searchTypeResp, err := g.svc.Searchanalytics.Query(g.siteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: start, EndDate: end, Dimensions: []string{"searchAppearance"},
	}).Do()

	var searchTypes []map[string]interface{}
	stColorMap := map[string]string{"WEB": "#3B82F6", "IMAGE": "#F59E0B", "VIDEO": "#EF4444", "NEWS": "#8B5CF6"}
	stNameMap := map[string]string{"WEB": "Web Araması", "IMAGE": "Görsel Arama", "VIDEO": "Video", "NEWS": "Haberler"}

	if err == nil && searchTypeResp != nil {
		var stTotal float64
		for _, r := range searchTypeResp.Rows {
			stTotal += r.Clicks
		}
		for _, r := range searchTypeResp.Rows {
			if len(r.Keys) == 0 {
				continue
			}
			name := stNameMap[r.Keys[0]]
			if name == "" {
				name = r.Keys[0]
			}
			pct := 0.0
			if stTotal > 0 {
				pct = math.Round(r.Clicks / stTotal * 100)
			}
			searchTypes = append(searchTypes, map[string]interface{}{
				"name":  name,
				"value": r.Clicks,
				"pct":   pct,
				"color": stColorMap[r.Keys[0]],
			})
		}
	}

	// SearchType boşsa varsayılan olarak Web ekle
	if len(searchTypes) == 0 && deviceTotal > 0 {
		searchTypes = append(searchTypes, map[string]interface{}{
			"name": "Web Araması", "value": deviceTotal, "pct": 100.0, "color": "#3B82F6",
		})
	}

	return devices, countries, searchTypes, nil
}

// ═══════════════════════════════════════════════════
// İvme analizi — Yükselen / Düşen sorgular
// ═══════════════════════════════════════════════════

func (g *GSCService) fetchTrendQueries(start, end string, totalDays int) ([]map[string]interface{}, []map[string]interface{}) {
	if totalDays < 14 {
		return nil, nil
	}

	// Son 7 gün vs önceki 7 gün
	endDate, _ := time.Parse("2006-01-02", end)
	recentEnd := endDate.Format("2006-01-02")
	recentStart := endDate.AddDate(0, 0, -6).Format("2006-01-02")
	prevEnd := endDate.AddDate(0, 0, -7).Format("2006-01-02")
	prevStart := endDate.AddDate(0, 0, -13).Format("2006-01-02")

	recentResp, err := g.svc.Searchanalytics.Query(g.siteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: recentStart, EndDate: recentEnd, Dimensions: []string{"query"}, RowLimit: 500,
	}).Do()
	if err != nil {
		return nil, nil
	}

	prevResp, _ := g.svc.Searchanalytics.Query(g.siteURL, &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: prevStart, EndDate: prevEnd, Dimensions: []string{"query"}, RowLimit: 500,
	}).Do()

	prevMap := map[string]float64{}
	if prevResp != nil {
		for _, r := range prevResp.Rows {
			if len(r.Keys) > 0 {
				prevMap[r.Keys[0]] = r.Impressions
			}
		}
	}

	type queryChange struct {
		Query    string
		Current  float64
		Previous float64
		ChangePct float64
	}

	var changes []queryChange
	for _, r := range recentResp.Rows {
		if len(r.Keys) == 0 {
			continue
		}
		q := r.Keys[0]
		prev := prevMap[q]
		changePct := 0.0
		if prev > 0 {
			changePct = ((r.Impressions - prev) / prev) * 100
		} else if r.Impressions > 0 {
			changePct = 100 // yeni sorgu
		}
		changes = append(changes, queryChange{
			Query: q, Current: r.Impressions, Previous: prev, ChangePct: changePct,
		})
	}

	// Yükselen (en çok artan)
	sort.Slice(changes, func(i, j int) bool { return changes[i].ChangePct > changes[j].ChangePct })
	var rising []map[string]interface{}
	for i := 0; i < min(5, len(changes)); i++ {
		if changes[i].ChangePct <= 0 {
			break
		}
		entry := map[string]interface{}{
			"query":       changes[i].Query,
			"impressions": formatNumber(changes[i].Current),
			"change":      fmt.Sprintf("+%%%.0f", changes[i].ChangePct),
		}
		if changes[i].Previous == 0 {
			entry["isNew"] = true
		}
		rising = append(rising, entry)
	}

	// Düşen (en çok azalan)
	sort.Slice(changes, func(i, j int) bool { return changes[i].ChangePct < changes[j].ChangePct })
	var falling []map[string]interface{}
	for i := 0; i < min(5, len(changes)); i++ {
		if changes[i].ChangePct >= 0 {
			break
		}
		// Düşen sorgular için mevcut pozisyon verisi al
		falling = append(falling, map[string]interface{}{
			"query":  changes[i].Query,
			"pos":    0.0, // pozisyon ayrıca çekilebilir
			"change": fmt.Sprintf("%.0f", math.Abs(changes[i].ChangePct)),
			"action": "AI Yaz",
		})
	}

	return rising, falling
}

// ═══════════════════════════════════════════════════
// DB Upsert helpers
// ═══════════════════════════════════════════════════

type SnapshotData struct {
	TotalClicks, TotalImpressions, AvgCTR, AvgPosition float64
	PrevClicks, PrevImpressions, PrevCTR, PrevPosition float64
	BrandClicks, NonBrandClicks, BrandCTR, NonBrandCTR float64
}

func (g *GSCService) upsertSnapshot(period, start, end string, d SnapshotData) {
	var snap models.SeoProjectSnapshot
	result := database.DB.Where("project_id = ? AND period_label = ?", g.project.ID, period).First(&snap)

	snap.ProjectID = g.project.ID
	snap.PeriodLabel = period
	snapDateStr := time.Now().Format("2006-01-02")
	snap.SnapshotDate = &snapDateStr
	snap.StartDate = start
	snap.EndDate = end
	snap.TotalClicks = d.TotalClicks
	snap.TotalImpressions = d.TotalImpressions
	snap.AvgCTR = d.AvgCTR
	snap.AvgPosition = d.AvgPosition
	snap.PrevClicks = d.PrevClicks
	snap.PrevImpressions = d.PrevImpressions
	snap.PrevCTR = d.PrevCTR
	snap.PrevPosition = d.PrevPosition
	snap.BrandClicks = d.BrandClicks
	snap.NonBrandClicks = d.NonBrandClicks
	snap.BrandCTR = d.BrandCTR
	snap.NonBrandCTR = d.NonBrandCTR

	if result.Error != nil {
		// Yeni kayıt oluştur
		if err := database.DB.Create(&snap).Error; err != nil {
			log.Printf("❌ Snapshot CREATE hatası: project_id=%d, period=%s, err=%v", g.project.ID, period, err)
		} else {
			log.Printf("✅ Snapshot oluşturuldu: project_id=%d, period=%s, clicks=%.0f, impressions=%.0f",
				g.project.ID, period, d.TotalClicks, d.TotalImpressions)
		}
	} else {
		// Mevcut kaydı güncelle
		if err := database.DB.Save(&snap).Error; err != nil {
			log.Printf("❌ Snapshot SAVE hatası: project_id=%d, period=%s, err=%v", g.project.ID, period, err)
		} else {
			log.Printf("✅ Snapshot güncellendi: project_id=%d, period=%s, clicks=%.0f, impressions=%.0f",
				g.project.ID, period, d.TotalClicks, d.TotalImpressions)
		}
	}
}

type DiscoveryData struct {
	DeviceBreakdown, CountryBreakdown, SearchTypeBreakdown []map[string]interface{}
	TopQueries, TopPages, CTROpportunities                 []map[string]interface{}
	RisingQueries, FallingQueries                          []map[string]interface{}
}

func (g *GSCService) upsertDiscoveryMetrics(period string, d DiscoveryData) {
	var dm models.SeoDiscoveryMetrics
	result := database.DB.Where("project_id = ? AND period_label = ?", g.project.ID, period).First(&dm)

	dm.ProjectID = g.project.ID
	dm.PeriodLabel = period
	dm.DeviceBreakdown = toJSONSlice(d.DeviceBreakdown)
	dm.CountryBreakdown = toJSONSlice(d.CountryBreakdown)
	dm.SearchTypeBreakdown = toJSONSlice(d.SearchTypeBreakdown)
	dm.TopQueries = toJSONSlice(d.TopQueries)
	dm.TopPages = toJSONSlice(d.TopPages)
	dm.CTROpportunities = toJSONSlice(d.CTROpportunities)
	dm.RisingQueries = toJSONSlice(d.RisingQueries)
	dm.FallingQueries = toJSONSlice(d.FallingQueries)

	if result.Error != nil {
		if err := database.DB.Create(&dm).Error; err != nil {
			log.Printf("❌ DiscoveryMetrics CREATE hatası: project_id=%d, period=%s, err=%v", g.project.ID, period, err)
		} else {
			log.Printf("✅ DiscoveryMetrics oluşturuldu: project_id=%d, period=%s", g.project.ID, period)
		}
	} else {
		if err := database.DB.Save(&dm).Error; err != nil {
			log.Printf("❌ DiscoveryMetrics SAVE hatası: project_id=%d, period=%s, err=%v", g.project.ID, period, err)
		} else {
			log.Printf("✅ DiscoveryMetrics güncellendi: project_id=%d, period=%s", g.project.ID, period)
		}
	}
}

func toJSONSlice(items []map[string]interface{}) models.JSONSlice {
	result := make(models.JSONSlice, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}

// ═══════════════════════════════════════════════════
// AI Summary üretme
// ═══════════════════════════════════════════════════

func (g *GSCService) GenerateAISummary(projectID uint, periodLabel string) error {
	var snapshot models.SeoProjectSnapshot
	if err := database.DB.Where("project_id = ? AND period_label = ?", projectID, periodLabel).First(&snapshot).Error; err != nil {
		return fmt.Errorf("snapshot bulunamadı: %v", err)
	}

	var discovery models.SeoDiscoveryMetrics
	database.DB.Where("project_id = ? AND period_label = ?", projectID, periodLabel).First(&discovery)

	// En çok kaybeden sorgular
	fallingJSON, _ := json.Marshal(discovery.FallingQueries)

	clickChange := 0.0
	if snapshot.PrevClicks > 0 {
		clickChange = ((snapshot.TotalClicks - snapshot.PrevClicks) / snapshot.PrevClicks) * 100
	}

	prompt := fmt.Sprintf(`Sen bir SEO uzmanısın. Aşağıdaki verilere göre 2 cümlelik, agresif ve yönlendirici bir SEO içgörüsü yaz.

Dönem: Son %s
Toplam Tıklama: %.0f (önceki dönem: %.0f, değişim: %%%.1f)
Toplam Gösterim: %.0f
Ort CTR: %%.1f
Ort Pozisyon: %.1f
Brand tık: %.0f | Non-Brand tık: %.0f
Düşen sorgular: %s

Kurallar:
- Türkçe yaz
- Kısa ve öz
- Rakamlarla destekle
- Aksiyon odaklı tavsiye ver`,
		periodLabel,
		snapshot.TotalClicks, snapshot.PrevClicks, clickChange,
		snapshot.TotalImpressions,
		snapshot.AvgCTR*100,
		snapshot.AvgPosition,
		snapshot.BrandClicks, snapshot.NonBrandClicks,
		string(fallingJSON),
	)

	_ = prompt // TODO: LLM API çağrısı eklenecek — şimdilik statik

	// Geçici: basit bir özet üret
	changeDir := "arttı"
	if clickChange < 0 {
		changeDir = "azaldı"
	}
	summary := fmt.Sprintf("Tıklama %%%.0f %s. ", math.Abs(clickChange), changeDir)
	if len(discovery.FallingQueries) > 0 {
		summary += fmt.Sprintf("%d sorgu düşüşte — bu sayfaların içeriklerini güncelleyin.", len(discovery.FallingQueries))
	} else {
		summary += "Tüm metrikler istikrarlı görünüyor."
	}

	snapshot.AISummaryText = summary
	database.DB.Save(&snapshot)

	return nil
}

// ═══════════════════════════════════════════════════
// Site projelerini senkronize et
// ═══════════════════════════════════════════════════

func SyncGSCSites(companyID uint, integration *models.SeoIntegration) ([]models.SeoProject, error) {
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

	client := cfg.Client(context.Background(), token)
	svc, err := searchconsole.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}

	sites, err := svc.Sites.List().Do()
	if err != nil {
		return nil, err
	}

	var projects []models.SeoProject
	for _, site := range sites.SiteEntry {
		// Display name: "sc-domain:example.com" → "example.com"
		displayName := site.SiteUrl
		displayName = strings.TrimPrefix(displayName, "sc-domain:")
		displayName = strings.TrimPrefix(displayName, "https://")
		displayName = strings.TrimPrefix(displayName, "http://")
		displayName = strings.TrimSuffix(displayName, "/")

	var project models.SeoProject
		result := database.DB.Where("company_id = ? AND site_url = ?", companyID, site.SiteUrl).First(&project)

		// DomainURL ve ProjectName otomatik oluştur
		domainURL := strings.TrimPrefix(site.SiteUrl, "sc-domain:")
		if !strings.HasPrefix(domainURL, "http") {
			domainURL = "https://" + domainURL
		}
		domainURL = strings.TrimSuffix(domainURL, "/")

		// ProjectName: domain'den türet (örn: "creatortr.com" → "Creatortr")
		projName := displayName
		projName = strings.TrimSuffix(projName, ".com")
		projName = strings.TrimSuffix(projName, ".net")
		projName = strings.TrimSuffix(projName, ".org")
		projName = strings.TrimSuffix(projName, ".io")
		projName = strings.TrimSuffix(projName, ".co")
		if len(projName) > 0 {
			projName = strings.ToUpper(projName[:1]) + projName[1:]
		}

		if result.Error != nil {
			// Yeni proje
			project = models.SeoProject{
				CompanyID:       companyID,
				SiteURL:         site.SiteUrl,
				DomainURL:       domainURL,
				ProjectName:     projName,
				DisplayName:     displayName,
				PermissionLevel: site.PermissionLevel,
				IsActive:        true,
			}
			database.DB.Create(&project)
		} else {
			project.PermissionLevel = site.PermissionLevel
			project.DisplayName = displayName
			project.DomainURL = domainURL
			project.ProjectName = projName
			database.DB.Save(&project)
		}
		projects = append(projects, project)
	}

	return projects, nil
}

// ═══════════════════════════════════════════════════
// Query + Page çapraz boyut sorgusu (Cannibalization analizi)
// ═══════════════════════════════════════════════════

func (g *GSCService) fetchQueryPageData(start, end string) ([]map[string]interface{}, error) {
	req := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:  start,
		EndDate:    end,
		Dimensions: []string{"query", "page"},
		RowLimit:   5000,
	}

	resp, err := g.svc.Searchanalytics.Query(g.siteURL, req).Do()
	if err != nil {
		return nil, err
	}

	var rows []map[string]interface{}
	for _, row := range resp.Rows {
		if len(row.Keys) < 2 {
			continue
		}
		rows = append(rows, map[string]interface{}{
			"query":       row.Keys[0],
			"page":        row.Keys[1],
			"clicks":      row.Clicks,
			"impressions": row.Impressions,
			"position":    row.Position,
		})
	}
	return rows, nil
}

// ═══════════════════════════════════════════════════
// Yardımcı fonksiyonlar
// ═══════════════════════════════════════════════════

func countryCodeToName(code string) string {
	codeMap := map[string]string{
		"tur": "Türkiye", "deu": "Almanya", "nld": "Hollanda", "aut": "Avusturya",
		"usa": "ABD", "gbr": "İngiltere", "fra": "Fransa", "ita": "İtalya",
		"esp": "İspanya", "bel": "Belçika", "che": "İsviçre", "pol": "Polonya",
		"rou": "Romanya", "bgr": "Bulgaristan", "grc": "Yunanistan",
		"can": "Kanada", "aus": "Avustralya", "bra": "Brezilya", "jpn": "Japonya",
		"ind": "Hindistan", "rus": "Rusya", "chn": "Çin", "kor": "G. Kore",
		"mex": "Meksika", "arg": "Arjantin", "are": "BAE", "sau": "S. Arabistan",
		"swe": "İsveç", "nor": "Norveç", "dnk": "Danimarka", "fin": "Finlandiya",
	}
	if name, ok := codeMap[strings.ToLower(code)]; ok {
		return name
	}
	return strings.ToUpper(code)
}

func formatNumber(n float64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", n/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", n/1000)
	}
	return fmt.Sprintf("%.0f", n)
}

func min(a, b int) int {
	if a < b { return a }
	return b
}
