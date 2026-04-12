package services

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"
)

// ═══════════════════════════════════════════════════
// RANKINGS ENGINE — GSC'den keyword sıralama verisi çekip DB'ye yazar
// ═══════════════════════════════════════════════════

// SyncRankings — Belirli proje için GSC'den keyword+date verisi çekip DB'ye yazar
func SyncRankings(projectID uint) error {
	var project models.SeoProject
	if err := database.DB.First(&project, projectID).Error; err != nil {
		return fmt.Errorf("proje bulunamadı: %v", err)
	}

	var integration models.SeoIntegration
	if err := database.DB.Where("company_id = ? AND provider = ?", project.CompanyID, "google").First(&integration).Error; err != nil {
		return fmt.Errorf("GSC entegrasyonu yok: %v", err)
	}

	log.Printf("📊 Rankings Sync başlıyor: project_id=%d, site=%s", projectID, project.SiteURL)

	// OAuth2 config
	oauthCfg := &oauth2.Config{
		ClientID:     config.Get("GOOGLE_CLIENT_ID"),
		ClientSecret: config.Get("GOOGLE_CLIENT_SECRET"),
		Endpoint:     oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/auth", TokenURL: "https://oauth2.googleapis.com/token"},
	}

	token := &oauth2.Token{
		AccessToken:  integration.AccessToken,
		RefreshToken: integration.RefreshToken,
		Expiry:       integration.TokenExpiry,
		TokenType:    "Bearer",
	}

	tokenSource := oauthCfg.TokenSource(context.Background(), token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("token yenileme hatası: %v", err)
	}

	// Token yenilendiyse kaydet
	if newToken.AccessToken != integration.AccessToken {
		integration.AccessToken = newToken.AccessToken
		integration.TokenExpiry = newToken.Expiry
		if newToken.RefreshToken != "" {
			integration.RefreshToken = newToken.RefreshToken
		}
		database.DB.Save(&integration)
	}

	client := oauth2.NewClient(context.Background(), tokenSource)
	svc, err := searchconsole.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("GSC servis hatası: %v", err)
	}

	// Son 28 günü 7'şerli chunk'lara böl
	end := time.Now().AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -27)

	type dayChunk struct {
		start string
		end   string
	}

	var chunks []dayChunk
	cs := start
	for cs.Before(end) || cs.Equal(end) {
		ce := cs.AddDate(0, 0, 6)
		if ce.After(end) {
			ce = end
		}
		chunks = append(chunks, dayChunk{cs.Format("2006-01-02"), ce.Format("2006-01-02")})
		cs = ce.AddDate(0, 0, 1)
		if cs.After(end) {
			break
		}
	}

	// Goroutine ile paralel GSC sorgusu
	type gscRow struct {
		Query       string
		Date        string
		Position    float64
		Clicks      float64
		Impressions float64
		CTR         float64
	}

	var allRows []gscRow
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, chunk := range chunks {
		wg.Add(1)
		go func(c dayChunk) {
			defer wg.Done()

			req := &searchconsole.SearchAnalyticsQueryRequest{
				StartDate:  c.start,
				EndDate:    c.end,
				Dimensions: []string{"query", "date"},
				RowLimit:   25000,
			}

			resp, err := svc.Searchanalytics.Query(project.SiteURL, req).Do()
			if err != nil {
				log.Printf("  ⚠️ GSC chunk hatası (%s-%s): %v", c.start, c.end, err)
				return
			}

			mu.Lock()
			for _, row := range resp.Rows {
				if len(row.Keys) < 2 {
					continue
				}
				allRows = append(allRows, gscRow{
					Query:       row.Keys[0],
					Date:        row.Keys[1],
					Position:    row.Position,
					Clicks:      row.Clicks,
					Impressions: row.Impressions,
					CTR:         row.Ctr,
				})
			}
			mu.Unlock()
		}(chunk)
	}
	wg.Wait()

	log.Printf("  📥 GSC'den %d satır çekildi", len(allRows))

	// Marka adını hazırla
	brandName := strings.ToLower(project.DisplayName)
	if brandName == "" {
		brandName = strings.TrimPrefix(strings.TrimPrefix(project.SiteURL, "https://"), "http://")
		brandName = strings.TrimSuffix(brandName, "/")
		parts := strings.Split(brandName, ".")
		if len(parts) > 0 {
			brandName = parts[0]
		}
	}

	commercialTerms := []string{"satın al", "fiyat", "ucuz", "al ", "sipariş", "online", "mağaza", "kargo", "indirim", "kampanya"}

	// Mevcut kelimeleri yükle
	keywordCache := map[string]uint{}
	var existingKws []models.SeoTrackedKeyword
	database.DB.Where("project_id = ?", projectID).Find(&existingKws)
	for _, kw := range existingKws {
		keywordCache[kw.Keyword] = kw.ID
	}

	// Yeni kelimeleri kaydet
	newCount := 0
	for _, row := range allRows {
		if _, exists := keywordCache[row.Query]; !exists {
			segment := "Non-Brand"
			if brandName != "" && strings.Contains(strings.ToLower(row.Query), brandName) {
				segment = "Brand"
			}

			intent := "informational"
			queryLower := strings.ToLower(row.Query)
			for _, term := range commercialTerms {
				if strings.Contains(queryLower, term) {
					intent = "commercial"
					break
				}
			}

			kw := models.SeoTrackedKeyword{
				ProjectID: projectID,
				Keyword:   row.Query,
				Segment:   segment,
				Intent:    intent,
			}
			database.DB.Create(&kw)
			keywordCache[row.Query] = kw.ID
			newCount++
		}
	}

	log.Printf("  📝 %d yeni kelime eklendi (toplam: %d)", newCount, len(keywordCache))

	// Günlük istatistikleri upsert (batch)
	for _, row := range allRows {
		kwID, ok := keywordCache[row.Query]
		if !ok {
			continue
		}

		var existing models.SeoKeywordDailyStat
		result := database.DB.Where("keyword_id = ? AND date = ?", kwID, row.Date).First(&existing)
		if result.Error != nil {
			database.DB.Create(&models.SeoKeywordDailyStat{
				ProjectID:   projectID,
				KeywordID:   kwID,
				Date:        row.Date,
				Position:    row.Position,
				Clicks:      row.Clicks,
				Impressions: row.Impressions,
				CTR:         row.CTR,
			})
		} else {
			existing.Position = row.Position
			existing.Clicks = row.Clicks
			existing.Impressions = row.Impressions
			existing.CTR = row.CTR
			database.DB.Save(&existing)
		}
	}

	log.Printf("✅ Rankings Sync tamamlandı: project_id=%d, %d kelime, %d günlük kayıt", projectID, len(keywordCache), len(allRows))
	return nil
}

