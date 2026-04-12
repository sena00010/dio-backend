package services

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
	"gorm.io/datatypes"
)

// ═══════════════════════════════════════════════════
// CONTENT BRIEF SERVICE — LLM ile SEO içerik brief'i üret & kaydet
// ═══════════════════════════════════════════════════

// BriefStep — LLM'den dönen brief adımı
type BriefStep struct {
	Step    string `json:"step"`
	Content string `json:"content"`
}

// GenerateBriefRequest — Frontend'den gelen istek
type GenerateBriefRequest struct {
	ProjectID       uint     `json:"project_id"`
	Title           string   `json:"title"`
	OpportunityType string   `json:"opportunity_type"` // blog, category, product, guide
	TargetKeywords  []string `json:"target_keywords"`
	Position        float64  `json:"position"`
	Impressions     float64  `json:"impressions"`
	CTR             float64  `json:"ctr"`
	EstimatedClicks int      `json:"estimated_clicks"`
}

// GenerateContentBrief — LLM'den brief üretip DB'ye kaydet
func GenerateContentBrief(req GenerateBriefRequest) (*models.SeoContentBrief, error) {
	log.Printf("📝 Content Brief üretimi başlıyor: %s", req.Title)

	// LLM prompt'u oluştur
	keywordsStr := strings.Join(req.TargetKeywords, ", ")

	prompt := fmt.Sprintf(`Sen uzman bir SEO içerik stratejistisin. Aşağıdaki bilgilerle %s türünde bir içerik brief'i oluştur.

BAŞLIK: %s
HEDEF KELİMELER: %s
MEVCUT POZİSYON: %.1f
GÖSTERIM: %.0f
CTR: %%.1f

Lütfen şu JSON formatında yanıt ver:
{
  "summary": "Bu içeriğin neden önemli olduğunu ve beklenen etkiyi anlatan 2-3 cümle",
  "steps": [
    {"step": "Meta Başlık", "content": "SEO uyumlu meta başlık önerisi (max 60 karakter)"},
    {"step": "Meta Açıklama", "content": "Çekici meta description (max 155 karakter)"},
    {"step": "H1 Başlık", "content": "Sayfa ana başlığı"},
    {"step": "Giriş Paragrafı", "content": "İlk 100 kelimelik giriş metni — hedef kelimeyi doğal şekilde içermeli"},
    {"step": "İçerik Yapısı", "content": "H2 ve H3 alt başlıkları listesi (en az 5 adet)"},
    {"step": "CTA Önerisi", "content": "Sayfaya uygun call-to-action metni"},
    {"step": "Internal Link Önerileri", "content": "Site içi bağlantı önerileri (3-5 adet)"},
    {"step": "Kelime Yoğunluğu", "content": "Hedef kelime kullanım önerileri ve yoğunluk tavsiyesi"}
  ]
}

KURALLAR:
- Tüm içerik Türkçe olmalı
- SEO best practice'lerine uy
- Doğal ve insan odaklı yaz, keyword stuffing yapma
- SADECE geçerli JSON döndür, markdown veya açıklama ekleme`, req.OpportunityType, req.Title, keywordsStr, req.Position, req.Impressions, req.CTR)

	// LLM'den brief al
	var aiSummary string
	var briefSteps []BriefStep

	aiResponse := callLLMForBrief(prompt)
	if aiResponse != "" {
		// JSON parse
		var parsed struct {
			Summary string      `json:"summary"`
			Steps   []BriefStep `json:"steps"`
		}
		if err := json.Unmarshal([]byte(aiResponse), &parsed); err == nil {
			aiSummary = parsed.Summary
			briefSteps = parsed.Steps
		} else {
			log.Printf("⚠️ LLM JSON parse hatası: %v", err)
			aiSummary = aiResponse
		}
	}

	// Fallback: eğer LLM başarısız olduysa default steps
	if len(briefSteps) == 0 {
		briefSteps = []BriefStep{
			{Step: "Meta Başlık", Content: fmt.Sprintf("%s | En İyi Rehber", req.Title)},
			{Step: "Meta Açıklama", Content: fmt.Sprintf("%s hakkında kapsamlı rehber. Trendler, modeller ve öneriler.", req.Title)},
			{Step: "H1 Başlık", Content: req.Title},
			{Step: "Giriş Paragrafı", Content: fmt.Sprintf("%s arayanlar için hazırladığımız bu rehberde en güncel bilgileri bulabilirsiniz.", req.Title)},
			{Step: "İçerik Yapısı", Content: "H2: Nedir? | H2: En İyi Modeller | H2: Nasıl Seçilir? | H2: Fiyat Karşılaştırma | H2: SSS"},
			{Step: "CTA Önerisi", Content: "Hemen keşfet ve en uygun fiyatlarla sipariş ver!"},
			{Step: "Internal Link", Content: "Ana kategori, benzer ürünler ve blog sayfalarına linkler"},
			{Step: "Kelime Yoğunluğu", Content: fmt.Sprintf("Ana kelime '%s' her 100 kelimede 1-2 kez kullanılmalı", req.TargetKeywords[0])},
		}
		if aiSummary == "" {
			aiSummary = fmt.Sprintf("Bu içerik %s için optimize edilecek. Mevcut pozisyon %.1f, hedef ilk 10.", req.Title, req.Position)
		}
	}

	// JSON'a çevir
	keywordsJSON, _ := json.Marshal(req.TargetKeywords)
	stepsJSON, _ := json.Marshal(briefSteps)

	// DB'ye kaydet
	brief := models.SeoContentBrief{
		ProjectID:       req.ProjectID,
		Title:           req.Title,
		OpportunityType: req.OpportunityType,
		TargetKeywords:  datatypes.JSON(keywordsJSON),
		AiBriefSteps:    datatypes.JSON(stepsJSON),
		AiSummary:       aiSummary,
		Status:          "draft",
		Position:        req.Position,
		Impressions:     req.Impressions,
		CTR:             req.CTR,
		EstimatedClicks: req.EstimatedClicks,
	}

	if err := database.DB.Create(&brief).Error; err != nil {
		return nil, fmt.Errorf("brief kayıt hatası: %v", err)
	}

	log.Printf("✅ Content Brief kaydedildi: id=%d, title=%s", brief.ID, brief.Title)
	return &brief, nil
}

