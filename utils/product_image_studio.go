package utils

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

type ProductStudioPreset struct {
	Key           string `json:"key"`
	Name          string `json:"name"`
	Platform      string `json:"platform"`
	BadgeTone     string `json:"badge_tone"`
	BadgeLabel    string `json:"badge_label"`
	Background    string `json:"background"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Score         int    `json:"score"`
	IsRecommended bool   `json:"is_recommended"`
	DesignNotes   string `json:"design_notes"`
}

func inferSourceCreativeDirection(referenceSummary string) string {
	summary := strings.ToLower(referenceSummary)

	switch {
	case strings.Contains(summary, "cat") || strings.Contains(summary, "pet") || strings.Contains(summary, "kitten") || strings.Contains(summary, "kedi") || strings.Contains(summary, "mama"):
		return "Kategoriye sadik kal: premium evcil hayvan bakimi atmosferi, yumusak ve guven veren isik, temiz raf gercekçiligi, hafif pembe/krem premium parilti ve urunle uyumlu malzeme cue'lari kullan. Paket her zaman baskin kahraman olsun."
	case strings.Contains(summary, "packaged") || strings.Contains(summary, "package") || strings.Contains(summary, "bag") || strings.Contains(summary, "paket") || strings.Contains(summary, "ambalaj"):
		return "Yuklenen urunu paketli perakende urunu gibi ele al. Paket on yuzunu, basili marka alanini, fayda veya lezzet bolgesini ve genel paket siluetini aynen koru; sadece ticari cekiciligi artir."
	case strings.Contains(summary, "cosmetic") || strings.Contains(summary, "serum") || strings.Contains(summary, "bottle"):
		return "Premium guzellik estetine yaslan: temiz yansimalar, rafine isik, parlak malzeme gercekçiligi ve minimal ama luks bir sahne kullan; urunun formunu degistirme."
	case strings.Contains(summary, "shoe") || strings.Contains(summary, "sneaker") || strings.Contains(summary, "footwear"):
		return "Moda odakli ama urune sadik bir kurgu kullan. Formu, materyal dokusunu ve taban profilini vurgula; urunun tipini kesinlikle degistirme."
	case strings.Contains(summary, "dress") || strings.Contains(summary, "jacket") || strings.Contains(summary, "garment") || strings.Contains(summary, "fabric"):
		return "Giyimde silueti ve kumas karakterini birebir koru. Sahneyi editoryal olarak iyilestir ama kesim, doku ve renk butunlugunu bozma."
	default:
		return "Ayni urunun etrafinda platforma uygun ticari bir iyilestirme yap. Kategoriyle tutarli, hafif ve ikincil destek ogeleri kullan; urunun tipini veya kimligini degistirme."
	}
}

func inferPlatformCreativeRules(preset ProductStudioPreset) string {
	switch strings.ToLower(preset.Platform) {
	case "trendyol", "hepsiburada":
		return "Pazaryeri once gelir: urunu merkezde veya hafif hero agirlikli tut, arka plani kontrollu kullan, prop'lari minimumda tut ve thumbnail kirpimlarina guvenli bosluk birak. Dramatik gosteri yerine netlik, guven ve taniyabilirlik oncelikli olsun."
	case "instagram":
		return "Sosyal medya once gelir: urun cok net taninabilir kalsin ama paylasim istegi yaratan daha estetik bir atmosfer, yumusak gradyanlar ve hafif lifestyle dokunuslar eklenebilir."
	case "web":
		return "Web odakli: gelecekte layout icin kullanilabilecek temiz bir hero alan, premium isik ve rafine landing page hissi kur."
	default:
		return "Urunu aninda taniyabilir kilan dengeli bir ecommerce kompozisyonu kur ve gorsel cekiciligi yukselt."
	}
}

func inferVisualReferenceStyle(referenceSummary string) string {
	summary := strings.ToLower(referenceSummary)

	if strings.Contains(summary, "cat") || strings.Contains(summary, "kitten") || strings.Contains(summary, "kedi") || strings.Contains(summary, "mama") {
		return "Referans estetik: yuklenen paketin on yuzunu koru; paket uzerindeki kedi gorselleri, marka alani ve urun fayda bolgeleri bozulmasin. Sonuc, yumusak premium pembe-krem isikla zenginlesmis, urunun daha parlatilmis ve daha profesyonel gorundugu, kategoriyle uyumlu icerik destekleri olan ticari bir gorsel gibi hissettirmeli."
	}

	return "Referans estetik: yuklenen urun sahnenin merkezinde kalsin, daha premium isik, daha temiz arka plan ve daha ticari kompozisyonla ayni urunun daha profesyonel bir versiyonu gibi gorunsun."
}

func DefaultProductStudioPresets() []ProductStudioPreset {
	return []ProductStudioPreset{
		{Key: "white_studio_a", Name: "Beyaz Stüdyo A", Platform: "Trendyol", BadgeTone: "success", BadgeLabel: "Trendyol", Background: "Beyaz Stüdyo", Width: 1200, Height: 1800, Score: 96, IsRecommended: true, DesignNotes: "Pazar yeri uyumlu temiz beyaz zemin, ana ürün odağı yüksek."},
		{Key: "cream_neutral_b", Name: "Krem Nötr B", Platform: "Hepsiburada", BadgeTone: "danger", BadgeLabel: "Hepsiburada", Background: "Krem Nötr", Width: 1200, Height: 1200, Score: 91, DesignNotes: "Yumuşak nötr tonlarla kategoriye daha sıcak bir raf hissi verir."},
		{Key: "dark_premium_c", Name: "Koyu Premium C", Platform: "Trendyol", BadgeTone: "success", BadgeLabel: "Trendyol", Background: "Koyu Premium", Width: 1200, Height: 1800, Score: 85, DesignNotes: "Premium algı ve kontrast isteyen ürünlerde öne çıkar."},
		{Key: "instagram_soft", Name: "Instagram Soft", Platform: "Instagram", BadgeTone: "warning", BadgeLabel: "Instagram", Background: "Lavanta Soft", Width: 1080, Height: 1350, Score: 79, DesignNotes: "Sosyal medyada kaydırmayı durduracak daha renkli ve duygusal kurgu."},
		{Key: "web_fresh", Name: "Web Fresh", Platform: "Web", BadgeTone: "info", BadgeLabel: "Web", Background: "Açık Mavi", Width: 1600, Height: 1200, Score: 72, DesignNotes: "Landing page ve kategori kartları için ferah görünüm."},
		{Key: "instagram_night", Name: "Instagram Night", Platform: "Instagram", BadgeTone: "warning", BadgeLabel: "Instagram", Background: "Gece Vitrini", Width: 1080, Height: 1350, Score: 68, DesignNotes: "Deneysel ve yüksek kontrastlı sosyal varyant."},
	}
}

func BuildProductStudioPrompt(company models.Company, studio models.ProductImageStudio, preset ProductStudioPreset, referenceSummary string) string {
	brandColors := company.BrandColors
	if brandColors == "" {
		brandColors = `["#F04E23","#111111","#F7F4EF"]`
	}

	categoryHint := studio.Category
	if categoryHint == "" {
		categoryHint = "Otomatik algilanan urun kategorisi"
	}

	return fmt.Sprintf(`Sen %s icin calisan kidemli bir e-ticaret art direktoru ve sosyal ticaret gorsel stratejistisin.

AMAÇ:
- Yuklenen gorseldeki urunun kendisini kullan.
- Urunun siluetini, oranlarini, ambalaj kimligini, on yuzunu ve taniyabilir urun tipini koru.
- Ayni urunu "%s" icin daha premium, daha profesyonel ve paylasilabilir bir platform gorseline donustur.
- Daginik arka plani temizle, ama yeni bir urun tasarlama.

TEMEL TALIMAT:
- Kaynak gercek sadece yuklenen gorseldir.
- Bu urunu kullanarak harika bir tasarimla revize et.
- Sonuc Hepsiburada, Trendyol ve sosyal medya hesaplarinda paylasilabilecek kadar guclu olmali.
- Asla farkli bir urun, farkli bir paket, farkli bir hayvan mamasi ya da farkli bir kategori uretme.
- Asla DIO ekleme, ekstra logo ekleme, yeni watermark ekleme.

KAYNAK URUN OZETI:
%s

MARKA / SATICI BAGLAMI:
- Marka: %s
- Sektor: %s
- Kategori: %s
- Hedef kitle: %s
- Iletisim tonu: %s
- Marka renkleri: %s

CIKTI OZELLIKLERI:
- Varyant adi: %s
- Platform: %s
- Tuval: %dx%d px
- Arka plan yonu: %s
- Tasarim notu: %s
- Kaynak odakli yaratıcı yon: %s
- Platform kurali: %s
- Referans estetik: %s

ZORUNLU KURALLAR:
- Bu bir yeni urun uretimi degil, yuklenen urunun gorsel revizesidir.
- Urun her zaman ana kahraman olsun.
- Urunun gorunen marka alani, paket yapisi, urun tipi ve genel kimligi korunmali.
- Paketli bir kedi mamasiysa sonuc yine ayni paketli kedi mamasi olmali.
- Urunun uzerindeki mevcut tasarim dili korunabilir ama disaridan yeni tasarim urun yaratma.
- DIO, baska logo, kampanya damgasi, rozet, watermark, UI, el, sticker veya alakasiz nesne ekleme.
- Gorseli daha iyi isik, daha temiz kompozisyon, daha iyi golge, daha premium doku ve daha profesyonel sahneleme ile guclendir.
- Destek ogeleri kullanilacaksa kategoriyle uyumlu olsun ve urunden rol calmaya calismasin.
- Pazaryerinde urun net secilmeli; sosyal medyada ise daha estetik ama yine urune sadik bir kompozisyon olmali.

GÖRSEL YÖNLENDİRME:
- Beyaz/nötr setuplarda yumusak studyo isigi, gercekci temas golgesi ve temiz katalog duzeni kullan.
- Premium koyu setuplarda kontrollu kontrast ve luks doku kullan ama urunu bozma.
- Sosyal varyantlarda daha zengin atmosfer kurulabilir ama urun yine ayni urun olarak kalmali.

Sadece gorsel uretime yonelik bir cevap dusun.`, company.Name, preset.Platform, referenceSummary, company.Name, company.Sector, categoryHint, company.TargetAudience, company.ToneOfVoice, brandColors, preset.Name, preset.Platform, preset.Width, preset.Height, preset.Background, preset.DesignNotes, inferSourceCreativeDirection(referenceSummary), inferPlatformCreativeRules(preset), inferVisualReferenceStyle(referenceSummary))
}

func DescribeProductImageReference(imageLocalURL, category string) string {
	imagePath := "." + imageLocalURL
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return fallbackReferenceDescription(category)
	}

	imageBase64 := base64.StdEncoding.EncodeToString(imageBytes)
	prompt := fmt.Sprintf(`Yuklenen urun fotografini bir gorsel uretim sistemi icin analiz et.

Amac:
- Gorseldeki urunu sadik sekilde tarif et ki ayni urun yeni sahnelerde korunabilsin.
- Kategori ipucu ile gorsel celisirse gorseli esas al.
- Paketli bir urunse ambalaj kimligini acikca belirt.
- Kedi mamasi, kopek mamasi, kozmetik, ayakkabi, giyim gibi net bir urun tipi varsa bunu dogru soyle.

Asagidaki noktalari kisaca anlat:
- urun tipi
- siluet ve form
- gorunen materyal ve doku
- baskin renkler
- ambalaj veya urun uzerindeki korunmasi gereken kimlik detaylari
- kamera acisi / kirpim
- temizlenmesi gereken arka plan sorunlari
- yeni gorsellerde korunmasi gereken ana ozellikler

Kategori ipucu: %s

Yanit sadece Turkce, tek paragraf, 70-140 kelime olsun.`, category)

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		if summary, err := describeReferenceWithOpenAI(apiKey, imageBase64, prompt); err == nil && summary != "" {
			return summary
		}
	}

	if apiKey := os.Getenv("GEMINI_API_KEY"); apiKey != "" {
		if summary, err := describeReferenceWithGemini(apiKey, imageBase64, prompt); err == nil && summary != "" {
			return summary
		}
	}

	return fallbackReferenceDescription(category)
}

func fallbackReferenceDescription(category string) string {
	category = strings.TrimSpace(category)
	if category == "" || strings.EqualFold(category, "otomatik algila") {
		return "Yuklenen gorselde gercek bir perakende urunu bulunuyor. Uretimde ayni urunun tipi, paket veya obje silueti, baskin renkleri, gorunen marka alani ve taniyabilir on yuz kimligi korunmali; sadece arka plan temizlenip daha premium bir e-ticaret ve sosyal medya sunumuna tasinmali."
	}
	return fmt.Sprintf("Yuklenen gorselde %s kategorisine ait gercek bir urun bulunuyor. Uretimde ayni urunun tipi, silueti, temel renk ailesi ve taniyabilir kimligi korunmali; sahne sadece daha temiz, daha premium ve paylasilabilir hale getirilmeli.", strings.ToLower(category))
}

func describeReferenceWithGemini(apiKey, imageBase64, prompt string) (string, error) {
	type inlineDataPart struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	}
	type gPart struct {
		Text       string          `json:"text,omitempty"`
		InlineData *inlineDataPart `json:"inlineData,omitempty"`
	}
	type gContent struct {
		Parts []gPart `json:"parts"`
	}
	type gReq struct {
		Contents []gContent `json:"contents"`
	}

	reqBody := gReq{
		Contents: []gContent{{
			Parts: []gPart{
				{Text: prompt},
				{InlineData: &inlineDataPart{MimeType: "image/png", Data: imageBase64}},
			},
		}},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", GeminiImageURL(apiKey), bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini vision %d", resp.StatusCode)
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return "", err
	}
	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty gemini vision response")
	}

	return strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text), nil
}

func describeReferenceWithOpenAI(apiKey, imageBase64, prompt string) (string, error) {
	dataURI := fmt.Sprintf("data:image/png;base64,%s", imageBase64)

	type visionContent struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL    string `json:"url"`
			Detail string `json:"detail"`
		} `json:"image_url,omitempty"`
	}
	type visionMessage struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	type visionRequest struct {
		Model       string          `json:"model"`
		Messages    []visionMessage `json:"messages"`
		MaxTokens   int             `json:"max_tokens"`
		Temperature float64         `json:"temperature"`
	}

	contentArray := []visionContent{
		{Type: "text", Text: prompt},
		{Type: "image_url", ImageURL: &struct {
			URL    string `json:"url"`
			Detail string `json:"detail"`
		}{URL: dataURI, Detail: "high"}},
	}
	contentJSON, _ := json.Marshal(contentArray)

	reqBody := visionRequest{
		Model: "gpt-4o",
		Messages: []visionMessage{
			{Role: "system", Content: json.RawMessage(`"Sen e-ticaret gorsel uretimi icin urun fotografi analiz ediyorsun. Yalnizca duz Turkce metinle cevap ver."`)},
			{Role: "user", Content: json.RawMessage(contentJSON)},
		},
		MaxTokens: 250, Temperature: 0.2,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai vision %d", resp.StatusCode)
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(body, &openAIResp); err != nil {
		return "", err
	}
	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("empty openai vision response")
	}

	return strings.TrimSpace(openAIResp.Choices[0].Message.Content), nil
}

func GenerateProductImageEdit(imageLocalURL, prompt string, width, height int) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY bulunamadi")
	}

	imagePath := "." + imageLocalURL
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("kaynak gorsel okunamadi: %v", err)
	}

	size := "1024x1024"
	switch {
	case height > width:
		size = "1024x1536"
	case width > height:
		size = "1536x1024"
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("model", "gpt-image-1"); err != nil {
		return "", err
	}
	if err := writer.WriteField("prompt", prompt); err != nil {
		return "", err
	}
	if err := writer.WriteField("size", size); err != nil {
		return "", err
	}

	part, err := writer.CreateFormFile("image", filepath.Base(imagePath))
	if err != nil {
		return "", err
	}
	if _, err := part.Write(imageBytes); err != nil {
		return "", err
	}

	if err := writer.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/images/edits", &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai image edit %d: %s", resp.StatusCode, string(respBody))
	}

	var editResp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &editResp); err != nil {
		return "", err
	}
	if len(editResp.Data) == 0 {
		return "", fmt.Errorf("openai image edit bos yanit")
	}

	if editResp.Data[0].B64JSON != "" {
		decoded, err := base64DecodeImage(editResp.Data[0].B64JSON)
		if err != nil {
			return "", err
		}
		return saveImageBytes(decoded)
	}
	if editResp.Data[0].URL != "" {
		return downloadAndSaveImage(editResp.Data[0].URL)
	}

	return "", fmt.Errorf("openai image edit gorsel donmedi")
}