// ═══════════════════════════════════════════════════
// VERİ OKUMA — Frontend için aggregated keyword listesi
// ═══════════════════════════════════════════════════

// RankedKeyword — Frontend'e dönecek zenginleştirilmiş kelime
type RankedKeyword struct {
	ID              uint      `json:"id"`
	Keyword         string    `json:"keyword"`
	Segment         string    `json:"segment"`
	Intent          string    `json:"intent"`
	CurrentPosition float64   `json:"current_position"`
	Change28D       float64   `json:"change_28d"`
	Impressions     float64   `json:"impressions"`
	Clicks          float64   `json:"clicks"`
	CTR             float64   `json:"ctr"`
	Trend7D         []float64 `json:"trend_7d"`
	TrendDirection  string    `json:"trend_direction"` // up, down, stable
	EstIncrease     string    `json:"estimated_increase,omitempty"`
}

// GetRankedKeywords — Tüm kelimeleri hesaplanmış alanlarla döner
func GetRankedKeywords(projectID uint) ([]RankedKeyword, error) {
	var keywords []models.SeoTrackedKeyword
	database.DB.Where("project_id = ?", projectID).Find(&keywords)

	if len(keywords) == 0 {
		return []RankedKeyword{}, nil
	}

	// Tüm keyword ID'leri
	kwIDs := make([]uint, len(keywords))
	for i, kw := range keywords {
		kwIDs[i] = kw.ID
	}

	// Son 28 günün tüm stat'larını tek sorguda çek
	var allStats []models.SeoKeywordDailyStat
	database.DB.Where("keyword_id IN ?", kwIDs).Order("date ASC").Find(&allStats)

	// keyword_id -> tarih sıralı stat listesi
	statMap := map[uint][]models.SeoKeywordDailyStat{}
	for _, s := range allStats {
		statMap[s.KeywordID] = append(statMap[s.KeywordID], s)
	}

	now := time.Now()
	day28ago := now.AddDate(0, 0, -28).Format("2006-01-02")
	day7ago := now.AddDate(0, 0, -7).Format("2006-01-02")

	result := make([]RankedKeyword, 0, len(keywords))

	for _, kw := range keywords {
		stats := statMap[kw.ID]
		if len(stats) == 0 {
			continue
		}

		// En güncel pozisyon
		latestStat := stats[len(stats)-1]
		currentPos := latestStat.Position

		// 28 gün önceki pozisyon
		var pos28ago float64
		for _, s := range stats {
			if s.Date <= day28ago {
				pos28ago = s.Position
			}
		}
		if pos28ago == 0 {
			pos28ago = stats[0].Position
		}

		change28d := pos28ago - currentPos

		// Toplam impressions/clicks
		totalImps := 0.0
		totalClicks := 0.0
		avgCTR := 0.0
		count := 0.0
		for _, s := range stats {
			totalImps += s.Impressions
			totalClicks += s.Clicks
			avgCTR += s.CTR
			count++
		}
		if count > 0 {
			avgCTR = avgCTR / count
		}

		// Son 7 gün trend
		trend7d := []float64{}
		for _, s := range stats {
			if s.Date >= day7ago {
				trend7d = append(trend7d, math.Round(s.Position*10)/10)
			}
		}

		// Trend yönü
		trendDir := "stable"
		if len(trend7d) >= 2 {
			first := trend7d[0]
			last := trend7d[len(trend7d)-1]
			if last < first-1 {
				trendDir = "up"
			} else if last > first+1 {
				trendDir = "down"
			}
		}

		// İlk sayfaya yakın tahmini
		estIncrease := ""
		if currentPos >= 11 && currentPos <= 20 {
			boost := totalImps * 0.03
			estIncrease = fmt.Sprintf("+%%%.0f tık", boost)
		}

		result = append(result, RankedKeyword{
			ID:              kw.ID,
			Keyword:         kw.Keyword,
			Segment:         kw.Segment,
			Intent:          kw.Intent,
			CurrentPosition: math.Round(currentPos*10) / 10,
			Change28D:       math.Round(change28d*10) / 10,
			Impressions:     totalImps,
			Clicks:          totalClicks,
			CTR:             math.Round(avgCTR*1000) / 10,
			Trend7D:         trend7d,
			TrendDirection:  trendDir,
			EstIncrease:     estIncrease,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CurrentPosition < result[j].CurrentPosition
	})

	return result, nil
}

// ═══════════════════════════════════════════════════
// ÖZET METRİKLER — 4 kartı besleyen summary
// ═══════════════════════════════════════════════════

type RankingSummary struct {
	Top3     int `json:"top_3"`
	Top10    int `json:"top_10"`
	Top20    int `json:"top_20"`
	Beyond20 int `json:"beyond_20"`
	Total    int `json:"total"`
}

func GetRankingSummary(projectID uint) RankingSummary {
	keywords, _ := GetRankedKeywords(projectID)
	summary := RankingSummary{Total: len(keywords)}

	for _, kw := range keywords {
		switch {
		case kw.CurrentPosition <= 3:
			summary.Top3++
		case kw.CurrentPosition <= 10:
			summary.Top10++
		case kw.CurrentPosition <= 20:
			summary.Top20++
		default:
			summary.Beyond20++
		}
	}

	return summary
}
