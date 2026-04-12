package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"gorm.io/gorm/clause"
)

// ═══════════════════════════════════════════════════
// BACKLINK ENGINE — DataForSEO API Entegrasyonu
// ═══════════════════════════════════════════════════

// ── DataForSEO API Response Yapıları ──

type dfsResponse struct {
	Tasks []dfsTask `json:"tasks"`
}

type dfsTask struct {
	Result []json.RawMessage `json:"result"`
}

// Summary response
type dfsSummaryResult struct {
	Backlinks        int     `json:"backlinks"`
	ReferringDomains int     `json:"referring_domains"`
	Rank             float64 `json:"rank"`
	BrokenBacklinks  int     `json:"broken_backlinks"`
	ReferringDomainsNofollow int `json:"referring_domains_nofollow"`
	BacklinksNofollow int    `json:"backlinks_nofollow"`
	NewBacklinksLast7Days  int `json:"new_backlinks_last7days"`
	LostBacklinksLast7Days int `json:"lost_backlinks_last7days"`
}

// Items response
type dfsBacklinksResult struct {
	Items []dfsBacklinkItem `json:"items"`
}

type dfsBacklinkItem struct {
	DomainFrom  string  `json:"domain_from"`
	URLFrom     string  `json:"url_from"`
	URLTo       string  `json:"url_to"`
	Rank        float64 `json:"rank"`
	SpamScore   float64 `json:"page_from_external_links"` // DataForSEO spam score alanı
	IsDofollow  bool    `json:"dofollow"`
	Anchor      string  `json:"anchor"`
	LinkType    string  `json:"type"`
	FirstSeen   string  `json:"first_seen"`
}

// ── Dashboard JSON Formatı ──

type BacklinkDashboard struct {
	Summary         BacklinkSummaryDTO    `json:"summary"`
	TopBacklinks    []BacklinkItemDTO     `json:"top_backlinks"`
	RecentBacklinks []BacklinkItemDTO     `json:"recent_backlinks"`
	SpamLinks       []BacklinkItemDTO     `json:"spam_links"`
	Opportunities   []BacklinkOpportunity `json:"opportunities"`
	AISummary       string                `json:"ai_summary"`
	Cached          bool                  `json:"cached"`
}

type BacklinkSummaryDTO struct {
	TotalBacklinks   int     `json:"total_backlinks"`
	ReferringDomains int     `json:"referring_domains"`
	DomainRating     int     `json:"domain_rating"`
	SpamLinks        int     `json:"spam_links"`
	DoFollowCount    int     `json:"dofollow_count"`
	NoFollowCount    int     `json:"nofollow_count"`
	NewBacklinks7d   int     `json:"new_backlinks_7d"`
	LostBacklinks7d  int     `json:"lost_backlinks_7d"`
	BrokenLinks      int     `json:"broken_links"`
	RankScore        float64 `json:"rank_score"`
}

type BacklinkItemDTO struct {
	SourceDomain string  `json:"source_domain"`
	SourceURL    string  `json:"source_url"`
	AnchorText   string  `json:"anchor_text"`
	Rank         float64 `json:"rank"`
	SpamScore    float64 `json:"spam_score"`
	IsDofollow   bool    `json:"is_dofollow"`
	LinkType     string  `json:"link_type"`
	FirstSeenAt  string  `json:"first_seen_at"`
	DomainRating int     `json:"domain_rating"`
}

type BacklinkOpportunity struct {
	SiteName   string `json:"site_name"`
	SiteURL    string `json:"site_url"`
	DR         int    `json:"dr"`
	CommonTopic string `json:"common_topic"`
	Action     string `json:"action"`
}

// ═══════════════════════════════════════════════════
// SYNC — DataForSEO'dan veri çek ve DB'ye yaz
// ═══════════════════════════════════════════════════

