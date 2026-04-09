package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

type AdStudioPreset struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Platform      string `json:"platform"`
	FormatLabel   string `json:"format_label"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	BadgeLabel    string `json:"badge_label"`
	BadgeTone     string `json:"badge_tone"`
	Score         int    `json:"score"`
	TextPosition  string `json:"text_position"`
	TextAlign     string `json:"text_align"`
	IsRecommended bool   `json:"is_recommended"`
	DesignNotes   string `json:"design_notes"`
}

type AdCopyPayload struct {
	Headline    string `json:"headline"`
	SubHeadline string `json:"sub_headline"`
	CTA         string `json:"cta"`
}

func inferAdCreativeDirection(referenceSummary string, preset AdStudioPreset) string {
	summary := strings.ToLower(referenceSummary)

	switch {
	case strings.Contains(summary, "cat") || strings.Contains(summary, "pet") || strings.Contains(summary, "kitten"):
		return "Build a warm, trustworthy pet-care campaign mood. Keep the actual product pack dominant, and if any contextual styling is implied, use only soft ingredient or pet-lifestyle cues that support the same pet-food category."
	case strings.Contains(summary, "packaged") || strings.Contains(summary, "package") || strings.Contains(summary, "bag"):
		return "Treat the image as a retail-pack campaign asset. Preserve the pack front and shelf-recognition while elevating lighting, depth, and campaign energy."
	default:
		if preset.Width > preset.Height {
			return "Use a campaign layout with a strong product hero and a clearly protected text-safe area for later social ad copy placement."
		}
		return "Create a scroll-stopping vertical or square campaign visual while keeping the uploaded product unmistakably recognizable."
	}
}

func DefaultAdStudioPresets() []AdStudioPreset {
	return []AdStudioPreset{
		{Key: "instagram_story", Name: "Instagram Story — Urun Tanitim", Platform: "Instagram", FormatLabel: "Instagram Story", Width: 1080, Height: 1920, BadgeLabel: "Instagram Story", BadgeTone: "warning", Score: 95, TextPosition: "bottom_left", TextAlign: "left", IsRecommended: true, DesignNotes: "Dikey, enerjik ve ilk 1 saniyede dikkat ceken sosyal reklam kurgusu."},
		{Key: "feed_square", Name: "Feed Kare — Kampanya", Platform: "Instagram", FormatLabel: "Feed Kare", Width: 1080, Height: 1080, BadgeLabel: "Feed Kare", BadgeTone: "warning", Score: 91, TextPosition: "bottom_left", TextAlign: "left", DesignNotes: "Feed akisi icin temiz urun odagi ve net teklif alani."},
		{Key: "facebook_banner", Name: "Facebook Banner — Lansman", Platform: "Facebook", FormatLabel: "Facebook Banner", Width: 1200, Height: 628, BadgeLabel: "Facebook Banner", BadgeTone: "info", Score: 88, TextPosition: "left", TextAlign: "left", DesignNotes: "Genis yatay oran, baslik ve kampanya mesaji icin rahat negatif alan."},
		{Key: "trendyol_banner", Name: "Trendyol Banner — Firsat", Platform: "Trendyol", FormatLabel: "Trendyol Banner", Width: 1200, Height: 400, BadgeLabel: "Trendyol Banner", BadgeTone: "success", Score: 86, TextPosition: "left", TextAlign: "left", DesignNotes: "Pazaryeri odakli, net teklif ve fiyat algisi yaratan banner kompozisyonu."},
	}
}

func BuildAdStudioPrompt(company models.Company, studio models.ProductImageStudio, preset AdStudioPreset, referenceSummary string, copy AdCopyPayload) string {
	brandColors := company.BrandColors
	if brandColors == "" {
		brandColors = `["#F04E23","#111111","#FFF7F2"]`
	}

	categoryHint := studio.Category
	if categoryHint == "" {
		categoryHint = "Otomatik algilanan urun kategorisi"
	}

	return fmt.Sprintf(`Sen yuklenen urun gorselinden platforma hazir sosyal ticaret kreatifi uretecek kidemli bir performans pazarlama art direktorusun.

