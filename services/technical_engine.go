package services

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"google.golang.org/api/searchconsole/v1"
)

// ═══════════════════════════════════════════════════
// TEKNİK SEO MOTOR — PSI + GSC Coverage + Health Score
// ═══════════════════════════════════════════════════

const psiAPIKey = "AIzaSyDXtWjjYzaaEowMnxZ_X7MAk1lwX0pLAJ8"

// ── Dashboard JSON Yapısı ──

type TechnicalDashboard struct {
	HealthSnapshot TechnicalHealthSnapshot `json:"health_snapshot"`
	GSCCoverage    GSCCoverage             `json:"gsc_coverage"`
	SpeedMetrics   SpeedMetrics            `json:"speed_metrics"`
	TechnicalIssues []TechnicalIssue       `json:"technical_issues"`
	SchemaStatus   []SchemaItem            `json:"schema_status"`
	AISummary      string                  `json:"ai_summary"`
}

type TechnicalHealthSnapshot struct {
	HealthScore    int  `json:"health_score"`
	MobileScore    int  `json:"mobile_score"`
	DesktopScore   int  `json:"desktop_score"`
	MobileFriendly bool `json:"mobile_friendly"`
	SchemaMissing  int  `json:"schema_missing"`
}

type GSCCoverage struct {
	IndexedPages    int `json:"indexed_pages"`
	NotIndexedPages int `json:"not_indexed_pages"`
	TotalSubmitted  int `json:"total_submitted"`
}

type SpeedMetrics struct {
	Mobile  []SpeedMetricItem `json:"mobile"`
	Desktop []SpeedMetricItem `json:"desktop"`
}

type SpeedMetricItem struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	RawMs  float64 `json:"raw_ms"`
	Status string `json:"status"` // good, needs-improvement, poor
	Target string `json:"target"`
}

type TechnicalIssue struct {
	Title       string   `json:"title"`
	Severity    string   `json:"severity"` // critical, warning, info
	Description string   `json:"description"`
	Count       int      `json:"count"`
	Impact      string   `json:"impact"`
	Actions     []string `json:"actions"`
}

type SchemaItem struct {
	Label  string `json:"label"`
	Icon   string `json:"icon"`
	Status string `json:"status"` // ok, missing
}

// ══════════════════════════════════════════════════════
// 1. PSI API ENTEGRASYONU
// ══════════════════════════════════════════════════════

type psiResult struct {
	Score   int
	LcpMs   float64
	FidMs   float64 // TBT olarak geliyor
	ClsScore float64
	TtfbMs  float64
	FcpMs   float64
	SiMs    float64
}

func fetchPSI(projectURL, strategy string) (*psiResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	cleanURL := strings.TrimPrefix(projectURL, "sc-domain:")
	if !strings.HasPrefix(cleanURL, "http") {
		cleanURL = "https://" + cleanURL
	}
	cleanURL = strings.TrimSuffix(cleanURL, "/")

	apiURL := fmt.Sprintf(
		"https://www.googleapis.com/pagespeedonline/v5/runPagespeed?url=%s&strategy=%s&key=%s&category=performance",
		cleanURL, strategy, psiAPIKey,
	)

	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PSI API hatası (%s): %v", strategy, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PSI API %d: %s", resp.StatusCode, string(body)[:200])
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("PSI JSON parse: %v", err)
	}

	result := &psiResult{}

	// Lighthouse score
	if lhr, ok := raw["lighthouseResult"].(map[string]interface{}); ok {
		if cats, ok := lhr["categories"].(map[string]interface{}); ok {
			if perf, ok := cats["performance"].(map[string]interface{}); ok {
				if s, ok := perf["score"].(float64); ok {
					result.Score = int(s * 100)
				}
			}
		}
		// Audits → metrikler
		if audits, ok := lhr["audits"].(map[string]interface{}); ok {
			result.LcpMs = extractAuditMs(audits, "largest-contentful-paint")
			result.FidMs = extractAuditMs(audits, "total-blocking-time") // FID yerine TBT
			result.ClsScore = extractAuditScore(audits, "cumulative-layout-shift")
			result.TtfbMs = extractAuditMs(audits, "server-response-time")
			result.FcpMs = extractAuditMs(audits, "first-contentful-paint")
			result.SiMs = extractAuditMs(audits, "speed-index")
		}
	}

	log.Printf("⚡ PSI %s skor=%d LCP=%.0fms FID=%.0fms CLS=%.2f TTFB=%.0fms",
		strategy, result.Score, result.LcpMs, result.FidMs, result.ClsScore, result.TtfbMs)

	return result, nil
}

