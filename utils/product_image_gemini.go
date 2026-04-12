package utils

import (
	"bytes"
	"encoding/base64"
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

// GenerateNewProductImageWithGemini — Gemini 2.5 Flash Image modeli kullanarak
// kullanıcının yüklediği referans görsele göre yeni bir ürün görseli üretir.
// Opsiyonel olarak marka bilgileri (company) gönderilerek markaya uygun görsel üretilir.
func GenerateNewProductImageWithGemini(imageURL string, company *models.Company, category string, aspectRatio string) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY bulunamadi")
	}

	// Görseli oku
	imageBytes, mimeType, err := loadImageBytes(imageURL)
	if err != nil {
		return "", fmt.Errorf("referans gorsel yuklenemedi: %v", err)
	}

	imageBase64 := base64.StdEncoding.EncodeToString(imageBytes)

	// Prompt oluştur
	prompt := buildGeminiNewProductPrompt(company, category, aspectRatio)

	log.Printf("🎨 Gemini ile yeni ürün görseli üretiliyor — Marka: %s, Kategori: %s, Oran: %s",
		companyName(company), category, aspectRatio)

	// Gemini API isteği — görsel + metin → görsel çıktı
	type inlineData struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	}
	type part struct {
		Text       string      `json:"text,omitempty"`
		InlineData *inlineData `json:"inlineData,omitempty"`
	}
	type content struct {
		Parts []part `json:"parts"`
	}
	type genConfig struct {
		ResponseModalities []string `json:"responseModalities"`
	}
	type request struct {
		Contents         []content `json:"contents"`
		GenerationConfig genConfig `json:"generationConfig"`
	}

	reqBody := request{
		Contents: []content{{
			Parts: []part{
				{InlineData: &inlineData{MimeType: mimeType, Data: imageBase64}},
				{Text: prompt},
			},
		}},
		GenerationConfig: genConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("request marshal hatasi: %v", err)
	}

	url := GeminiImageURL(apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("request olusturma hatasi: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini API istegi basarisiz: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API hata %d: %s", resp.StatusCode, string(respBody))
	}

	// Yanıtı parse et
	type geminiPart struct {
		Text       string `json:"text,omitempty"`
		InlineData *struct {
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		} `json:"inlineData,omitempty"`
	}
	type geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []geminiPart `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	var gResp geminiResp
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return "", fmt.Errorf("response parse hatasi: %v", err)
	}

	if gResp.Error != nil {
		return "", fmt.Errorf("gemini hatasi: %s", gResp.Error.Message)
	}

	if len(gResp.Candidates) == 0 {
		return "", fmt.Errorf("gemini bos yanit dondu")
	}

	// Yanıttaki görsel verisini bul
	var resultBase64 string
	for _, p := range gResp.Candidates[0].Content.Parts {
		if p.InlineData != nil && p.InlineData.Data != "" {
			resultBase64 = p.InlineData.Data
			break
		}
	}

	if resultBase64 == "" {
		return "", fmt.Errorf("gemini yanitinda gorsel verisi bulunamadi")
	}

	log.Printf("✅ Gemini ile yeni ürün görseli başarıyla üretildi — Marka: %s", companyName(company))
	return saveProductStudioResult(resultBase64, "", "Gemini-2.5-Flash-Image")
}

// buildGeminiNewProductPrompt — Markaya ve kategoriye uygun yeni ürün görseli üretme prompt'u
func buildGeminiNewProductPrompt(company *models.Company, category string, aspectRatio string) string {
	var sb strings.Builder

	sb.WriteString(`Bu gorseldeki urunu referans alarak, ayni urunun profesyonel bir e-ticaret cekimi gibi gorunen YENI bir versiyonunu olustur.

TEMEL KURALLAR:
- Gorseldeki urunu birebir koru — ayni paket, ayni renkler, ayni form, ayni marka yazilari
- Urunu kesinlikle degistirme, yeniden tasarlama veya farkli bir urun olusturma
- Urunun uzerindeki mevcut logo, yazi, gorsel ogeleri bozma
- Sonuc, profesyonel bir studyo cekimi kalitesinde olmali