TEMEL TALIMAT:
- Kaynak gercek sadece yuklenen gorsel ve yuklenen urundur.
- Bu urunu kullanarak guclu, estetik ve paylasilabilir bir kampanya gorseli olustur.
- Sonuc Hepsiburada, Trendyol ve sosyal medya hesaplarinda paylasilabilecek seviyede olmali.
- Yeni bir urun tasarlama, urun tipini degistirme, farkli paket olusturma.
- Gorselin icine DIO, ekstra logo, watermark veya alakasiz marka unsuru ekleme.

KAYNAK URUN OZETI:
%s

MARKA BAGLAMI:
- Marka: %s
- Sektor: %s
- Kategori: %s
- Hedef kitle: %s
- Iletisim tonu: %s
- Renkler: %s

KREATIF OZELLIKLERI:
- Format: %s
- Platform: %s
- Tuval: %dx%d
- Temiz kalmasi gereken metin alani: %s
- Metin hizalama beklentisi: %s
- Desteklenecek baslik: "%s"
- Desteklenecek alt baslik: "%s"
- CTA tonu: "%s"
- Tasarim notu: %s
- Yaratıcı yon: %s

ZORUNLU KURALLAR:
- Yuklenen urun kimligi aynen korunmali.
- Bu is yeni urun uretimi degil, yuklenen urunun kampanya revizesidir.
- Paketli bir evcil hayvan mamasiysa ayni paketli mama karakteri korunmali.
- Urun ana kahraman olmali ve net secilmeli.
- Daha sonra metin eklenebilmesi icin temiz bir guvenli alan birak.
- Kompozisyon premium, campaign-ready ve donusum odakli olmali.
- Gorselin icine yeni text, logo, label, watermark, UI veya kampanya rozeti basma.
- DIO veya ekstra marka kimligi ekleme.
- Arka plan ve destek ogeleri platformla ve urun kategorisiyle uyumlu olsun.
- Destek ogeleri urunden rol calmamali.
- Banner formatlarda yatay akis daha guclu olsun.
- Story formatlarda daha etkileyici dikey anlatim kurulabilir ama urun ayni urun olarak kalmali.

Sadece gorsel uretime yonelik dusun.`, referenceSummary, company.Name, company.Sector, categoryHint, company.TargetAudience, company.ToneOfVoice, brandColors, preset.FormatLabel, preset.Platform, preset.Width, preset.Height, preset.TextPosition, preset.TextAlign, copy.Headline, copy.SubHeadline, copy.CTA, preset.DesignNotes, inferAdCreativeDirection(referenceSummary, preset))
}

func GenerateAdCopy(company models.Company, studio models.ProductImageStudio, preset AdStudioPreset) AdCopyPayload {
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		if payload, err := generateAdCopyWithOpenAI(apiKey, company, studio, preset); err == nil {
			return payload
		}
	}

	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		if payload, err := generateAdCopyWithGemini(apiKey, company, studio, preset); err == nil {
			return payload
		}
	}

	return fallbackAdCopy(company, studio, preset)
}

func fallbackAdCopy(company models.Company, studio models.ProductImageStudio, preset AdStudioPreset) AdCopyPayload {
	name := company.Name
	if name == "" {
		name = "Markaniz"
	}

	category := studio.Category
	if category == "" {
		category = "urun"
	}

	summary := strings.ToLower(studio.SourceAnalysis)
	isPetFood := strings.Contains(summary, "cat") || strings.Contains(summary, "pet") || strings.Contains(summary, "kitten")

	switch preset.Key {
	case "instagram_story":
		if isPetFood {
			return AdCopyPayload{
				Headline:    "Minik patilere ozel",
				SubHeadline: "Guven veren premium sunumla yeni nesil urun vitrini",
				CTA:         "Kesfet",
			}
		}
		return AdCopyPayload{
			Headline:    fmt.Sprintf("%s ile dikkat cekin", name),
			SubHeadline: fmt.Sprintf("%s kategorisinde yeni sezon hissi veren premium sunum", strings.ToLower(category)),
			CTA:         "Kesfet",
		}
	case "facebook_banner":
		if isPetFood {
			return AdCopyPayload{
				Headline:    "Patili dostlara ozel",
				SubHeadline: "Premium urun sunumuyla guven veren kampanya vitrini",
				CTA:         "Incele",
			}
		}
		return AdCopyPayload{
			Headline:    "Bugune ozel secili firsatlar",
			SubHeadline: "Daha temiz sunum, daha guclu ilk izlenim, daha yuksek tiklama potansiyeli",
			CTA:         "Teklifi Incele",
		}
	case "trendyol_banner":
		if isPetFood {
			return AdCopyPayload{
				Headline:    "Guven veren mama sunumu",
				SubHeadline: "Pazaryeri icin net, temiz ve premium urun vitrini",
				CTA:         "Simdi Gor",
			}
		}
		return AdCopyPayload{
			Headline:    "One cikan urun vitrininde yerinizi alin",
			SubHeadline: "Pazaryeri uyumlu banner kompozisyonu ile donusume odaklanin",
			CTA:         "Simdi Gor",
		}
	default:
		if isPetFood {
			return AdCopyPayload{
				Headline:    "Premium mama vitrini",
				SubHeadline: "Sosyal medya ve pazaryeri icin guven veren urun sunumu",
				CTA:         "Incele",
			}
		}
		return AdCopyPayload{
			Headline:    fmt.Sprintf("%s ile yeni kampanya dili", name),
			SubHeadline: "Urun odakli kreatif ile sosyal reklam performansini destekleyin",
			CTA:         "Incele",
		}
	}
}

func generateAdCopyWithOpenAI(apiKey string, company models.Company, studio models.ProductImageStudio, preset AdStudioPreset) (AdCopyPayload, error) {
	prompt := fmt.Sprintf(`You are a senior ecommerce copywriter.