func extractAuditMs(audits map[string]interface{}, key string) float64 {
	if audit, ok := audits[key].(map[string]interface{}); ok {
		if v, ok := audit["numericValue"].(float64); ok {
			return v
		}
	}
	return 0
}

func extractAuditScore(audits map[string]interface{}, key string) float64 {
	if audit, ok := audits[key].(map[string]interface{}); ok {
		if v, ok := audit["numericValue"].(float64); ok {
			return v
		}
	}
	return 0
}

// ══════════════════════════════════════════════════════
// 2. GSC COVERAGE — Search Analytics + Sitemap
// ══════════════════════════════════════════════════════

// getIndexedPageCount — GSC'den son 28 günde impression alan unique sayfa sayısı
func getIndexedPageCount(svc *searchconsole.Service, siteURL string) (int, error) {
	now := time.Now()
	startDate := now.AddDate(0, 0, -28).Format("2006-01-02")
	endDate := now.AddDate(0, 0, -2).Format("2006-01-02")

	req := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:  startDate,
		EndDate:    endDate,
		Dimensions: []string{"page"},
		RowLimit:   25000,
	}

	resp, err := svc.Searchanalytics.Query(siteURL, req).Do()
	if err != nil {
		return 0, fmt.Errorf("GSC page count hatası: %v", err)
	}

	return len(resp.Rows), nil
}

// getSitemapPageCount — Sitemap.xml'den <loc> tag sayısı
func getSitemapPageCount(siteURL string) int {
	cleanURL := strings.TrimPrefix(siteURL, "sc-domain:")
	if !strings.HasPrefix(cleanURL, "http") {
		cleanURL = "https://" + cleanURL
	}
	cleanURL = strings.TrimSuffix(cleanURL, "/")

	sitemapURL := cleanURL + "/sitemap.xml"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", sitemapURL, nil)
	req.Header.Set("User-Agent", "DIO-SEO-Bot/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("⚠️ Sitemap fetch hatası: %v", err)
		return 0
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("⚠️ Sitemap %d döndü: %s", resp.StatusCode, sitemapURL)
		return 0
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0
	}

	// XML'de <loc> tag'larını say
	// Önce sitemap index mi diye kontrol et
	type SitemapIndex struct {
		Sitemaps []struct {
			Loc string `xml:"loc"`
		} `xml:"sitemap"`
	}
	type URLSet struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}

	// Sitemap index mi?
	var sIdx SitemapIndex
	if err := xml.Unmarshal(body, &sIdx); err == nil && len(sIdx.Sitemaps) > 0 {
		// Sitemap index — alt sitemapları topla
		total := 0
		for _, sm := range sIdx.Sitemaps {
			total += fetchChildSitmapCount(sm.Loc)
		}
		log.Printf("📍 Sitemap index: %d alt sitemap, toplam %d URL", len(sIdx.Sitemaps), total)
		return total
	}

	// Normal URLSet
	var urlSet URLSet
	if err := xml.Unmarshal(body, &urlSet); err == nil {
		log.Printf("📍 Sitemap: %d URL bulundu", len(urlSet.URLs))
		return len(urlSet.URLs)
	}

	// Fallback: <loc> stringini say
	count := strings.Count(string(body), "<loc>")
	log.Printf("📍 Sitemap fallback <loc> count: %d", count)
	return count
}

func fetchChildSitmapCount(url string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	return strings.Count(string(body), "<loc>")
}

