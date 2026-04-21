package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ──────────────────────────────────────────────
// Konu Önerisi Tipleri
// ──────────────────────────────────────────────

type TopicSuggestion struct {
	TopicTitle       string   `json:"topic_title"`
	Category         string   `json:"category"`
	Score            int      `json:"score"`
	AiReasoning      string   `json:"ai_reasoning"`
	TrendData        string   `json:"trend_data"`
	SuggestedFormats []string `json:"suggested_formats"`
	UrgencyTag       string   `json:"urgency_tag,omitempty"`
	DaysLeft         *int     `json:"days_left,omitempty"`
}

// ──────────────────────────────────────────────
// Ana fonksiyon — Konu Önerisi Üretici
// ──────────────────────────────────────────────

func GenerateTopicSuggestions(company models.Company) ([]TopicSuggestion, error) {
	prompt := buildTopicSuggestionPrompt(company)

	// Önce Gemini dene
	result, err := callGeminiForTopics(prompt)
	if err == nil {
		return result, nil
	}
	log.Printf("⚠️ Gemini konu önerisi hatası, OpenAI'ye geçiliyor: %v", err)

	// OpenAI fallback
	result, err = callOpenAIForTopics(prompt)
	if err != nil {
		return nil, fmt.Errorf("hem Gemini hem OpenAI konu önerisi başarısız: %v", err)
	}
	return result, nil
}

// ──────────────────────────────────────────────
// AI Prompt — E-Ticaret Uzmanı Rolü
// ──────────────────────────────────────────────

func buildTopicSuggestionPrompt(company models.Company) string {
	// Bugünün tarihini al — mevsimsel analiz için
	now := time.Now()
	currentDate := now.Format("2 January 2006")
	currentMonth := now.Format("January")

	// Yaklaşan özel günleri belirle
	upcomingEvents := getUpcomingEvents(now)

	// Marka bağlamını oluştur
	brandContext := buildBrandContextForTopics(company)

	return fmt.Sprintf(`Sen dünyanın en iyi e-ticaret ve dijital pazarlama uzmanısın. 15 yıllık deneyiminle yüzlerce markayı başarıya taşıdın. Sektör trendlerini, rakip analizlerini ve tüketici davranışlarını bir satranç ustası gibi önceden okuyorsun.

BUGÜNÜN TARİHİ: %s (Ay: %s)

YAKINLAŞAN ÖZEL GÜNLER VE FIRSATLAR:
%s

%s

═══════════════════════════════════════════
GÖREV: Bu markaya özel, profesyonel konu önerileri üret
═══════════════════════════════════════════

Bu marka için tam olarak 8 adet sosyal medya konu önerisi hazırla. Her öneri şu kategorilerden birine ait olmalı:
- "Bugün için": Bugün veya bu hafta hemen yapılması gereken acil/fırsat içerikler
- "Trendde": Şu anda sektörde yükselen, ilgi gören konular
- "Planlı": Yaklaşan özel günler, kampanyalar için önceden planlanması gereken içerikler
- "Evergreen": Her zaman geçerli, sürekli değer üreten içerik konuları

HER ÖNERİ İÇİN ŞUNLARI BELİRLE:
1. "topic_title": Kısa, çarpıcı konu başlığı (Türkçe, max 8 kelime)
2. "category": Yukarıdaki 4 kategoriden biri
3. "score": 60-98 arası AI güven skoru (ne kadar etkili olacağına dair tahminin)
4. "ai_reasoning": Neden bu konuyu önerdiğinin profesyonel açıklaması. Rakip analizi, trend verisi, sektör bilgisi ile destekle. Bu açıklama markanın sektörüne ÖZGü olmalı. (2-3 cümle, Türkçe)
5. "trend_data": Tahmini trend artış yüzdesi (ör: "%%245 artış", "%%120 artış")
6. "suggested_formats": Hangi formatlarda yayınlanmalı — en az 2 format seç: ["Feed", "Story", "Reels", "Carousel", "TikTok", "YouTube"]
7. "urgency_tag": (opsiyonel) Eğer acil veya özel bir durum varsa etiket (ör: "Bugün için — Acil", "Bugün için — Fırsat", "Sosyal Kanıt", "Organik Erişim", "Sezonluk")
8. "days_left": (opsiyonel) Eğer planlı bir etkinlik varsa kaç gün kaldığı (sadece integer)

ÖNEMLİ KURALLAR:
- Her öneri bu MARKANIN sektörüne, ürünlerine ve hedef kitlesine birebir UYGUN olmalı
- Genel, herhangi bir markaya uygulanabilir öneriler VERME. Her öneri bu markanın DNA'sına işlenmiş olmalı
- ai_reasoning alanında rakip analizi yap: "Rakipler henüz...", "Sektörde X trendi...", "Hedef kitleniz bu konuda..."
- Score dağılımı dengeli olsun: 2 adet 85+, 3 adet 75-85, 3 adet 65-75
- Kategori dağılımı: 2 "Bugün için", 2 "Trendde", 2 "Planlı", 2 "Evergreen"
- Mevsimsel analiz yap: %s ayındayız, buna uygun öneriler ver
- Eğer yaklaşan bayram/özel gün varsa mutlaka "Planlı" kategorisinde yer alsın

YANITINI SADECE GEÇERLİ BİR JSON DİZİSİ OLARAK VER. Markdown yok, açıklama yok.

Örnek format:
[{"topic_title":"...","category":"Bugün için","score":92,"ai_reasoning":"...","trend_data":"%%X artış","suggested_formats":["Feed","Reels"],"urgency_tag":"Bugün için — Acil"},...]`, currentDate, currentMonth, upcomingEvents, brandContext, currentMonth)
}