Create ad copy for a product-based creative.

Brand: %s
Sector: %s
Category: %s
Audience: %s
Tone: %s
Format: %s
Platform: %s
Reference summary: %s

Rules:
- Write in Turkish
- headline max 6 words
- sub_headline max 12 words
- cta max 3 words
- must fit a visual ad creative
- must be clearly relevant to the actual product in the reference summary, not just the declared category
- if the product is pet food, packaged food, cosmetic, apparel, footwear, or another distinct retail type, the copy must match that exact retail type
- output only valid JSON with keys headline, sub_headline, cta`, company.Name, company.Sector, studio.Category, company.TargetAudience, company.ToneOfVoice, preset.FormatLabel, preset.Platform, studio.SourceAnalysis)

	reqBody := openAIRequest{
		Model:       "gpt-4o-mini",
		Temperature: 0.8,
		MaxTokens:   200,
		Messages: []openAIMessage{
			{Role: "system", Content: "You write concise high-converting ad copy and always return JSON."},
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
		return AdCopyPayload{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return AdCopyPayload{}, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(body))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return AdCopyPayload{}, err
	}
	if len(openAIResp.Choices) == 0 {
		return AdCopyPayload{}, fmt.Errorf("empty openai response")
	}

	content := cleanJSONResponse(openAIResp.Choices[0].Message.Content)
	var payload AdCopyPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return AdCopyPayload{}, err
	}
	return payload, nil
}

func generateAdCopyWithGemini(apiKey string, company models.Company, studio models.ProductImageStudio, preset AdStudioPreset) (AdCopyPayload, error) {
	prompt := fmt.Sprintf(`Write Turkish ad copy for a product creative.
Brand: %s
Sector: %s
Category: %s
Audience: %s
Tone: %s
Format: %s
Platform: %s
Reference: %s

Return JSON only:
{"headline":"","sub_headline":"","cta":""}

Rules:
- headline max 6 words
- sub_headline max 12 words
- cta max 3 words
- copy must match the actual product in the reference summary, not generic category assumptions`, company.Name, company.Sector, studio.Category, company.TargetAudience, company.ToneOfVoice, preset.FormatLabel, preset.Platform, studio.SourceAnalysis)

	reqBody := geminiRequest{
		Contents: []geminiContent{{Parts: []geminiPart{{Text: prompt}}}},
	}
	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", GeminiImageURL(apiKey), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return AdCopyPayload{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return AdCopyPayload{}, fmt.Errorf("gemini error %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return AdCopyPayload{}, err
	}
	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return AdCopyPayload{}, fmt.Errorf("empty gemini response")
	}

	content := cleanJSONResponse(geminiResp.Candidates[0].Content.Parts[0].Text)
	var payload AdCopyPayload
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return AdCopyPayload{}, err
	}
	return payload, nil
}