// ══════════════════════════════════════════════════════
// 3. SAĞLIK SKORU ALGORİTMASI
// ══════════════════════════════════════════════════════

func calculateHealthScore(mobileScore int, indexedPages, totalSubmitted int, criticalIssueCount int) int {
	// PSI Mobil: %50
	psiComponent := float64(mobileScore) * 0.50

	// GSC Dizin Kapsamı: %30
	var indexComponent float64
	if totalSubmitted > 0 {
		ratio := float64(indexedPages) / float64(totalSubmitted)
		if ratio > 1 {
			ratio = 1
		}
		indexComponent = ratio * 100.0 * 0.30
	} else {
		indexComponent = 50 * 0.30 // veri yoksa 50 varsay
	}

	// Kritik hata cezası: her hata -5
	penaltyComponent := float64(criticalIssueCount) * 5.0

	score := int(math.Round(psiComponent + indexComponent + 20.0 - penaltyComponent))
	// 20.0 = base offset (schema, diğer faktörler)

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

// ══════════════════════════════════════════════════════
// 4. PSI SYNC (DB'ye kaydet) — Arka planda çalışır
// ══════════════════════════════════════════════════════

func SyncTechnicalHealth(projectID uint) error {
	var project models.SeoProject
	if err := database.DB.First(&project, projectID).Error; err != nil {
		return fmt.Errorf("proje bulunamadı: %v", err)
	}

	log.Printf("⚙️ Teknik SEO sync başlıyor: project=%d (%s)", projectID, project.SiteURL)

	var wg sync.WaitGroup
	var mobilePSI, desktopPSI *psiResult
	var mobileErr, desktopErr error
	var indexedPages, sitemapCount int
	var indexErr error

	// 1) PSI Mobile — goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		mobilePSI, mobileErr = fetchPSI(project.SiteURL, "mobile")
	}()

	// 2) PSI Desktop — goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		desktopPSI, desktopErr = fetchPSI(project.SiteURL, "desktop")
	}()

	// 3) Sitemap sayısı — goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		sitemapCount = getSitemapPageCount(project.SiteURL)
	}()

	// 4) GSC indexed pages — goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		var integration models.SeoIntegration
		if err := database.DB.Where("company_id = ? AND provider = ?", project.CompanyID, "google").First(&integration).Error; err != nil {
			indexErr = err
			return
		}
		gscSvc, err := NewGSCService(&project, &integration)
		if err != nil {
			indexErr = err
			return
		}
		indexedPages, indexErr = getIndexedPageCount(gscSvc.svc, project.SiteURL)
	}()

	wg.Wait()

	// Hata logla ama devam et
	if mobileErr != nil {
		log.Printf("⚠️ PSI Mobile hatası: %v", mobileErr)
	}
	if desktopErr != nil {
		log.Printf("⚠️ PSI Desktop hatası: %v", desktopErr)
	}
	if indexErr != nil {
		log.Printf("⚠️ GSC Index hatası: %v", indexErr)
	}

	// Kritik görev sayısı
	var criticalCount int64
	database.DB.Model(&models.SeoActionTask{}).Where("project_id = ? AND severity = ? AND status = ?", projectID, "critical", "open").Count(&criticalCount)

	// Schema eksik kelime sayısı
	var schemaMissing int64
	database.DB.Model(&models.SeoTrackedKeyword{}).Where("project_id = ? AND last_position > ?", projectID, 5).Count(&schemaMissing)

	// Not-indexed hesapla
	notIndexed := sitemapCount - indexedPages
	if notIndexed < 0 {
		notIndexed = 0
	}

	// Sağlık skoru
	mScore := 0
	if mobilePSI != nil {
		mScore = mobilePSI.Score
	}
	healthScore := calculateHealthScore(mScore, indexedPages, sitemapCount, int(criticalCount))

	// DB Upsert
	var existing models.SeoTechnicalHealth
	tx := database.DB.Where("project_id = ?", projectID).First(&existing)

	record := models.SeoTechnicalHealth{
		ProjectID:          projectID,
		OverallHealthScore: healthScore,
		IndexedPages:       indexedPages,
		NotIndexedPages:    notIndexed,
		TotalSubmitted:     sitemapCount,
		SchemaMissing:      int(schemaMissing),
		MobileFriendly:     true, // PSI mobile-friendly detaylı check ayrı API ister
	}

	if mobilePSI != nil {
		record.MobileScore = mobilePSI.Score
		record.LcpMs = mobilePSI.LcpMs
		record.FidMs = mobilePSI.FidMs
		record.ClsScore = mobilePSI.ClsScore
		record.TtfbMs = mobilePSI.TtfbMs
		record.FcpMs = mobilePSI.FcpMs
		record.SiMs = mobilePSI.SiMs
	}

	if desktopPSI != nil {
		record.DesktopScore = desktopPSI.Score
		record.DesktopLcp = desktopPSI.LcpMs
		record.DesktopFid = desktopPSI.FidMs
		record.DesktopCls = desktopPSI.ClsScore
		record.DesktopTtfb = desktopPSI.TtfbMs
	}

	if tx.Error == nil {
		record.ID = existing.ID
		database.DB.Save(&record)
	} else {
		database.DB.Create(&record)
	}

	log.Printf("✅ Teknik SEO sync tamamlandı: project=%d health=%d mobile=%d desktop=%d indexed=%d sitemap=%d",
		projectID, healthScore, record.MobileScore, record.DesktopScore, indexedPages, sitemapCount)

	return nil
}

