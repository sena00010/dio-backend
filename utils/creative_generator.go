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
// Kreatif Varyant Tipleri
// ──────────────────────────────────────────────

type CreativeConfig struct {
	Topics       []string `json:"topics"`
	Emphasis     string   `json:"emphasis"`
	Platforms    []string `json:"platforms"`
	Formats      []string `json:"formats"`
	VariantCount int      `json:"variant_count"`
}

type CreativeVariant struct {
	ID              string               `json:"id"`
	Name            string               `json:"name"`
	Format          string               `json:"format"`
	AiScore         int                  `json:"ai_score"`
	IsAiRecommended bool                 `json:"is_ai_recommended"`
	Emphasis        string               `json:"emphasis"`
	Headline        string               `json:"headline"`
	SubHeadline     string               `json:"sub_headline"`
	Caption         string               `json:"caption"`
	Cta             string               `json:"cta"`
	Hashtags        []string             `json:"hashtags"`
	Performance     PerformanceEstimate  `json:"performance"`
	VisualDesc      string               `json:"visual_description"`
	ColorPalette    []string             `json:"color_palette"`
	DesignNotes     string               `json:"design_notes"`
}

type PerformanceEstimate struct {
	EstimatedReach    string `json:"estimated_reach"`
	EngagementRate    string `json:"engagement_rate"`
	BestTime          string `json:"best_time"`
	BrandFit          int    `json:"brand_fit"`
	CompetitorDensity string `json:"competitor_density"`
}

// ──────────────────────────────────────────────
// Ana fonksiyon — Kreatif Varyant Üretici
// ──────────────────────────────────────────────

func GenerateCreativeVariants(company models.Company, cfg CreativeConfig) ([]CreativeVariant, error) {
	prompt := buildCreativePrompt(company, cfg)

	// Gemini dene
	result, err := callGeminiForCreatives(prompt)
	if err == nil {
		return result, nil
	}
	log.Printf("⚠️ Gemini kreatif üretme hatası, OpenAI'ye geçiliyor: %v", err)

	// OpenAI fallback
	result, err = callOpenAIForCreatives(prompt)
	if err != nil {
		return nil, fmt.Errorf("hem Gemini hem OpenAI kreatif üretme başarısız: %v", err)
	}
	return result, nil
}

// ──────────────────────────────────────────────
// AI Prompt — 30 Yıllık E-Ticaret & Tasarım Uzmanı
// ──────────────────────────────────────────────