// ──────────────────────────────────────────────
// Marka bağlamı oluşturucu
// ──────────────────────────────────────────────

func buildBrandContextForTopics(company models.Company) string {
	ctx := fmt.Sprintf(`MARKA PROFİLİ (Konu önerileri bu markanın DNA'sına uygun olmalıdır):
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🏢 Marka Adı: %s
🏭 Sektör: %s`, company.Name, company.Sector)

	if company.BrandType != "" {
		ctx += fmt.Sprintf("\n🏷️ Marka Tipi: %s", company.BrandType)
	}
	if company.Description != "" {
		ctx += fmt.Sprintf("\n📝 Marka Açıklaması: %s", company.Description)
	}
	if company.TargetAudience != "" {
		ctx += fmt.Sprintf("\n🎯 Hedef Kitle: %s", company.TargetAudience)
	}
	if company.ProductCategories != "" {
		ctx += fmt.Sprintf("\n📦 Ürün/Hizmet Kategorileri: %s", company.ProductCategories)
	}
	if company.CompetitiveEdge != "" {
		ctx += fmt.Sprintf("\n⚔️ Rekabet Avantajı: %s", company.CompetitiveEdge)
	}
	if company.Slogan != "" {
		ctx += fmt.Sprintf("\n💬 Slogan: %s", company.Slogan)
	}
	if company.ToneOfVoice != "" {
		ctx += fmt.Sprintf("\n🎤 Marka Tonu: %s → Önerilerin dili bu tona uygun olmalı", company.ToneOfVoice)
	}
	if company.Goal != "" {
		ctx += fmt.Sprintf("\n🎯 İş Hedefi: %s → Öneriler bu hedefe hizmet etmeli", company.Goal)
	}
	if company.Platforms != "" {
		ctx += fmt.Sprintf("\n📱 Aktif Platformlar: %s → Önerileri bu platformların formatlarına uygun üret", company.Platforms)
	}
	if company.Website != "" {
		ctx += fmt.Sprintf("\n🌐 Website: %s", company.Website)
	}
	if company.ContentLanguage != "" {
		ctx += fmt.Sprintf("\n🌍 İçerik Dili: %s", company.ContentLanguage)
	}
	if company.BrandType != "" {
		ctx += fmt.Sprintf("\n🏷️ Marka Tipi: %s", company.BrandType)
	}
	if company.BrandColors != "" {
		ctx += fmt.Sprintf("\n🎨 Marka Renkleri: %s → suggested_formats seçerken görsel yoğun formatları tercih et", company.BrandColors)
	}
	if company.SocialMedia != "" {
		ctx += fmt.Sprintf("\n📱 Sosyal Medya: %s", company.SocialMedia)
	}
	if company.ForbiddenWords != "" {
		ctx += fmt.Sprintf("\n🚫 Yasaklı Kelimeler: %s → Konu başlıklarında bunlardan KAÇIN", company.ForbiddenWords)
	}

	ctx += "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

	return ctx
}