// ══════════════════════════════════════════════════════
// 5. DASHBOARD ENDPOINT — JSON döner
// ══════════════════════════════════════════════════════

func GetTechnicalDashboard(projectID uint) (*TechnicalDashboard, error) {
	var health models.SeoTechnicalHealth
	hasData := database.DB.Where("project_id = ?", projectID).First(&health).Error == nil

	if !hasData {
		// Henüz veri yoksa arka planda sync başlat, boş döndür
		go func() {
			if err := SyncTechnicalHealth(projectID); err != nil {
				log.Printf("⚠️ Background technical sync hatası: %v", err)
			}
		}()
		return &TechnicalDashboard{
			AISummary: "Teknik analiz başlatıldı. Sayfayı birkaç saniye sonra yenileyin.",
		}, nil
	}

	// ── Health Snapshot ──
	snapshot := TechnicalHealthSnapshot{
		HealthScore:    health.OverallHealthScore,
		MobileScore:    health.MobileScore,
		DesktopScore:   health.DesktopScore,
		MobileFriendly: health.MobileFriendly,
		SchemaMissing:  health.SchemaMissing,
	}

	// ── GSC Coverage ──
	coverage := GSCCoverage{
		IndexedPages:    health.IndexedPages,
		NotIndexedPages: health.NotIndexedPages,
		TotalSubmitted:  health.TotalSubmitted,
	}

	// ── Speed Metrics ──
	mobileMetrics := []SpeedMetricItem{
		makeSpeedItem("LCP (Largest Contentful Paint)", health.LcpMs, 2500, 4000, "< 2.5s"),
		makeSpeedItem("TBT (Total Blocking Time)", health.FidMs, 200, 600, "< 200ms"),
		makeSpeedItem("CLS (Cumulative Layout Shift)", health.ClsScore*1000, 100, 250, "< 0.1"),
		makeSpeedItem("TTFB (Time to First Byte)", health.TtfbMs, 800, 1800, "< 0.8s"),
		makeSpeedItem("FCP (First Contentful Paint)", health.FcpMs, 1800, 3000, "< 1.8s"),
		makeSpeedItem("SI (Speed Index)", health.SiMs, 3400, 5800, "< 3.4s"),
	}

	// CLS özel format
	mobileMetrics[2].Value = fmt.Sprintf("%.2f", health.ClsScore)

	desktopMetrics := []SpeedMetricItem{
		makeSpeedItem("LCP (Largest Contentful Paint)", health.DesktopLcp, 2500, 4000, "< 2.5s"),
		makeSpeedItem("TBT (Total Blocking Time)", health.DesktopFid, 200, 600, "< 200ms"),
		makeSpeedItem("CLS (Cumulative Layout Shift)", health.DesktopCls*1000, 100, 250, "< 0.1"),
		makeSpeedItem("TTFB (Time to First Byte)", health.DesktopTtfb, 800, 1800, "< 0.8s"),
	}
	desktopMetrics[2].Value = fmt.Sprintf("%.2f", health.DesktopCls)

	speed := SpeedMetrics{
		Mobile:  mobileMetrics,
		Desktop: desktopMetrics,
	}

	// ── Technical Issues (action_tasks'tan) ──
	var tasks []models.SeoActionTask
	database.DB.Where("project_id = ? AND status = ?", projectID, "open").
		Order("FIELD(severity, 'critical', 'high', 'medium', 'low')").
		Find(&tasks)

	issues := buildTechnicalIssues(tasks, health)

	// ── Schema Status ──
	schemas := []SchemaItem{
		{Label: "Product", Icon: "🏷️", Status: schemaStatus(health.SchemaMissing > 20)},
		{Label: "Article", Icon: "📝", Status: schemaStatus(health.SchemaMissing > 10)},
		{Label: "FAQ", Icon: "❓", Status: "missing"},
		{Label: "BreadcrumbList", Icon: "🔗", Status: schemaStatus(health.SchemaMissing > 30)},
	}

	// ── AI Summary ──
	aiSummary := buildTechnicalAISummary(health, len(issues))

	return &TechnicalDashboard{
		HealthSnapshot:  snapshot,
		GSCCoverage:     coverage,
		SpeedMetrics:    speed,
		TechnicalIssues: issues,
		SchemaStatus:    schemas,
		AISummary:       aiSummary,
	}, nil
}