GORSEL YONERGELERI:
- Urun merkezde, net ve keskin
- Profesyonel studyo isigi — yumusak, difuze isik ile urun detaylari vurgulanmali
- Temiz, daginiklik olmayan bir arka plan
- Urunun altinda gercekci, yumusak bir temas golgesi
- E-ticaret platformlarinda (Trendyol, Hepsiburada, Amazon) kullanilabilir kalite
- Thumbnail'da bile urun net taninabilir olmali

`)

	// Marka bilgileri varsa ekle
	if company != nil {
		sb.WriteString("MARKA BILGILERI:\n")
		sb.WriteString(fmt.Sprintf("- Marka Adi: %s\n", company.Name))

		if company.Sector != "" {
			sb.WriteString(fmt.Sprintf("- Sektor: %s\n", company.Sector))
		}
		if company.BrandColors != "" {
			sb.WriteString(fmt.Sprintf("- Marka Renkleri: %s\n", company.BrandColors))
			sb.WriteString("- Arka planda ve destekleyici ogelerde marka renklerini kullan\n")
		}
		if company.TargetAudience != "" {
			sb.WriteString(fmt.Sprintf("- Hedef Kitle: %s\n", company.TargetAudience))
		}
		if company.ToneOfVoice != "" {
			sb.WriteString(fmt.Sprintf("- Iletisim Tonu: %s\n", company.ToneOfVoice))
		}
		if company.Slogan != "" {
			sb.WriteString(fmt.Sprintf("- Slogan: %s\n", company.Slogan))
		}
		if company.Description != "" {
			desc := company.Description
			if len(desc) > 200 {
				desc = desc[:200]
			}
			sb.WriteString(fmt.Sprintf("- Aciklama: %s\n", desc))
		}

		sb.WriteString("\nMARKA UYUMU:\n")
		sb.WriteString("- Gorselin genel estetigi marka kimligine uygun olmali\n")
		sb.WriteString("- Marka renkleri arka plan, destek ogeleri ve isik tonlarinda yansitilmali\n")
		sb.WriteString("- Premium ve profesyonel bir marka algisi olusturmali\n")
		sb.WriteString("- Markanin hedef kitlesine hitap eden bir gorsel dili kullanmali\n")
	}

	// Kategori bilgisi
	if category != "" && !strings.EqualFold(category, "Otomatik Algıla") && !strings.EqualFold(category, "Otomatik Algila") {
		sb.WriteString(fmt.Sprintf("\nURUN KATEGORISI: %s\n", category))
		sb.WriteString(inferCategoryStyleHint(category))
	}

	// Aspect ratio
	if aspectRatio != "" {
		sb.WriteString(fmt.Sprintf("\nGORSEL ORANI: %s\n", aspectRatio))
	}

	sb.WriteString(`
KESINLIKLE YAPMA:
- Urunun kendisini degistirme veya farkli bir urun olusturma
- Logo, watermark, damga veya yazi ekleme
- Urunun rengini, seklini veya oranlarini degistirme
- Alakasiz nesneler veya insanlar ekleme