// ──────────────────────────────────────────────
// Yaklaşan özel günler ve etkinlikler
// ──────────────────────────────────────────────

func getUpcomingEvents(now time.Time) string {
	type Event struct {
		Name     string
		Date     time.Time
		DaysLeft int
	}

	year := now.Year()

	// Türkiye'deki önemli günler ve uluslararası etkinlikler
	allEvents := []struct {
		Name  string
		Month int
		Day   int
	}{
		{"Yılbaşı", 1, 1},
		{"Sevgililer Günü", 2, 14},
		{"Kadınlar Günü", 3, 8},
		{"Anneler Günü", 5, 11},
		{"23 Nisan Ulusal Egemenlik ve Çocuk Bayramı", 4, 23},
		{"1 Mayıs İşçi Bayramı", 5, 1},
		{"19 Mayıs Atatürk'ü Anma", 5, 19},
		{"Babalar Günü", 6, 15},
		{"15 Temmuz Demokrasi ve Milli Birlik Günü", 7, 15},
		{"30 Ağustos Zafer Bayramı", 8, 30},
		{"Öğretmenler Günü", 11, 24},
		{"29 Ekim Cumhuriyet Bayramı", 10, 29},
		{"Black Friday", 11, 28},
		{"Cyber Monday", 12, 1},
		{"Yılbaşı Kampanyaları", 12, 25},
		// Mevsimsel
		{"Yaz Sezonu Başlangıcı", 6, 1},
		{"Kış Sezonu Başlangıcı", 12, 1},
		{"Okul Dönemi Kampanyaları", 9, 1},
		{"İlkbahar Koleksiyonu", 3, 1},
		{"Sonbahar Koleksiyonu", 9, 15},
	}

	var upcoming []string
	for _, e := range allEvents {
		eventDate := time.Date(year, time.Month(e.Month), e.Day, 0, 0, 0, 0, now.Location())
		if eventDate.Before(now) {
			eventDate = time.Date(year+1, time.Month(e.Month), e.Day, 0, 0, 0, 0, now.Location())
		}
		daysLeft := int(eventDate.Sub(now).Hours() / 24)
		if daysLeft <= 60 && daysLeft >= 0 {
			upcoming = append(upcoming, fmt.Sprintf("- %s (%d gün sonra)", e.Name, daysLeft))
		}
	}

	if len(upcoming) == 0 {
		return "- Yakın tarihte özel bir gün bulunmuyor, mevsimsel trendlere odaklan."
	}

	return strings.Join(upcoming, "\n")
}

// ──────────────────────────────────────────────
// Gemini API çağrısı — Konu Önerileri
// ──────────────────────────────────────────────

func callGeminiForTopics(prompt string) ([]TopicSuggestion, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY tanımlı değil")
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	url := GeminiTextURL(apiKey)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("request oluşturma hatası: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gemini isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini API hatası %d: %s", resp.StatusCode, string(respBody))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("yanıt parse hatası: %v", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("boş yanıt")
	}

	content := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var suggestions []TopicSuggestion
	if err := json.Unmarshal([]byte(content), &suggestions); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %v — raw: %s", err, content[:min(len(content), 300)])
	}

	log.Printf("✅ Gemini: %d konu önerisi üretildi", len(suggestions))
	return suggestions, nil
}

// ──────────────────────────────────────────────
// OpenAI API çağrısı — Konu Önerileri
// ──────────────────────────────────────────────