func SyncBacklinkProject(projectID uint) (bool, error) {
	// 1) Kota Koruması — son 7 gün içinde güncellenmişse skip
	var existing models.SeoBacklinkSummary
	if err := database.DB.Where("project_id = ?", projectID).First(&existing).Error; err == nil {
		if time.Since(existing.UpdatedAt) < 7*24*time.Hour {
			log.Printf("📌 Backlink verisi güncel (son 7 gün), cache kullanılıyor: project=%d", projectID)
			return true, nil // cached
		}
	}

	// 2) Proje URL'ini al
	var project models.SeoProject
	if err := database.DB.First(&project, projectID).Error; err != nil {
		return false, fmt.Errorf("proje bulunamadı: %v", err)
	}

	domain := cleanDomain(project.SiteURL)
	log.Printf("🔗 Backlink sync başlıyor: project=%d, domain=%s", projectID, domain)

	// 3) API çağrıları
	login := config.Get("DATAFORSEO_LOGIN")
	password := config.Get("DATAFORSEO_PASSWORD")
	if login == "" || password == "" {
		return false, fmt.Errorf("DataForSEO credentials eksik (.env)")
	}

	// API 1: Summary
	summary, err := fetchDFSSummary(domain, login, password)
	if err != nil {
		return false, fmt.Errorf("summary API hatası: %v", err)
	}

	// API 2: Items (en güçlü 100)
	items, err := fetchDFSBacklinks(domain, login, password)
	if err != nil {
		log.Printf("⚠️ Backlink items hatası (devam ediliyor): %v", err)
		items = []dfsBacklinkItem{}
	}

	// 4) Spam ve DoFollow hesapla
	spamCount := 0
	dofollowCount := 0
	nofollowCount := 0
	for _, item := range items {
		if item.SpamScore > 70 {
			spamCount++
		}
		if item.IsDofollow {
			dofollowCount++
		} else {
			nofollowCount++
		}
	}

	// DR hesapla (DataForSEO rank 0-1000 → DR 0-100)
	dr := int(summary.Rank / 10)
	if dr > 100 {
		dr = 100
	}

	// 5) Summary Upsert (Race condition önlemek için OnConflict kullanılır)
	summaryModel := models.SeoBacklinkSummary{
		ProjectID:        projectID,
		Domain:           domain,
		TotalBacklinks:   summary.Backlinks,
		ReferringDomains: summary.ReferringDomains,
		RankScore:        summary.Rank,
		DomainRating:     dr,
		DoFollowCount:    dofollowCount,
		NoFollowCount:    nofollowCount,
		SpamLinks:        spamCount,
		BrokenLinks:      summary.BrokenBacklinks,
		NewBacklinks7d:   summary.NewBacklinksLast7Days,
		LostBacklinks7d:  summary.LostBacklinksLast7Days,
	}

	if err := database.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "project_id"}},
		UpdateAll: true,
	}).Create(&summaryModel).Error; err != nil {
		log.Printf("❌ Summary upsert hatası (project=%d): %v", projectID, err)
	}

	log.Printf("✅ Summary kaydedildi: backlinks=%d, domains=%d, DR=%d, spam=%d",
		summary.Backlinks, summary.ReferringDomains, dr, spamCount)

	// 6) Items — Truncate & Insert
	database.DB.Where("project_id = ?", projectID).Delete(&models.SeoBacklinkItem{})

	if len(items) > 0 {
		batchItems := make([]models.SeoBacklinkItem, 0, len(items))
		for _, item := range items {
			var firstSeen *time.Time
			if item.FirstSeen != "" {
				if t, err := time.Parse("2006-01-02 15:04:05 +00:00", item.FirstSeen); err == nil {
					firstSeen = &t
				}
			}

			batchItems = append(batchItems, models.SeoBacklinkItem{
				ProjectID:    projectID,
				TargetDomain: domain,
				SourceDomain: item.DomainFrom,
				SourceURL:    item.URLFrom,
				TargetURL:    item.URLTo,
				AnchorText:   item.Anchor,
				Rank:         item.Rank,
				SpamScore:    item.SpamScore,
				IsDofollow:   item.IsDofollow,
				LinkType:     item.LinkType,
				FirstSeenAt:  firstSeen,
			})
		}

		if err := database.DB.CreateInBatches(batchItems, 50).Error; err != nil {
			log.Printf("⚠️ Backlink items batch insert hatası: %v", err)
		} else {
			log.Printf("✅ %d backlink item kaydedildi", len(batchItems))
		}
	}

	return false, nil
}