Sadece gorsel uret, metin yaniti verme.`)

	return sb.String()
}

// inferCategoryStyleHint — kategoriye gore stil ipucu
func inferCategoryStyleHint(category string) string {
	cat := strings.ToLower(category)
	switch {
	case strings.Contains(cat, "giyim") || strings.Contains(cat, "moda"):
		return "- Moda odakli, editoryal bir sunum. Kumas dokusunu ve silueti vurgula.\n"
	case strings.Contains(cat, "ayakkab"):
		return "- Dinamik aci ile ayakkabiyi one cikar. Materyal ve taban detaylari net gorunsun.\n"
	case strings.Contains(cat, "evcil") || strings.Contains(cat, "hayvan"):
		return "- Sicak ve guven veren atmosfer. Pastel tonlar ve yumusak isik kullan.\n"
	case strings.Contains(cat, "kozmetik"):
		return "- Luks ve rafine estetik. Parlak yansimalar ve temiz cizgiler.\n"
	case strings.Contains(cat, "elektronik"):
		return "- Modern ve teknolojik sunum. Minimalist arka plan, keskin detaylar.\n"
	case strings.Contains(cat, "dekorasyon") || strings.Contains(cat, "ev"):
		return "- Sicak ev ortami hissi. Dogal isik ve rahatlatici tonlar.\n"
	default:
		return "- Urune uygun profesyonel studyo ortami.\n"
	}
}

// loadImageBytes — URL veya lokal yoldan görsel byte'larını yükler
func loadImageBytes(imageURL string) ([]byte, string, error) {
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		client := &http.Client{Timeout: 60 * time.Second}
		resp, err := client.Get(imageURL)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", err
		}
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "image/png"
		}
		return data, contentType, nil
	}

	// Lokal dosya
	path := "./assets" + imageURL
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	mimeType := detectMimeType(path)
	return data, mimeType, nil
}

// companyName — nil-safe marka adı
func companyName(c *models.Company) string {
	if c == nil {
		return "Genel"
	}
	return c.Name
}

// CreationModeSettings — Frontend'ten gelen tüm canvas ayarları
type CreationModeSettings struct {
	ScenePrompt  string   `json:"scene_prompt"`
	BgType       string   `json:"bg_type"`       // solid, gradient, texture, scene
	BgColor      string   `json:"bg_color"`       // #HEX
	GradColor1   string   `json:"grad_color1"`
	GradColor2   string   `json:"grad_color2"`
	GradDir      string   `json:"grad_direction"`
	BgOpacity    int      `json:"bg_opacity"`
	AspectRatio  string   `json:"aspect_ratio"`   // 1:1, 4:5, 9:16, 16:9, TY, HB, IG
	Category     string   `json:"category"`
	TextLayers   []TextLayer `json:"text_layers"`
	EffectShadow bool     `json:"effect_shadow"`
	EffectReflection bool `json:"effect_reflection"`
	EffectHighlight  bool `json:"effect_highlight"`
	EffectBgBlur     bool `json:"effect_bg_blur"`
	ShadowIntensity  int  `json:"shadow_intensity"`
	Platforms    []string `json:"platforms"`
	UseBrandLogo bool     `json:"use_brand_logo"`
}

// TextLayer — Canvas'taki metin katmanı
type TextLayer struct {
	Text      string `json:"text"`
	FontStyle string `json:"font_style"`
	FontSize  int    `json:"font_size"`
	Color     string `json:"color"`
	Position  string `json:"position"` // top-left, bottom-right, center, bottom-center
}

// GenerateProductImageFromPrompt — Kaynak görsel OLMADAN, text-to-image ile
// sıfırdan ürün görseli üretir. Tüm canvas ayarlarını prompt'a çevirerek Gemini'ye gönderir.
// Opsiyonel olarak marka logosu da inline image olarak gönderilebilir.
func GenerateProductImageFromPrompt(settings CreationModeSettings, company *models.Company) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY bulunamadi")
	}

	prompt := buildCreationModePrompt(settings, company)

	log.Printf("🎨 AI text-to-image — Marka: %s, Logo: %v, Prompt kısalt: %.80s...",
		companyName(company), settings.UseBrandLogo, settings.ScenePrompt)

	type inlineData struct {
		MimeType string `json:"mimeType"`
		Data     string `json:"data"`
	}
	type part struct {
		Text       string      `json:"text,omitempty"`
		InlineData *inlineData `json:"inlineData,omitempty"`
	}
	type content struct {
		Parts []part `json:"parts"`
	}
	type genConfig struct {
		ResponseModalities []string `json:"responseModalities"`
	}
	type request struct {
		Contents         []content `json:"contents"`
		GenerationConfig genConfig `json:"generationConfig"`
	}

	// Part listesi oluştur
	parts := []part{{Text: prompt}}

	// Marka logosu varsa ve kullanıcı istiyorsa → inline image olarak ekle
	if settings.UseBrandLogo && company != nil && company.LogoURL != "" {
		logoBytes, logoMime, err := loadImageBytes(company.LogoURL)
		if err == nil {
			logoB64 := base64.StdEncoding.EncodeToString(logoBytes)
			parts = append(parts, part{
				InlineData: &inlineData{MimeType: logoMime, Data: logoB64},
			})
			parts = append(parts, part{
				Text: "Bu gorselde yukaridaki marka logosunu da estetik ve uygun bir sekilde yerlestir. Logoyu gorselin bir kosesine veya altina kucuk ve profesyonel sekilde ekle.",
			})
			log.Printf("🏷️ Marka logosu eklendi: %s", company.LogoURL)
		} else {
			log.Printf("⚠️ Marka logosu yüklenemedi, atlanıyor: %v", err)
		}
	}

	reqBody := request{
		Contents: []content{{
			Parts: parts,
		}},
		GenerationConfig: genConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("request marshal hatasi: %v", err)
	}

	url := GeminiImageURL(apiKey)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("request olusturma hatasi: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini API istegi basarisiz: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API hata %d: %s", resp.StatusCode, string(respBody))
	}

	type geminiPart struct {
		Text       string `json:"text,omitempty"`
		InlineData *struct {
			MimeType string `json:"mimeType"`
			Data     string `json:"data"`
		} `json:"inlineData,omitempty"`
	}
	type geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []geminiPart `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	var gResp geminiResp
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return "", fmt.Errorf("response parse hatasi: %v", err)
	}

	if gResp.Error != nil {
		return "", fmt.Errorf("gemini hatasi: %s", gResp.Error.Message)
	}

	if len(gResp.Candidates) == 0 {
		return "", fmt.Errorf("gemini bos yanit dondu")
	}

	var resultBase64 string
	for _, p := range gResp.Candidates[0].Content.Parts {
		if p.InlineData != nil && p.InlineData.Data != "" {
			resultBase64 = p.InlineData.Data
			break
		}
	}

	if resultBase64 == "" {
		return "", fmt.Errorf("gemini yanitinda gorsel verisi bulunamadi")
	}

	log.Printf("✅ Gemini text-to-image başarılı — Marka: %s", companyName(company))
	return saveProductStudioResult(resultBase64, "", "Gemini-2.5-Flash-Image")
}