func callOpenAIForTopics(prompt string) ([]TopicSuggestion, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY tanımlı değil")
	}

	reqBody := openAIRequest{
		Model:       GetAiModel("gpt_fast_general", "gpt-4o-mini"),
		Temperature: 0.85,
		MaxTokens:   4096,
		Messages: []openAIMessage{
			{Role: "system", Content: "Sen dünyanın en profesyonel e-ticaret ve dijital pazarlama uzmanısın. Sektör trendlerini, rakip boşluklarını ve mevsimsel fırsatları bir satranç ustası gibi analiz ediyorsun. Yanıtlarını SADECE geçerli JSON olarak ver. Markdown formatı KULLANMA."},
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("request oluşturma hatası: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API hatası %d: %s", resp.StatusCode, string(respBody))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("yanıt parse hatası: %v", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("boş yanıt")
	}

	content := strings.TrimSpace(openAIResp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var suggestions []TopicSuggestion
	if err := json.Unmarshal([]byte(content), &suggestions); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %v — raw: %s", err, content[:min(len(content), 300)])
	}

	log.Printf("✅ OpenAI: %d konu önerisi üretildi", len(suggestions))
	return suggestions, nil
}

// ──────────────────────────────────────────────
// Trend Radar — Marka sektörüne özel trend verileri
// ──────────────────────────────────────────────

func GenerateTrendRadar(company models.Company) (string, error) {
	prompt := fmt.Sprintf(`Sen bir dijital pazarlama trend analisti olarak çalışıyorsun. Aşağıdaki marka profili için sektöre özel trend analizi ve rakip boşluk analizi yap.

MARKA: %s
SEKTÖR: %s
ÜRÜN KATEGORİLERİ: %s
HEDEF KİTLE: %s
REKABET AVANTAJI: %s

Aşağıdaki JSON formatında yanıt ver:
{
  "trend_radar": [
    {"keyword": "trend anahtar kelime", "source": "Google/TikTok/Instagram", "change": "+%%XXX"},
    ... (6 adet)
  ],
  "competitor_gaps": [
    {"title": "Boşluk başlığı", "desc": "Neden bu bir fırsat? (1-2 cümle)"},
    ... (3 adet)
  ]
}

KURALLAR:
- Trendler bu markanın SEKTÖRÜNE özel olmalı
- Anahtar kelimeler Türkçe olmalı
- Artış yüzdeleri gerçekçi ama dikkat çekici olmalı (%%40 - %%300 arası)
- Rakip boşlukları bu sektörde gerçekten var olan fırsatları yansıtmalı
- SADECE JSON döndür, markdown veya açıklama ekleme`, company.Name, company.Sector, company.ProductCategories, company.TargetAudience, company.CompetitiveEdge)

	// Gemini dene
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey != "" {
		result, err := callGeminiRaw(apiKey, prompt)
		if err == nil {
			return result, nil
		}
		log.Printf("⚠️ Gemini trend radar hatası: %v", err)
	}

	// OpenAI fallback
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		return "", fmt.Errorf("API anahtarları tanımlı değil")
	}

	return callOpenAIRaw(openaiKey, prompt)
}

func callGeminiRaw(apiKey, prompt string) (string, error) {
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	url := GeminiTextURL(apiKey)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API hatası %d", resp.StatusCode)
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", err
	}

	if len(geminiResp.Candidates) == 0 {
		return "", fmt.Errorf("boş yanıt")
	}

	content := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content), nil
}

func callOpenAIRaw(apiKey, prompt string) (string, error) {
	reqBody := openAIRequest{
		Model:       GetAiModel("gpt_fast_general", "gpt-4o-mini"),
		Temperature: 0.7,
		MaxTokens:   2000,
		Messages: []openAIMessage{
			{Role: "system", Content: "Sen bir dijital pazarlama trend analistisin. SADECE geçerli JSON döndür."},
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API hatası %d", resp.StatusCode)
	}

	var oaiResp openAIResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return "", err
	}

	if len(oaiResp.Choices) == 0 {
		return "", fmt.Errorf("boş yanıt")
	}

	content := strings.TrimSpace(oaiResp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content), nil
}