// ═══════════════════════════════════════════════════
// DASHBOARD — JSON hazırla
// ═══════════════════════════════════════════════════

func GetBacklinkDashboard(projectID uint) (*BacklinkDashboard, error) {
	var summary models.SeoBacklinkSummary
	hasSummary := database.DB.Where("project_id = ?", projectID).First(&summary).Error == nil

	if !hasSummary {
		// İlk kez — arka planda sync başlat
		go func() {
			_, _ = SyncBacklinkProject(projectID)
		}()
		return &BacklinkDashboard{
			AISummary: "Backlink analizi başlatıldı. Sayfayı birkaç saniye sonra yenileyin.",
		}, nil
	}

	// Items
	var items []models.SeoBacklinkItem
	database.DB.Where("project_id = ?", projectID).Order("`rank` DESC").Limit(100).Find(&items)

	// DTO'ları oluştur
	summaryDTO := BacklinkSummaryDTO{
		TotalBacklinks:   summary.TotalBacklinks,
		ReferringDomains: summary.ReferringDomains,
		DomainRating:     summary.DomainRating,
		SpamLinks:        summary.SpamLinks,
		DoFollowCount:    summary.DoFollowCount,
		NoFollowCount:    summary.NoFollowCount,
		NewBacklinks7d:   summary.NewBacklinks7d,
		LostBacklinks7d:  summary.LostBacklinks7d,
		BrokenLinks:      summary.BrokenLinks,
		RankScore:        summary.RankScore,
	}

	// Top backlinks (rank'e göre)
	topLinks := make([]BacklinkItemDTO, 0)
	recentLinks := make([]BacklinkItemDTO, 0)
	spamLinks := make([]BacklinkItemDTO, 0)

	for _, item := range items {
		dto := BacklinkItemDTO{
			SourceDomain: item.SourceDomain,
			SourceURL:    item.SourceURL,
			AnchorText:   item.AnchorText,
			Rank:         item.Rank,
			SpamScore:    item.SpamScore,
			IsDofollow:   item.IsDofollow,
			LinkType:     item.LinkType,
			DomainRating: int(item.Rank / 10),
		}
		if item.FirstSeenAt != nil {
			dto.FirstSeenAt = item.FirstSeenAt.Format("2 Oca 2006")
		}

		topLinks = append(topLinks, dto)

		if item.SpamScore > 70 {
			spamLinks = append(spamLinks, dto)
		}

		if item.FirstSeenAt != nil && time.Since(*item.FirstSeenAt) < 30*24*time.Hour {
			recentLinks = append(recentLinks, dto)
		}
	}

	// Max 20
	if len(topLinks) > 20 {
		topLinks = topLinks[:20]
	}
	if len(recentLinks) > 10 {
		recentLinks = recentLinks[:10]
	}

	// Opportunities (basit link fırsatları — rakip analizi için)
	opportunities := buildOpportunities(items)

	// AI Summary
	aiSummary := buildBacklinkAISummary(summary)

	return &BacklinkDashboard{
		Summary:         summaryDTO,
		TopBacklinks:    topLinks,
		RecentBacklinks: recentLinks,
		SpamLinks:       spamLinks,
		Opportunities:   opportunities,
		AISummary:       aiSummary,
		Cached:          time.Since(summary.UpdatedAt) < 7*24*time.Hour,
	}, nil
}

// ═══════════════════════════════════════════════════
// DATAFORSEO API ÇAĞRILARI
// ═══════════════════════════════════════════════════

