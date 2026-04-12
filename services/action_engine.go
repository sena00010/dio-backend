package services

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ═══════════════════════════════════════════════════
// ACTION ENGINE — GSC verisini analiz edip görev üretir
// ═══════════════════════════════════════════════════

// RunActionEngine — Belirli bir proje için tüm analiz algoritmalarını çalıştırır
func RunActionEngine(projectID uint) error {
	var project models.SeoProject
	if err := database.DB.First(&project, projectID).Error; err != nil {
		return fmt.Errorf("proje bulunamadı: %v", err)
	}

	var integration models.SeoIntegration
	if err := database.DB.Where("company_id = ? AND provider = ?", project.CompanyID, "google").First(&integration).Error; err != nil {
		return fmt.Errorf("GSC entegrasyonu yok: %v", err)
	}

	log.Printf("⚙️ Action Engine başlıyor: project_id=%d, site=%s", projectID, project.SiteURL)

	// GSC Service oluştur
	gscSvc, err := NewGSCService(&project, &integration)
	if err != nil {
		return fmt.Errorf("GSC servisi hatası: %v", err)
	}

	totalTasks := 0

	// ── Algoritma A: CTR Fırsatları ──
	ctrCount, err := analyzeCTROpportunities(gscSvc, projectID)
	if err != nil {
		log.Printf("  ⚠️  CTR analizi hatası: %v", err)
	} else {
		totalTasks += ctrCount
		log.Printf("  ✅ CTR Fırsatları: %d görev üretildi", ctrCount)
	}

	// ── Algoritma B: İçerik Çakışması ──
	canCount, err := analyzeCannibalization(gscSvc, projectID)
	if err != nil {
		log.Printf("  ⚠️  Çakışma analizi hatası: %v", err)
	} else {
		totalTasks += canCount
		log.Printf("  ✅ İçerik Çakışması: %d görev üretildi", canCount)
	}

	// ── Algoritma C: 404 / Dizin Sorunları ──
	indexCount := analyzeIndexIssues(projectID)
	totalTasks += indexCount

	// ── Mock Data: Büyüme Fırsatları ──
	mockCount := insertMockGrowthTasks(projectID)
	totalTasks += mockCount

	log.Printf("✅ Action Engine tamamlandı: project_id=%d, toplam %d görev", projectID, totalTasks)
	return nil
}

// RunActionEngineForAllProjects — Tüm aktif projeler için motoru çalıştırır (Cron için)
func RunActionEngineForAllProjects() {
	var projects []models.SeoProject
	database.DB.Where("is_active = ?", true).Find(&projects)

	log.Printf("🕐 Cron: Action Engine %d aktif proje için çalışıyor...", len(projects))
	for _, p := range projects {
		if err := RunActionEngine(p.ID); err != nil {
			log.Printf("  ❌ Proje %d hatası: %v", p.ID, err)
		}
	}
	log.Println("🕐 Cron: Action Engine tamamlandı")
}

// ═══════════════════════════════════════════════════
// ALGORİTMA A: CTR FIRSATLARI
// Kural: impressions > 1000 VE ctr < 0.02
// ═══════════════════════════════════════════════════

func analyzeCTROpportunities(gscSvc *GSCService, projectID uint) (int, error) {
	end := time.Now().AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -27)
	startDate := start.Format("2006-01-02")
	endDate := end.Format("2006-01-02")

	topPages, _, err := gscSvc.fetchPageData(startDate, endDate)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, page := range topPages {
		impressions, _ := page["impressions"].(float64)
		ctr, _ := page["ctr"].(float64)
		pageURL, _ := page["page"].(string)
		pos, _ := page["pos"].(float64)

		// Not: fetchPageData'da ctr = math.Round(row.Ctr*1000) / 10 yani %1.2 gibi
		// Gerçek CTR = ctr / 100
		realCTR := ctr / 100.0

		if impressions > 1000 && realCTR < 0.02 {
			targetCTR := 0.03
			potentialGain := impressions * (targetCTR - realCTR)
			rankPage := int(math.Ceil(pos / 10))

			lookupKey := fmt.Sprintf("ctr_%d_%s", projectID, pageURL)

			task := models.SeoActionTask{
				ProjectID:     projectID,
				Category:      "ctr_opportunity",
				TaskType:      "low_ctr",
				Title:         truncateTitle(pageURL) + " sayfası",
				Description:   fmt.Sprintf("%.0fK gösterim alıyor ama CTR sadece %%%.1f. Başlık ve meta açıklamayı optimize ederek tahmini +%.0f tık/ay kazanabilirsiniz.", impressions/1000, ctr, potentialGain),
				Severity:      classifySeverity(potentialGain),
				PotentialGain: math.Round(potentialGain),
				ActionLabel:   "AI Başlık Yaz",
				LookupKey:     lookupKey,
				ActionData: models.ActionData{
					"page_url":    pageURL,
					"impressions": impressions,
					"current_ctr": ctr,
					"target_ctr":  3.0,
					"position":    pos,
					"rank_page":   rankPage,
				},
			}

			upsertActionTask(&task)
			count++
		}
	}

	return count, nil
}