func buildCreativePrompt(company models.Company, cfg CreativeConfig) string {
	now := time.Now()
	currentDate := now.Format("2 January 2006")

	brandContext := buildCreativeBrandContext(company)
	topicsStr := strings.Join(cfg.Topics, ", ")
	platformsStr := strings.Join(cfg.Platforms, ", ")
	formatsStr := strings.Join(cfg.Formats, ", ")

	// Vurgu açıklamaları
	emphasisMap := map[string]string{
		"yenilik":      "YENİLİK & HEYECAN — \"İlk sen gör\" hissi yaratmalısın. Ürünü/hizmeti çığır açan bir yenilik, trend belirleyen bir hareket olarak konumlandır. FOMO (kaçırma korkusu) oluştur. Erken erişim, sneak peek, \"sınırlı\" dili kullan.",
		"fiyat":        "FİYAT & KAMPANYA — İndirim, fırsat, değer vurgusu yap. Aciliyet yaratan dil kullan: \"Son 48 saat\", \"Stoklar tükeniyor\", \"Sadece bugün\". Fiyat karşılaştırması ile algı yarat.",
		"kalite":       "ÜRÜN KALİTESİ — Materyal, dayanıklılık, ustalık vurgusu yap. Premium hissiyat yarat. Detay çekimleri, ingrediyenler, production bilgileri ile güven aşıla.",
		"sosyal_kanit": "SOSYAL KANIT — Gerçek müşteri yorumları, before/after, kullanıcı deneyimleri. UGC (User Generated Content) hissi ver. Güven oluştur, \"binlerce müşteri\" dilini kullan.",
	}
	emphasisDesc := emphasisMap[cfg.Emphasis]
	if emphasisDesc == "" {
		emphasisDesc = emphasisMap["yenilik"]
	}

	return fmt.Sprintf(`Sen dünyanın en ödüllü kreatif direktörü, dijital pazarlama stratejisti ve sektör analistisin. 30 yıldır Apple, Nike, Coca-Cola, L'Oréal gibi global devlerin kampanyalarını yönetiyorsun. SEO, sosyal medya trendleri, tüketici psikolojisi, renk teorisi, tipografi ve görsel hiyerarşi konularında eşsiz bir uzmansın.

Bugünün tarihi: %s

%s

═══════════════════════════════════════════
GÖREV: Bu marka için profesyonel kreatif varyantlar üret
═══════════════════════════════════════════

KONU(LAR): %s
VURGU STRATEJİSİ: %s
PLATFORMLAR: %s
FORMATLAR: %s
VARYANT SAYISI: %d

Her varyant tamamen FARKLI bir kreatif yaklaşım olmalı. Her biri farklı bir açıdan konuya yaklaşmalı, farklı görsel konsept, farklı ton, farklı CTA kullanmalı. Birbirinin kopyası olan varyantlar KABUL EDİLMEZ.

HER VARYANT İÇİN ŞUNLARI ÜRET (JSON formatında):

1. "id": Unique ID (ör: "variant_a", "variant_b")
2. "name": Varyant adı (ör: "Varyant A", "Varyant B")
3. "format": Hangi format için optimize edildi — %s formatlarından biri
4. "ai_score": 60-98 arası kreatif kalite skoru (en iyi varyantlar 85+)
5. "is_ai_recommended": Boolean — en yüksek skora sahip varyant(lar) true
6. "emphasis": Vurgu stratejisi
7. "headline": Görselde kullanılacak ana başlık (max 6 kelime, çarpıcı, Türkçe)
8. "sub_headline": Alt başlık (max 10 kelime, açıklayıcı)
9. "caption": Sosyal medya paylaşım metni — platformun diline uygun, doğal, samimi (80-150 kelime). SEO ve engagement odaklı. Emoji kullan ama abartma. Hook cümlesiyle başla. CTA içersin. Marka sesine sadık kal.
10. "cta": Call-to-Action butonu metni (ör: "Keşfet", "İncele", "Hemen Al", "Koleksiyonu Gör")
11. "hashtags": Hashtag dizisi (8-12 adet, sektöre özel + trend hashtagler + marka hashtagi)
12. "performance": Performans tahmin objesi:
    - "estimated_reach": Tahmini erişim aralığı (ör: "4.8K – 7.2K")
    - "engagement_rate": Tahmini etkileşim oranı (ör: "%%4.8 – 6.4")
    - "best_time": En iyi paylaşım zamanı (ör: "Salı 19:00 – 21:00")
    - "brand_fit": Marka uyum skoru (80-100 arası integer)
    - "competitor_density": Rakip yoğunluğu ("Düşük", "Orta", "Yüksek")
13. "visual_description": (CRITICAL — This will be used to generate a real AI image. Write it as a PROFESSIONAL IMAGE GENERATION PROMPT in English.)
    MANDATORY RULES for visual_description:
    - Write EXACTLY like a professional photographer briefing a photoshoot
    - Start with the SCENE TYPE: "A cinematic photograph of...", "An editorial close-up of...", "A lifestyle flat-lay featuring..."
    - Include SPECIFIC SUBJECTS relevant to the brand's sector and products:
      * HR/Recruitment firm → Real people in modern offices, handshakes, team celebrations, interview settings
      * Restaurant → Beautifully plated food, steam rising, dining ambiance, chef at work
      * Fashion → Model wearing clothes, fabric textures, styling details
      * Tech → Product on desk, screens glowing, sleek workspace
      * E-commerce → Product hero shots, unboxing moments, customer hands holding items
    - Describe LIGHTING in photography terms: "warm golden hour backlighting", "soft diffused window light", "dramatic chiaroscuro", "clean studio lighting with subtle rim light"
    - Describe CAMERA parameters: "shot at f/1.8 with shallow depth of field", "35mm wide angle", "macro close-up with bokeh"
    - Include MOOD/ATMOSPHERE: "cozy and inviting", "corporate yet warm", "energetic and dynamic"
    - Reference the BRAND COLORS naturally: "accents of [brand hex] in the props/clothing/background/lighting gel"
    - Think about the TARGET AUDIENCE: images should feature people/settings that resonate with them
    - ABSOLUTE ZERO-TEXT RULE: Do NOT include ANY text, letters, words, numbers, logos, watermarks, signs, labels, or typography in the image. PURE VISUAL ONLY.
    - Make it SCROLL-STOPPING: the kind of image that would make someone pause on Instagram
    - Minimum 40 words, maximum 120 words per visual_description
14. "color_palette": 3-4 hex renk kodu dizisi — marka renklerini baz al ama yaratıcı ol
15. "design_notes": Tasarımcı notu — bu varyantı özel kılan nedir, neden bu yaklaşım seçildi (1-2 cümle, Türkçe)

CAPTION YAZIM KURALLARI:
- Hook cümlesiyle başla (ilk 2 saniyede dikkat çek)
- Platform diline uygun ol (Instagram: estetik, TikTok: samimi/eğlenceli, Facebook: bilgilendirici)
- Marka tonunu koru: %s
- SEO anahtar kelimelerini doğal şekilde kat
- Son cümle CTA olsun
- Sosyal medya kullanıcı adlarından bahset (varsa)
- Yasak kelimelerden kaçın

SKOR DAĞILIMI:
- En az 1 varyant 90+ skor → AI'ın birincil önerisi
- 2-3 varyant 75-89 → güçlü alternatifler
- Geri kalanlar 60-74 → deneysel/cesur yaklaşımlar

YANITINI SADECE GEÇERLİ BİR JSON DİZİSİ OLARAK VER. Markdown yok, açıklama yok. Her öğe yukarıdaki tüm alanları içermeli.`, currentDate, brandContext, topicsStr, emphasisDesc, platformsStr, formatsStr, cfg.VariantCount, formatsStr, company.ToneOfVoice)
}