// callLLMForBrief — Gemini/OpenAI'dan brief yanıtı al
func callLLMForBrief(prompt string) string {
	// Gemini dene
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey != "" {
		result, err := utils.CallGeminiRawText(geminiKey, prompt)
		if err == nil && result != "" {
			// JSON temizle
			result = strings.TrimSpace(result)
			result = strings.TrimPrefix(result, "```json")
			result = strings.TrimPrefix(result, "```")
			result = strings.TrimSuffix(result, "```")
			return strings.TrimSpace(result)
		}
		log.Printf("⚠️ Gemini brief hatası: %v", err)
	}

	// OpenAI fallback
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey != "" {
		result, err := utils.CallOpenAIRawText(openaiKey, prompt)
		if err == nil && result != "" {
			result = strings.TrimSpace(result)
			result = strings.TrimPrefix(result, "```json")
			result = strings.TrimPrefix(result, "```")
			result = strings.TrimSuffix(result, "```")
			return strings.TrimSpace(result)
		}
		log.Printf("⚠️ OpenAI brief hatası: %v", err)
	}

	return ""
}

// GetContentBriefs — Projeye ait brief'leri listele
func GetContentBriefs(projectID uint) ([]models.SeoContentBrief, error) {
	var briefs []models.SeoContentBrief
	if err := database.DB.Where("project_id = ?", projectID).Order("created_at DESC").Find(&briefs).Error; err != nil {
		return nil, err
	}
	return briefs, nil
}

// ═══════════════════════════════════════════════════
// İÇERİK FIRSATI ÜRETİCİ — Kelimelerden otomatik fırsat tespiti
// ═══════════════════════════════════════════════════