// ═══════════════════════════════════════════════════
// ALGORİTMA B: İÇERİK ÇAKIŞMASI (Cannibalization)
// Kural: Aynı query için 2+ farklı sayfa gösterim alıyor
// ═══════════════════════════════════════════════════

func analyzeCannibalization(gscSvc *GSCService, projectID uint) (int, error) {
	end := time.Now().AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -27)
	startDate := start.Format("2006-01-02")
	endDate := end.Format("2006-01-02")

	// query + page birlikte çek
	rows, err := gscSvc.fetchQueryPageData(startDate, endDate)
	if err != nil {
		return 0, err
	}

	// query → sayfalar map'i
	type pageEntry struct {
		URL         string
		Clicks      float64
		Impressions float64
		Position    float64
	}
	queryPages := map[string][]pageEntry{}

	for _, row := range rows {
		query, _ := row["query"].(string)
		page, _ := row["page"].(string)
		clicks, _ := row["clicks"].(float64)
		impressions, _ := row["impressions"].(float64)
		pos, _ := row["position"].(float64)

		queryPages[query] = append(queryPages[query], pageEntry{
			URL: page, Clicks: clicks, Impressions: impressions, Position: pos,
		})
	}

	count := 0
	for query, pages := range queryPages {
		if len(pages) < 2 {
			continue
		}

		// En iyi (düşük pozisyon = daha iyi) ve en kötü sayfayı bul
		best := pages[0]
		worst := pages[1]
		for _, p := range pages {
			if p.Position < best.Position {
				best = p
			}
			if p.Position > worst.Position {
				worst = p
			}
		}

		lookupKey := fmt.Sprintf("cannibal_%d_%s", projectID, query)

		task := models.SeoActionTask{
			ProjectID:     projectID,
			Category:      "critical",
			TaskType:      "cannibalization",
			Title:         fmt.Sprintf("%d sayfada içerik çakışması", len(pages)),
			Description:   fmt.Sprintf("İki farklı sayfa aynı konuyu anlatıyor. Google hangisini göstereceğini bilemiyor ve her ikisinin sırasını düşürüyor. Birisini ana sayfa yap, diğerini ona yönlendir."),
			Severity:      "critical",
			PotentialGain: 0,
			ActionLabel:   "Nasıl Yapılır?",
			LookupKey:     lookupKey,
			ActionData: models.ActionData{
				"query":        query,
				"page_count":   len(pages),
				"primary_url":  best.URL,
				"primary_pos":  math.Round(best.Position*10) / 10,
				"conflict_url": worst.URL,
				"conflict_pos": math.Round(worst.Position*10) / 10,
			},
		}

		upsertActionTask(&task)
		count++

		// Max 10 çakışma kaydet
		if count >= 10 {
			break
		}
	}

	return count, nil
}

// ═══════════════════════════════════════════════════
// ALGORİTMA C: DİZİN / TEKNİK SORUNLAR
// ═══════════════════════════════════════════════════

func analyzeIndexIssues(projectID uint) int {
	count := 0

	// 404 hatası — varsayılan görev
	lookupKey := fmt.Sprintf("404_%d", projectID)
	task := models.SeoActionTask{
		ProjectID:     projectID,
		Category:      "critical",
		TaskType:      "not_found_404",
		Title:         "4 sayfada \"Sayfa Bulunamadı\" hatası",
		Description:   "Bu 4 sayfa Google'da görünmüyor çünkü ziyaretçi geldiğinde \"Sayfa bulunamadı\" mesajıyla karşılaşıyor. Google da bu sayfaları listeden çıkardı.",
		Severity:      "critical",
		PotentialGain: 340,
		ActionLabel:   "Sayfaları Gör",
		LookupKey:     lookupKey,
		ActionData: models.ActionData{
			"affected_urls": []string{"/urunler/eski-koleksiyon", "/blog/2024-trend"},
			"extra_count":   2,
		},
	}
	upsertActionTask(&task)
	count++

	return count
}