// ──────────────────────────────────────────────
// Marka bağlamı oluşturucu (kreatif)
// ──────────────────────────────────────────────

func buildCreativeBrandContext(company models.Company) string {
	ctx := fmt.Sprintf(`MARKA PROFİLİ (Tüm içerik bu bilgiler ışığında üretilmelidir):
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🏢 Marka: %s
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
	if company.Goal != "" {
		ctx += fmt.Sprintf("\n🎯 İş Hedefi: %s", company.Goal)
	}
	if company.Website != "" {
		ctx += fmt.Sprintf("\n🌐 Website: %s", company.Website)
	}
	if company.ContentLanguage != "" {
		ctx += fmt.Sprintf("\n🌍 İçerik Dili: %s", company.ContentLanguage)
	}
	if company.Platforms != "" {
		ctx += fmt.Sprintf("\n📱 Aktif Platformlar: %s", company.Platforms)
	}

	// ── Font Direktifi ──
	if company.BrandFont != "" {
		ctx += fmt.Sprintf(`

✏️ MARKA FONTU: "%s"
→ Tüm headline'larda ve ana görsel metinlerde bu font kullanılmalıdır.
→ design_layers'daki headline layer'ı bu fontu kullanmalı.
→ visual_description'da bu tipografi stiline uygun bir kompozisyon düşün.`, company.BrandFont)
	}

	// ── Renk Sistemi ──
	if company.BrandColors != "" {
		ctx += fmt.Sprintf(`

🎨 MARKA RENK PALETİ: %s
→ ZORUNLU: color_palette alanı mutlaka bu renkleri içermelidir.
→ visual_description'da bu renkleri ışık, kıyafet, arka plan, prop olarak entegre et.
→ CTA ve badge elementlerinde marka renklerini kullan.
→ Headline rengi marka paletinden seçilmeli, açık arka planlarda koyu marka rengi, koyu arka planlarda açık marka rengi kullan.`, company.BrandColors)
	}

	// ── Ton Direktifi ──
	if company.ToneOfVoice != "" {
		ctx += fmt.Sprintf(`

🎤 MARKA SES TONU: "%s"
→ TÜM metinler (headline, sub_headline, caption, CTA) bu tonda yazılmalıdır.
→ Bu ton markayı diğerlerinden ayıran DNA'dır — her kelimede hissedilmeli.`, company.ToneOfVoice)
	}

	// ── Sosyal Medya Hesapları ──
	if company.SocialMedia != "" {
		ctx += fmt.Sprintf("\n\n📱 SOSYAL MEDYA HESAPLARI: %s", company.SocialMedia)
		// JSON parse et, kullanıcı adlarını çıkar
		var socials map[string]string
		if err := json.Unmarshal([]byte(company.SocialMedia), &socials); err == nil {
			var activeAccounts []string
			for platform, username := range socials {
				if username != "" {
					activeAccounts = append(activeAccounts, fmt.Sprintf("%s: @%s", platform, strings.TrimPrefix(username, "@")))
				}
			}
			if len(activeAccounts) > 0 {
				ctx += fmt.Sprintf(`
→ Aktif hesaplar: %s
→ Caption'larda bu kullanıcı adlarını doğal şekilde kullan (ör: "Detaylar @username profilimizde")
→ CTA'larda bu hesaplara yönlendir`, strings.Join(activeAccounts, ", "))
			}
		}
	}

	// ── Yasaklı Kelimeler ──
	if company.ForbiddenWords != "" {
		ctx += fmt.Sprintf(`

🚫 YASAKLI KELİMELER: %s
→ Bu kelimeler hiçbir alanda (headline, caption, CTA, hashtag) ASLA kullanılamaz.
→ Bu kelimelerin yakın anlamlarından da kaçın.`, company.ForbiddenWords)
	}

	ctx += "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	return ctx
}