// ══════════════════════════════════════════════════════
// HELPERS
// ══════════════════════════════════════════════════════

func makeSpeedItem(label string, valueMs float64, goodThreshold, poorThreshold float64, target string) SpeedMetricItem {
	status := "good"
	if valueMs > poorThreshold {
		status = "poor"
	} else if valueMs > goodThreshold {
		status = "needs-improvement"
	}

	// Format value
	var valueStr string
	if valueMs >= 1000 {
		valueStr = fmt.Sprintf("%.1fs", valueMs/1000)
	} else {
		valueStr = fmt.Sprintf("%.0fms", valueMs)
	}

	return SpeedMetricItem{
		Label:  label,
		Value:  valueStr,
		RawMs:  valueMs,
		Status: status,
		Target: target,
	}
}

func schemaStatus(missing bool) string {
	if missing {
		return "missing"
	}
	return "ok"
}

func buildTechnicalIssues(tasks []models.SeoActionTask, health models.SeoTechnicalHealth) []TechnicalIssue {
	issues := []TechnicalIssue{}

	// Kayıp sayfalar (404)
	var count404 int
	for _, t := range tasks {
		if strings.Contains(t.TaskType, "404") || strings.Contains(t.Title, "404") {
			count404++
		}
	}
	if count404 > 0 || health.NotIndexedPages > 0 {
		missing := count404
		if missing == 0 {
			missing = health.NotIndexedPages
		}
		issues = append(issues, TechnicalIssue{
			Title:       "Kayıp Sayfalar (404 Hatası)",
			Severity:    "critical",
			Description: "Ziyaretçi bu sayfalara geldiğinde 'Sayfa bulunamadı' mesajı görüyor. Google bu sayfaları sıralamadan çıkardı.",
			Count:       missing,
			Impact:      fmt.Sprintf("Her ay ~%d tıklamayı kaybediyorsun.", missing*85),
			Actions:     []string{"Hangi Sayfalar?", "Nasıl Düzeltirim?"},
		})
	}

	// Çakışan içerik (cannibalization)
	var cannibCount int
	for _, t := range tasks {
		if strings.Contains(t.TaskType, "cannibalization") || strings.Contains(t.TaskType, "duplicate") {
			cannibCount++
		}
	}
	if cannibCount > 0 {
		issues = append(issues, TechnicalIssue{
			Title:       "Çakışan (Duplicate) İçerik",
			Severity:    "critical",
			Description: "Birden fazla URL aynı içeriği sunuyor. Google hangi sayfayı sıralayacağını karıştırıyor ve SEO gücünüz bölünüyor.",
			Count:       cannibCount,
			Impact:      "Canonical tag ekleyerek düzeltebilirsin.",
			Actions:     []string{"Çakışan Sayfaları Gör", "Canonical Rehberi"},
		})
	}

	// Dizine alınmayan sayfalar
	if health.NotIndexedPages > 0 {
		issues = append(issues, TechnicalIssue{
			Title:       "Dizine Alınmayan Sayfalar",
			Severity:    "warning",
			Description: "Bu sayfalar Google tarafından tarandı ama dizine alınmadı. İçerik kalitesi veya teknik sorunlar olabilir.",
			Count:       health.NotIndexedPages,
			Impact:      "Sitemap'te olup dizinde olmayan sayfalar.",
			Actions:     []string{"Sayfaları İncele", "GSC'de Kontrol Et"},
		})
	}

	// Schema eksik
	if health.SchemaMissing > 5 {
		issues = append(issues, TechnicalIssue{
			Title:       "Schema Markup Eksik",
			Severity:    "warning",
			Description: "Yapısal veri (schema) eksik olan sayfalar arama sonuçlarında yıldız, fiyat gibi zengin sonuçları gösteremiyor.",
			Count:       health.SchemaMissing,
			Impact:      "Schema eklemek CTR'yi %20-30 artırabilir.",
			Actions:     []string{"Eksik Sayfalar", "Schema Rehberi"},
		})
	}

	// Yavaş LCP
	if health.LcpMs > 2500 {
		issues = append(issues, TechnicalIssue{
			Title:       "Yavaş Sayfa Yüklenme (LCP)",
			Severity:    "warning",
			Description: fmt.Sprintf("LCP süresi %.1fs — Google standardı 2.5s'nin üstünde. Bu sıralama düşüşüne neden oluyor.", health.LcpMs/1000),
			Count:       1,
			Impact:      "Resim optimizasyonu ve lazy loading ile düzel.",
			Actions:     []string{"Hız Raporu", "Optimizasyon Rehberi"},
		})
	}

	// Düşük mobil skor
	if health.MobileScore > 0 && health.MobileScore < 50 {
		issues = append(issues, TechnicalIssue{
			Title:       "Düşük Mobil Performans",
			Severity:    "critical",
			Description: fmt.Sprintf("Mobil performans skoru %d/100 — Google bunu ciddi bir sıralama faktörü olarak kullanıyor.", health.MobileScore),
			Count:       1,
			Impact:      "Mobil performansı artırmak sıralamaları iyileştirir.",
			Actions:     []string{"Detaylı Rapor"},
		})
	}

	return issues
}

func buildTechnicalAISummary(health models.SeoTechnicalHealth, issueCount int) string {
	status := "iyi"
	if health.OverallHealthScore < 60 {
		status = "kritik"
	} else if health.OverallHealthScore < 80 {
		status = "orta"
	}

	summary := fmt.Sprintf("Teknik durum: %s (%d/100).", status, health.OverallHealthScore)

	if health.MobileScore > 0 {
		summary += fmt.Sprintf(" Site hızı mobil %d, masaüstü %d puan.", health.MobileScore, health.DesktopScore)
	}

	if health.IndexedPages > 0 {
		summary += fmt.Sprintf(" Google'da %d sayfa indekslenmiş, %d sayfa henüz dizine alınmamış.",
			health.IndexedPages, health.NotIndexedPages)
	}

	if issueCount > 0 {
		summary += fmt.Sprintf(" Toplam %d teknik sorun düzeltme bekliyor.", issueCount)
	} else {
		summary += " Kritik teknik sorun bulunmuyor."
	}

	return summary
}