type ContentOpportunityResponse struct {
	Title           string   `json:"title"`
	Badges          []Badge  `json:"badges"`
	Position        float64  `json:"position"`
	Impressions     string   `json:"impressions"`
	CTR             string   `json:"ctr"`
	Keywords        []string `json:"keywords"`
	EstimatedClicks int      `json:"estimated_clicks"`
	Type            string   `json:"type"`
}

type Badge struct {
	Label string `json:"label"`
	Color string `json:"color"`
}

// GetContentOpportunities — Kelime verisinden otomatik fırsat listesi oluştur
func GetContentOpportunities(projectID uint) ([]ContentOpportunityResponse, error) {
	keywords, err := GetRankedKeywords(projectID)
	if err != nil {
		return nil, err
	}

	if len(keywords) == 0 {
		return []ContentOpportunityResponse{}, nil
	}

	// Yüksek gösterimli ama düşük CTR'li kelimeleri filtrele (esnek eşikler)
	type candidate struct {
		kw    RankedKeyword
		score float64
	}

	var candidates []candidate
	for _, kw := range keywords {
		if kw.Impressions > 10 && kw.CTR < 10 {
			score := kw.Impressions * (10.0 - kw.CTR) / 100.0
			candidates = append(candidates, candidate{kw, score})
		}
	}

	// Fallback: filtreden hiç geçen yoksa en yüksek gösterimli kelimeleri al
	if len(candidates) == 0 {
		for _, kw := range keywords {
			candidates = append(candidates, candidate{kw, kw.Impressions})
		}
	}

	// Score'a göre sırala
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	opportunities := make([]ContentOpportunityResponse, 0)
	taken := map[string]bool{}

	for _, c := range candidates {
		kw := c.kw
		if taken[kw.Keyword] {
			continue
		}
		taken[kw.Keyword] = true

		// İlgili kelimeleri bul
		words := strings.Fields(kw.Keyword)
		var related []string
		for _, k := range keywords {
			if k.Keyword == kw.Keyword {
				continue
			}
			for _, w := range words {
				if len(w) > 3 && strings.Contains(k.Keyword, w) {
					related = append(related, k.Keyword)
					break
				}
			}
			if len(related) >= 4 {
				break
			}
		}

		// Badge'ları belirle
		var badges []Badge
		if kw.CurrentPosition > 20 {
			badges = append(badges, Badge{"Yeni", "new"})
		} else if kw.CurrentPosition < 15 && kw.Impressions > 3000 {
			badges = append(badges, Badge{"Trend", "trend"})
		} else {
			badges = append(badges, Badge{"CTR", "ctr"})
		}

		if kw.Intent == "transactional" || kw.Intent == "commercial" {
			badges = append(badges, Badge{"Kategori", "category"})
		} else {
			badges = append(badges, Badge{"Blog", "blog"})
		}

		estClicks := int(math.Round(kw.Impressions * 0.02 * (1 + func() float64 {
			if kw.CurrentPosition > 10 {
				return 0.5
			}
			return 0.2
		}())))

		// Başlık
		title := capitalizeFirst(kw.Keyword)
		if kw.Intent != "transactional" && kw.Intent != "commercial" {
			suffixes := []string{" — Stil Rehberi", " — Trendler", " — Kapsamlı Rehber", " Koleksiyonu"}
			title += suffixes[len(opportunities)%len(suffixes)]
		}

		impStr := formatK(kw.Impressions)

		opportunities = append(opportunities, ContentOpportunityResponse{
			Title:           title,
			Badges:          badges,
			Position:        kw.CurrentPosition,
			Impressions:     impStr,
			CTR:             fmt.Sprintf("%%%.1f", kw.CTR),
			Keywords:        append([]string{kw.Keyword}, related...),
			EstimatedClicks: estClicks,
			Type:            func() string { if kw.Intent == "transactional" { return "category" }; return "blog" }(),
		})

		if len(opportunities) >= 6 {
			break
		}
	}

	return opportunities, nil
}

func capitalizeFirst(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func formatK(n float64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", n/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", n/1000)
	}
	return fmt.Sprintf("%.0f", n)
}