// buildCreationModePrompt — Canvas ayarlarının tamamını prompt'a çevirir
func buildCreationModePrompt(s CreationModeSettings, company *models.Company) string {
	var sb strings.Builder

	sb.WriteString("Profesyonel bir e-ticaret urun gorseli olustur.\n\n")

	// Kullanıcının yazdığı sahne açıklaması — EN ÖNEMLİ KISIM
	if s.ScenePrompt != "" {
		sb.WriteString(fmt.Sprintf("KULLANICININ ISTEGI:\n%s\n\n", s.ScenePrompt))
	}

	// Arka plan ayarları
	sb.WriteString("ARKA PLAN AYARLARI:\n")
	switch s.BgType {
	case "solid":
		if s.BgColor != "" {
			sb.WriteString(fmt.Sprintf("- Duz renk arka plan: %s\n", s.BgColor))
		} else {
			sb.WriteString("- Beyaz duz arka plan\n")
		}
	case "gradient":
		sb.WriteString(fmt.Sprintf("- Gradient arka plan: %s → %s yonunde %s\n", s.GradColor1, s.GradColor2, s.GradDir))
	case "texture":
		sb.WriteString("- Dokulu arka plan (subtle texture)\n")
	case "scene":
		sb.WriteString("- Yasam sahnesi (lifestyle) arka plan — urunun dogal kullanim ortaminda gosterilmesi\n")
	default:
		sb.WriteString("- Temiz, profesyonel beyaz arka plan\n")
	}

	if s.BgOpacity > 0 && s.BgOpacity < 100 {
		sb.WriteString(fmt.Sprintf("- Arka plan opaklik: %%%d\n", s.BgOpacity))
	}

	// Metin katmanları
	if len(s.TextLayers) > 0 {
		sb.WriteString("\nMETIN KATMANLARI:\n")
		for i, t := range s.TextLayers {
			pos := t.Position
			if pos == "" { pos = "center" }
			sb.WriteString(fmt.Sprintf("- Metin %d: \"%s\" — konum: %s, font: %s, boyut: %dpx, renk: %s\n",
				i+1, t.Text, pos, t.FontStyle, t.FontSize, t.Color))
		}
		sb.WriteString("- Bu metinleri gorselin uzerine estetik ve okunabilir sekilde yerlestir\n")
	}

	// Efektler
	var effects []string
	if s.EffectShadow {
		effects = append(effects, fmt.Sprintf("yumusak golge (yogunluk: %%%d)", s.ShadowIntensity))
	}
	if s.EffectReflection {
		effects = append(effects, "yansima efekti")
	}
	if s.EffectHighlight {
		effects = append(effects, "isik vurgulari (highlight)")
	}
	if s.EffectBgBlur {
		effects = append(effects, "arka plan bulaniklik (blur)")
	}
	if len(effects) > 0 {
		sb.WriteString(fmt.Sprintf("\nEFEKTLER: %s\n", strings.Join(effects, ", ")))
	}

	// Marka bilgileri
	if company != nil {
		sb.WriteString("\nMARKA BILGILERI:\n")
		sb.WriteString(fmt.Sprintf("- Marka: %s\n", company.Name))
		if company.Sector != "" {
			sb.WriteString(fmt.Sprintf("- Sektor: %s\n", company.Sector))
		}
		if company.BrandColors != "" {
			sb.WriteString(fmt.Sprintf("- Marka Renkleri: %s — arka plan ve destek ogelerinde kullan\n", company.BrandColors))
		}
		if company.ToneOfVoice != "" {
			sb.WriteString(fmt.Sprintf("- Iletisim Tonu: %s\n", company.ToneOfVoice))
		}
		if company.Slogan != "" {
			sb.WriteString(fmt.Sprintf("- Slogan: %s\n", company.Slogan))
		}
		if company.Description != "" {
			desc := company.Description
			if len(desc) > 150 { desc = desc[:150] }
			sb.WriteString(fmt.Sprintf("- Marka Aciklamasi: %s\n", desc))
		}
	}

	// Kategori
	if s.Category != "" && !strings.EqualFold(s.Category, "Otomatik Algila") {
		sb.WriteString(fmt.Sprintf("\nURUN KATEGORISI: %s\n", s.Category))
		sb.WriteString(inferCategoryStyleHint(s.Category))
	}

	// Aspect ratio
	if s.AspectRatio != "" {
		sb.WriteString(fmt.Sprintf("\nGORSEL ORANI: %s\n", s.AspectRatio))
	}

	// Platform bilgisi
	if len(s.Platforms) > 0 {
		sb.WriteString(fmt.Sprintf("\nHEDEF PLATFORMLAR: %s\n", strings.Join(s.Platforms, ", ")))
		sb.WriteString("- Bu platformlarin gorsel standartlarina uygun uret\n")
	}

	sb.WriteString(`
GORSEL KAPSAMLI KURALLAR:
- Urun merkezde, net, keskin ve profesyonel gorunumlu olmali
- E-ticaret platformlarinda (Trendyol, Hepsiburada, Amazon) kullanilabilir kalitede
- Studyo isigi ile profesyonel fotograf cekimi hissi ver
- Thumbnail boyutunda bile urun net taninabilir olmali
- Logo, watermark veya damga EKLEME
- Goruntunun tamami urun gorseli olmali, bos alan minimumda tutulmali

Sadece gorsel uret, metin yaniti verme.`)

	return sb.String()
}