func fetchDFSSummary(domain, login, password string) (*dfsSummaryResult, error) {
	payload := []map[string]interface{}{
		{
			"target": domain,
			"internal_list_limit": 0,
		},
	}

	body, err := callDFS("https://api.dataforseo.com/v3/backlinks/summary/live", payload, login, password)
	if err != nil {
		return nil, err
	}

	var resp dfsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("JSON parse: %v", err)
	}

	if len(resp.Tasks) == 0 || len(resp.Tasks[0].Result) == 0 {
		return nil, fmt.Errorf("boş API yanıtı")
	}

	var result dfsSummaryResult
	if err := json.Unmarshal(resp.Tasks[0].Result[0], &result); err != nil {
		return nil, fmt.Errorf("result parse: %v", err)
	}

	return &result, nil
}

func fetchDFSBacklinks(domain, login, password string) ([]dfsBacklinkItem, error) {
	payload := []map[string]interface{}{
		{
			"target":   domain,
			"limit":    100,
			"order_by": []string{"rank,desc"},
			"mode":     "as_is",
		},
	}

	body, err := callDFS("https://api.dataforseo.com/v3/backlinks/backlinks/live", payload, login, password)
	if err != nil {
		return nil, err
	}

	var resp dfsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("JSON parse: %v", err)
	}

	if len(resp.Tasks) == 0 || len(resp.Tasks[0].Result) == 0 {
		return nil, fmt.Errorf("boş API yanıtı")
	}

	var result dfsBacklinksResult
	if err := json.Unmarshal(resp.Tasks[0].Result[0], &result); err != nil {
		return nil, fmt.Errorf("result parse: %v", err)
	}

	return result.Items, nil
}

func callDFS(url string, payload interface{}, login, password string) ([]byte, error) {
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(login, password)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP hatası: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("body okuma hatası: %v", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API %d: %s", resp.StatusCode, string(body[:200]))
	}

	return body, nil
}

// ═══════════════════════════════════════════════════
// YARDIMCILAR
// ═══════════════════════════════════════════════════

func cleanDomain(siteURL string) string {
	d := strings.TrimPrefix(siteURL, "sc-domain:")
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	d = strings.TrimSuffix(d, "/")
	return d
}

func buildOpportunities(items []models.SeoBacklinkItem) []BacklinkOpportunity {
	// Benzersiz domainler
	seen := map[string]bool{}
	opps := []BacklinkOpportunity{}

	for _, item := range items {
		if seen[item.SourceDomain] || item.SpamScore > 70 {
			continue
		}
		seen[item.SourceDomain] = true

		dr := int(item.Rank / 10)
		if dr < 20 {
			continue // Kalitesiz siteleri atla
		}

		topic := "İçerik Paylaşımı"
		if strings.Contains(item.SourceDomain, "blog") {
			topic = "Blog"
		} else if strings.Contains(item.SourceDomain, "news") || strings.Contains(item.SourceDomain, "haber") {
			topic = "Haber"
		} else if strings.Contains(item.SourceDomain, "forum") {
			topic = "Forum"
		}

		opps = append(opps, BacklinkOpportunity{
			SiteName:    item.SourceDomain,
			SiteURL:     item.SourceURL,
			DR:          dr,
			CommonTopic: topic,
			Action:      "E-posta",
		})

		if len(opps) >= 10 {
			break
		}
	}

	return opps
}

func buildBacklinkAISummary(s models.SeoBacklinkSummary) string {
	status := "iyi"
	if s.DomainRating < 20 {
		status = "düşük"
	} else if s.DomainRating < 40 {
		status = "orta"
	}

	summary := fmt.Sprintf("Backlink durumu: %s. Toplam %d backlink, %d farklı siteden geliyor. Domain Rating: %d/100.",
		status, s.TotalBacklinks, s.ReferringDomains, s.DomainRating)

	if s.SpamLinks > 0 {
		summary += fmt.Sprintf(" %d zararlı link tespit edildi — Google'dan ceza almamak için temizlenmeli.", s.SpamLinks)
	}

	if s.NewBacklinks7d > 0 {
		summary += fmt.Sprintf(" Son 7 günde %d yeni link kazanıldı.", s.NewBacklinks7d)
	}

	if s.LostBacklinks7d > 0 {
		summary += fmt.Sprintf(" %d link kaybedildi.", s.LostBacklinks7d)
	}

	return summary
}