// ═══════════════════════════════════════════════════
// MOCK DATA: Büyüme Fırsatları
// ═══════════════════════════════════════════════════

func insertMockGrowthTasks(projectID uint) int {
	mocks := []models.SeoActionTask{
		{
			ProjectID:     projectID,
			Category:      "growth",
			TaskType:      "competitor_weakness",
			Title:         "Rakip B zayıfladı — sıranı al",
			Description:   "\"Oversize kadın giyim\" kelimesinde rakip B 12. sıradan 17. sıraya düştü. Sen şu an 9. sıradasın. Bu haftaki bir içerik güncellemesiyle ilk 5'e girebilirsin.",
			Severity:      "medium",
			PotentialGain: 480,
			ActionLabel:   "İçerik Güncelle",
			LookupKey:     fmt.Sprintf("growth_competitor_%d", projectID),
			ActionData: models.ActionData{
				"competitor_name":         "Rakip B",
				"keyword":                "Oversize kadın giyim",
				"my_position":            9,
				"competitor_new_position": 17,
				"target_position":        5,
			},
		},
		{
			ProjectID:     projectID,
			Category:      "growth",
			TaskType:      "schema_missing",
			Title:         "48 sayfada \"yıldız puanı\" görseli eksik",
			Description:   "Google'da bazı sonuçların altında yıldız, fiyat veya stok bilgisi görünür. Ürün sayfalarına küçük bir kod ekleyince bu görsel çıkar ve tıklanma oranı ortalama %40 artar.",
			Severity:      "low",
			PotentialGain: 0,
			ActionLabel:   "Hazır Kodu Al",
			LookupKey:     fmt.Sprintf("growth_schema_%d", projectID),
			ActionData: models.ActionData{
				"affected_pages":        48,
				"estimated_ctr_boost":   "+%40",
				"note":                  "Teknik bilgi gerekmez — hazır kodu kopyala yapıştır.",
			},
		},
		{
			ProjectID:     projectID,
			Category:      "growth",
			TaskType:      "backlink_opportunity",
			Title:         "12 blog seni henüz linklemiyor",
			Description:   "Rakip A'yı linkleyen 34 moda blogunun 12'si seni henüz linklememiş. Başka sitelerden gelen linkler Google'da sıralamayı artırır — bu bloglar için hazır e-posta şablonu var.",
			Severity:      "low",
			PotentialGain: 0,
			ActionLabel:   "E-posta Şablonu",
			LookupKey:     fmt.Sprintf("growth_backlink_%d", projectID),
			ActionData: models.ActionData{
				"competitor":    "Rakip A",
				"total_blogs":   34,
				"missing_blogs": 12,
			},
		},
	}

	count := 0
	for i := range mocks {
		upsertActionTask(&mocks[i])
		count++
	}
	return count
}

// ═══════════════════════════════════════════════════
// YARDIMCI HELPERS
// ═══════════════════════════════════════════════════

func upsertActionTask(task *models.SeoActionTask) {
	var existing models.SeoActionTask
	result := database.DB.Where("lookup_key = ?", task.LookupKey).First(&existing)

	if result.Error != nil {
		task.Status = "open"
		database.DB.Create(task)
	} else {
		existing.Title = task.Title
		existing.Description = task.Description
		existing.PotentialGain = task.PotentialGain
		existing.ActionData = task.ActionData
		existing.Severity = task.Severity
		database.DB.Save(&existing)
	}
}

func truncateTitle(url string) string {
	parts := strings.Split(strings.Trim(url, "/"), "/")
	if len(parts) == 0 {
		return url
	}
	last := parts[len(parts)-1]
	last = strings.ReplaceAll(last, "-", " ")
	if len(last) > 40 {
		last = last[:40] + "..."
	}
	if len(last) > 0 {
		last = strings.ToUpper(last[:1]) + last[1:]
	}
	return last
}

func classifySeverity(gain float64) string {
	if gain > 200 {
		return "critical"
	}
	if gain > 100 {
		return "high"
	}
	if gain > 50 {
		return "medium"
	}
	return "low"
}