// ──────────────────────────────────────────────
// Gemini API — Kreatif Üretici
// ──────────────────────────────────────────────

func callGeminiForCreatives(prompt string) ([]CreativeVariant, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY tanımlı değil")
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	url := GeminiTextURL(apiKey)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gemini isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini API hatası %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
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

	var variants []CreativeVariant
	if err := json.Unmarshal([]byte(content), &variants); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %v — raw: %s", err, content[:min(len(content), 400)])
	}

	log.Printf("✅ Gemini: %d kreatif varyant üretildi", len(variants))
	return variants, nil
}

// ──────────────────────────────────────────────
// OpenAI API — Kreatif Üretici
// ──────────────────────────────────────────────

func callOpenAIForCreatives(prompt string) ([]CreativeVariant, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY tanımlı değil")
	}

	reqBody := openAIRequest{
		Model:       GetAiModel("gpt_fast_general", "gpt-4o-mini"),
		Temperature: 0.9,
		MaxTokens:   8000,
		Messages: []openAIMessage{
			{Role: "system", Content: "Sen dünyanın en ödüllü kreatif direktörü ve dijital pazarlama stratejistisin. 30 yıllık deneyimle Apple, Nike, Coca-Cola gibi devlerin kampanyalarını yönettin. SEO, sosyal medya, tüketici psikolojisi, tipografi ve renk teorisinde eşsiz bir uzmansın. Yanıtlarını SADECE geçerli JSON dizisi olarak ver. Markdown KULLANMA."},
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API hatası %d: %s", resp.StatusCode, string(body))
	}

	var oaiResp openAIResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return nil, fmt.Errorf("yanıt parse hatası: %v", err)
	}

	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("boş yanıt")
	}

	content := strings.TrimSpace(oaiResp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var variants []CreativeVariant
	if err := json.Unmarshal([]byte(content), &variants); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %v — raw: %s", err, content[:min(len(content), 400)])
	}

	log.Printf("✅ OpenAI: %d kreatif varyant üretildi", len(variants))
	return variants, nil
}
