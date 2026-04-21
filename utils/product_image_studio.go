package utils

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		return "Kategoriye sadik kal: premium evcil hayvan bakimi atmosferi, yumusak ve guven veren isik, temiz raf gercekligi, hafif pembe/krem premium parilti ve urunle uyumlu malzeme cue'lari kullan. Paket her zaman baskin kahraman olsun."
	case strings.Contains(summary, "packaged") || strings.Contains(summary, "package") || strings.Contains(summary, "bag") || strings.Contains(summary, "paket") || strings.Contains(summary, "ambalaj"):
		return "Yuklenen urunu paketli perakende urunu gibi ele al. Paket on yuzunu, basili marka alanini, fayda veya lezzet bolgesini ve genel paket siluetini aynen koru; sadece ticari cekiciligi artir."
	case strings.Contains(summary, "cosmetic") || strings.Contains(summary, "serum") || strings.Contains(summary, "bottle"):
		return "Premium guzellik estetine yaslan: temiz yansimalar, rafine isik, parlak malzeme gercekligi ve minimal ama luks bir sahne kullan; urunun formunu degistirme."
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
		return "Referans estetik: yuklenen paketin on yuzunu koru; paket uzerindeki kedi gorselleri, marka alani ve urun fayda bolgeleri bozulmasin."
	}

	return "Referans estetik: yuklenen urun sahnenin merkezinde kalsin, daha premium isik, daha temiz arka plan ve daha ticari kompozisyonla ayni urunun daha profesyonel bir versiyonu gibi gorunsun."
}

func DefaultProductStudioPresets() []ProductStudioPreset {
	return []ProductStudioPreset{
		{Key: "white_studio_a", Name: "Beyaz Studyo A", Platform: "Trendyol", BadgeTone: "success", BadgeLabel: "Trendyol", Background: "Beyaz Studyo", Width: 1200, Height: 1800, Score: 96, IsRecommended: true, DesignNotes: "Pazar yeri uyumlu temiz beyaz zemin, ana urun odagi yuksek."},
		{Key: "cream_neutral_b", Name: "Krem Notr B", Platform: "Hepsiburada", BadgeTone: "danger", BadgeLabel: "Hepsiburada", Background: "Krem Notr", Width: 1200, Height: 1200, Score: 91, DesignNotes: "Yumusak notr tonlarla kategoriye daha sicak bir raf hissi verir."},
		{Key: "dark_premium_c", Name: "Koyu Premium C", Platform: "Trendyol", BadgeTone: "success", BadgeLabel: "Trendyol", Background: "Koyu Premium", Width: 1200, Height: 1800, Score: 85, DesignNotes: "Premium algi ve kontrast isteyen urunlerde one cikar."},
		{Key: "instagram_soft", Name: "Instagram Soft", Platform: "Instagram", BadgeTone: "warning", BadgeLabel: "Instagram", Background: "Lavanta Soft", Width: 1080, Height: 1350, Score: 79, DesignNotes: "Sosyal medyada kaydirmayi durduracak daha renkli ve duygusal kurgu."},
		{Key: "web_fresh", Name: "Web Fresh", Platform: "Web", BadgeTone: "info", BadgeLabel: "Web", Background: "Acik Mavi", Width: 1600, Height: 1200, Score: 72, DesignNotes: "Landing page ve kategori kartlari icin ferah gorunum."},
		{Key: "instagram_night", Name: "Instagram Night", Platform: "Instagram", BadgeTone: "warning", BadgeLabel: "Instagram", Background: "Gece Vitrini", Width: 1080, Height: 1350, Score: 68, DesignNotes: "Deneysel ve yuksek kontrastli gece vitrini efekti."},
	}
}

// BuildProductStudioPrompt -- Detayli prompt (fallback generation icin).
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

AMAC:
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
- Kaynak odakli yaratici yon: %s
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

GORSEL YONLENDIRME:
- Beyaz/notr setuplarda yumusak studyo isigi, gercekci temas golgesi ve temiz katalog duzeni kullan.
- Premium koyu setuplarda kontrollu kontrast ve luks doku kullan ama urunu bozma.
- Sosyal varyantlarda daha zengin atmosfer kurulabilir ama urun yine ayni urun olarak kalmali.

Sadece gorsel uretime yonelik bir cevap dusun.`, company.Name, preset.Platform, referenceSummary, company.Name, company.Sector, categoryHint, company.TargetAudience, company.ToneOfVoice, brandColors, preset.Name, preset.Platform, preset.Width, preset.Height, preset.Background, preset.DesignNotes, inferSourceCreativeDirection(referenceSummary), inferPlatformCreativeRules(preset), inferVisualReferenceStyle(referenceSummary))
}

// BuildProductImageEditPrompt -- OpenAI gpt-image-1 image edit API icin
// ChatGPT tarzi promosyon/pazarlama gorseli olusturma prompti.
// Urun korunarak etrafina profesyonel e-ticaret ve sosyal medya tasarim ogeleri eklenir.
func BuildProductImageEditPrompt(studio models.ProductImageStudio, preset ProductStudioPreset, referenceSummary string) string {
	productHint := inferProductTypeHint(referenceSummary)
	sceneDirective := inferSceneDirective(preset)
	categoryCreativeHint := inferCategoryCreativeHint(referenceSummary)

	return fmt.Sprintf(`Bu gorseldeki urunu kullanarak %s platformu icin profesyonel bir e-ticaret / pazarlama gorseli olustur.

URUN: %s
Urunu oldugu gibi koru — ayni paket, ayni renkler, ayni marka yazilari, ayni form. Urunu yeniden cizme veya degistirme.

GOREV:
Bu urun gorselini, tum buyuk e-ticaret platformlarinda (Trendyol, Hepsiburada, Amazon) ve sosyal medyada (Instagram, Facebook) paylasima hazir, profesyonel bir tanitim gorselina donustur.

%s

TASARIM YONERGELERI:
- Urunu sahnenin ana kahramani olarak konumla, one cikar
- %s
- Urunle uyumlu, kategoriye ozgu dekoratif ve destekleyici ögeler ekle (ornegin: ilgili malzemeler, dogal ortam parcalari, lifestyle hissi)
- Profesyonel studyo isigi, yumusak golgelendirme ve derinlik hissi kullan
- Platform standartlarina uygun, satin alma istegi uyandiran bir kompozisyon kur
- Gerekirse kisa bir tanitici baslik veya slogan urune uygun sekilde yerlestirilebilir
- Gorsel, scroll sirasinda dikkati cekecek kadar cekici ve profesyonel olmali

KESINLIKLE YAPMA:
- Urunun kendisini degistirme, yeniden tasarlama veya farkli bir urun olusturma
- Urunun uzerindeki mevcut marka/logo/yazi/gorsel ogeleri bozma
- Urunun rengini, seklini veya oranlarini degistirme
- Alakasiz veya kategoriye uymayan nesneler ekleme

CIKTI: %dx%d piksel, profesyonel e-ticaret kalitesinde pazarlama gorseli`,
		preset.Platform,
		productHint,
		sceneDirective,
		categoryCreativeHint,
		preset.Width, preset.Height)
}

// inferSceneDirective -- preset ve platforma gore sahne yonergesi
func inferSceneDirective(preset ProductStudioPreset) string {
	bg := strings.ToLower(preset.Background)
	platform := strings.ToLower(preset.Platform)

	var scene string
	switch {
	case strings.Contains(bg, "beyaz") || strings.Contains(bg, "white"):
		scene = "SAHNE: Temiz beyaz studyo ortami. Profesyonel e-ticaret katalog cekimi gibi. Urun merkezde, temiz arka plan, profesyonel isik."
	case strings.Contains(bg, "krem") || strings.Contains(bg, "cream") || strings.Contains(bg, "notr") || strings.Contains(bg, "neutral"):
		scene = "SAHNE: Sicak, dogal tonlarda rafine bir ortam. Krem/bej arka plan, yumusak dogal isik, davetkar ve goven veren bir atmosfer."
	case strings.Contains(bg, "koyu") || strings.Contains(bg, "dark") || strings.Contains(bg, "premium"):
		scene = "SAHNE: Koyu/siyah premium arka plan. Luks marka hissi, kontrollu spot isik ve dramatik kontrast. Premium urun sunumu."
	case strings.Contains(bg, "lavanta") || strings.Contains(bg, "soft"):
		scene = "SAHNE: Yumusak pastel renkler, estetik ve paylasim istegi uyandiran Instagram tarzinda bir ortam. Renkli ama zarif."
	case strings.Contains(bg, "mavi") || strings.Contains(bg, "blue"):
		scene = "SAHNE: Ferah, modern acik mavi tonlarda bir ortam. Temiz, web odakli ve profesyonel landing page gorunumu."
	case strings.Contains(bg, "gece") || strings.Contains(bg, "night"):
		scene = "SAHNE: Koyu gece mavisi, dramatik spot isik efekti. Yuksek kontrast vitrin gorunumu, dikkat cekici ve cesur."
	default:
		scene = "SAHNE: Profesyonel studyo ortami, temiz ve modern."
	}

	// Platform bazli ek yonlendirme
	switch platform {
	case "trendyol", "hepsiburada":
		scene += " Pazaryeri icin optimize: urun net secilmeli, thumbnail'da bile taninabilir olmali, guven veren profesyonel kompozisyon."
	case "instagram":
		scene += " Sosyal medya icin optimize: scroll durdurucu, estetik, paylasim istegi yaratan, lifestyle hissi veren kompozisyon."
	case "web":
		scene += " Web sayfasi icin optimize: hero banner tarzinda, genis ve etkileyici bir sunum."
	}

	return scene
}

// inferCategoryCreativeHint -- urun kategorisine gore yaratici yonlendirme
func inferCategoryCreativeHint(referenceSummary string) string {
	summary := strings.ToLower(referenceSummary)
	switch {
	case strings.Contains(summary, "cat") || strings.Contains(summary, "kedi") || strings.Contains(summary, "mama") || strings.Contains(summary, "pet") || strings.Contains(summary, "kitten"):
		return "Evcil hayvan kategorisi: Yumusak, sicak ve guven veren bir atmosfer kur. Sevimli kedi/hayvan gorselleri, pati izi motifleri, dogal malzemeler veya taze et/balik gibi kategoriyle iliskili destekleyici ogeler eklenebilir. Veteriner onayi veya kalite hissi veren bir tasarim dili kullan."
	case strings.Contains(summary, "kopek") || strings.Contains(summary, "dog"):
		return "Evcil hayvan kategorisi: Enerjik, sicak ve guven veren bir atmosfer. Kopek gorselleri, pati izleri, dogal malzemeler gibi destekleyici ogeler eklenebilir."
	case strings.Contains(summary, "cosmetic") || strings.Contains(summary, "serum") || strings.Contains(summary, "bottle") || strings.Contains(summary, "kozmetik"):
		return "Kozmetik kategorisi: Luks, temiz ve rafine bir estetik. Su damlaciqlari, cicek yapraklari, dogal bitkisel ogeler, parlak yansimalar gibi premium guzellik sektorune ait destekleyici ogeler kullan."
	case strings.Contains(summary, "shoe") || strings.Contains(summary, "sneaker") || strings.Contains(summary, "ayakkabi"):
		return "Ayakkabi kategorisi: Dinamik, moda odakli bir sunum. Hareket hissi, sokak atmosferi veya premium vitrin gorunumu olustur."
	case strings.Contains(summary, "dress") || strings.Contains(summary, "giyim") || strings.Contains(summary, "jacket") || strings.Contains(summary, "fabric"):
		return "Giyim kategorisi: Editoryal moda cekimi tarzi. Kumas dokusunu ve silueti vurgula, zarif ve sik bir sunum."
	case strings.Contains(summary, "elektronik") || strings.Contains(summary, "electronic") || strings.Contains(summary, "tech"):
		return "Elektronik kategorisi: Modern, teknolojik ve premium bir sunum. Temiz cizgiler, futuristik isik efektleri ve minimalist tasarim."
	default:
		return "Urunle uyumlu, kategorisine ozgu profesyonel destekleyici ogeler ve dekoratif detaylar ekle. Urunun cekiciligini artiran ama ondan rol calmayan bir kompozisyon kur."
	}
}

// inferProductTypeHint -- referans ozetinden kisa urun tipi cikarir
func inferProductTypeHint(referenceSummary string) string {
	if referenceSummary == "" {
		return "Gorseldeki urun"
	}
	summary := strings.ToLower(referenceSummary)
	switch {
	case strings.Contains(summary, "kedi") || strings.Contains(summary, "cat") || strings.Contains(summary, "mama"):
		return "Kedi mamasi paketi"
	case strings.Contains(summary, "kopek") || strings.Contains(summary, "dog"):
		return "Kopek mamasi paketi"
	case strings.Contains(summary, "serum") || strings.Contains(summary, "cosmetic") || strings.Contains(summary, "kozmetik"):
		return "Kozmetik urun"
	case strings.Contains(summary, "shoe") || strings.Contains(summary, "sneaker") || strings.Contains(summary, "ayakkabi"):
		return "Ayakkabi"
	case strings.Contains(summary, "dress") || strings.Contains(summary, "giyim") || strings.Contains(summary, "jacket"):
		return "Giyim urunu"
	case strings.Contains(summary, "elektronik") || strings.Contains(summary, "electronic"):
		return "Elektronik urun"
	default:
		hint := referenceSummary
		if len(hint) > 80 {
			hint = hint[:80]
		}
		return hint
	}
}

func DescribeProductImageReference(imageURL, category string) string {
	var imageBytes []byte
	var err error

	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		// CDN URL — indir
		client := &http.Client{Timeout: 30 * time.Second}
		resp, dlErr := client.Get(imageURL)
		if dlErr != nil {
			return fallbackReferenceDescription(category)
		}
		defer resp.Body.Close()
		imageBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return fallbackReferenceDescription(category)
		}
	} else {
		// Lokal dosya yolu
		imagePath := "./assets" + imageURL
		imageBytes, err = os.ReadFile(imagePath)
		if err != nil {
			return fallbackReferenceDescription(category)
		}
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
		Model: GetAiModel("gpt_smart_general", "gpt-4o"),
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

func GenerateProductImageEdit(imageURL, prompt string, width, height int) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY bulunamadi")
	}

	var imageBytes []byte
	var imagePath string
	var err error

	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		// CDN URL — indir
		client := &http.Client{Timeout: 60 * time.Second}
		resp, dlErr := client.Get(imageURL)
		if dlErr != nil {
			return "", fmt.Errorf("kaynak gorsel indirilemedi: %v", dlErr)
		}
		defer resp.Body.Close()
		imageBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("kaynak gorsel okunamadi: %v", err)
		}
		imagePath = "source_image.png"
	} else {
		imagePath = "./assets" + imageURL
		imageBytes, err = os.ReadFile(imagePath)
		if err != nil {
			return "", fmt.Errorf("kaynak gorsel okunamadi: %v", err)
		}
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

	imgEditModel := GetAiModel("gpt_image_edit", "gpt-image-1")
	if err := writer.WriteField("model", imgEditModel); err != nil {
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

	// Dosyayi Bunny CDN veya assets/uploads/product-studio/ altina kaydet
	fileName := fmt.Sprintf("studio_%d.png", time.Now().UnixNano())

	if editResp.Data[0].B64JSON != "" {
		decoded, decErr := base64.StdEncoding.DecodeString(editResp.Data[0].B64JSON)
		if decErr != nil {
			decoded, decErr = base64.RawStdEncoding.DecodeString(editResp.Data[0].B64JSON)
			if decErr != nil {
				return "", fmt.Errorf("base64 decode basarisiz: %v", decErr)
			}
		}

		// Bunny CDN'e yukle
		if IsBunnyCDNConfigured() {
			cdnURL, err := UploadToBunnyCDN(decoded, "product-studio", fileName)
			if err != nil {
				log.Printf("⚠️ Bunny CDN upload basarisiz, lokale kaydediliyor: %v", err)
			} else {
				log.Printf("Product studio gorsel Bunny CDN'e kaydedildi: %s", cdnURL)
				return cdnURL, nil
			}
		}

		// Fallback: lokale kaydet
		uploadDir := "./assets/uploads/product-studio"
		if mkErr := os.MkdirAll(uploadDir, 0755); mkErr != nil {
			return "", fmt.Errorf("product-studio klasoru olusturulamadi: %v", mkErr)
		}
		outPath := filepath.Join(uploadDir, fileName)
		if wErr := os.WriteFile(outPath, decoded, 0644); wErr != nil {
			return "", fmt.Errorf("dosya yazma hatasi: %v", wErr)
		}
		publicURL := "/uploads/product-studio/" + fileName
		log.Printf("Product studio gorsel kaydedildi: %s", publicURL)
		return publicURL, nil
	}

	if editResp.Data[0].URL != "" {
		dlClient := &http.Client{Timeout: 60 * time.Second}
		dlResp, dlErr := dlClient.Get(editResp.Data[0].URL)
		if dlErr != nil {
			return "", fmt.Errorf("gorsel indirme hatasi: %v", dlErr)
		}
		defer dlResp.Body.Close()
		if dlResp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("gorsel indirme hata kodu: %d", dlResp.StatusCode)
		}
		dlBytes, rdErr := io.ReadAll(dlResp.Body)
		if rdErr != nil {
			return "", fmt.Errorf("gorsel okuma hatasi: %v", rdErr)
		}

		// Bunny CDN'e yukle
		if IsBunnyCDNConfigured() {
			cdnURL, err := UploadToBunnyCDN(dlBytes, "product-studio", fileName)
			if err != nil {
				log.Printf("⚠️ Bunny CDN upload basarisiz, lokale kaydediliyor: %v", err)
			} else {
				log.Printf("Product studio gorsel Bunny CDN'e kaydedildi: %s", cdnURL)
				return cdnURL, nil
			}
		}

		// Fallback: lokale kaydet
		uploadDir := "./assets/uploads/product-studio"
		os.MkdirAll(uploadDir, 0755)
		outPath := filepath.Join(uploadDir, fileName)
		if wErr := os.WriteFile(outPath, dlBytes, 0644); wErr != nil {
			return "", fmt.Errorf("dosya yazma hatasi: %v", wErr)
		}
		publicURL := "/uploads/product-studio/" + fileName
		log.Printf("Product studio gorsel kaydedildi: %s", publicURL)
		return publicURL, nil
	}

	return "", fmt.Errorf("openai image edit gorsel donmedi")
}

// GenerateProductImageWithSource -- Kaynak urun gorselini AI'ya gonderip
// yeni pazarlama/promosyon gorseli ureten fonksiyon.
// Oncelik: OpenAI gpt-image-1 (ChatGPT tarzi) → Gemini (son care)
func GenerateProductImageWithSource(imageURL, prompt string) (string, error) {
	var imageBytes []byte
	var imagePath string
	var err error

	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		// CDN URL — indir
		client := &http.Client{Timeout: 60 * time.Second}
		resp, dlErr := client.Get(imageURL)
		if dlErr != nil {
			return "", fmt.Errorf("kaynak gorsel indirilemedi: %v", dlErr)
		}
		defer resp.Body.Close()
		imageBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("kaynak gorsel okunamadi: %v", err)
		}
		imagePath = "source_image.png"
	} else {
		imagePath = "./assets" + imageURL
		imageBytes, err = os.ReadFile(imagePath)
		if err != nil {
			return "", fmt.Errorf("kaynak gorsel okunamadi: %v", err)
		}
	}	// 1) OpenAI gpt-image-1 — ChatGPT ile ayni motor, kaynak gorseli gonderir
	openaiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if openaiKey != "" {
		result, err := callOpenAIImageEditWithSource(openaiKey, imagePath, imageBytes, prompt)
		if err == nil {
			return result, nil
		}
		log.Printf("⚠️ OpenAI gpt-image-1 gorsel+prompt uretimi basarisiz: %v", err)
	}

	// 2) Gemini multimodal — son care fallback
	geminiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if geminiKey != "" {
		imageBase64 := base64.StdEncoding.EncodeToString(imageBytes)
		mimeType := detectMimeType(imagePath)
		result, err := callGeminiImageWithSource(geminiKey, imageBase64, mimeType, prompt)
		if err == nil {
			return result, nil
		}
		log.Printf("⚠️ Gemini gorsel+prompt uretimi basarisiz: %v", err)
	}

	return "", fmt.Errorf("kaynak gorsel ile uretim basarisiz — hicbir provider calismadi")
}

// detectMimeType -- dosya uzantisindan MIME type tespit eder
func detectMimeType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/png"
	}
}

// callOpenAIImageEditWithSource -- OpenAI gpt-image-1 /images/edits endpointine
// kaynak urun gorselini multipart form ile gonderir.
// Bu, ChatGPT'nin gorsel uretme motorunun aynisidir.
func callOpenAIImageEditWithSource(apiKey, imagePath string, imageBytes []byte, prompt string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	imgEditModel := GetAiModel("gpt_image_edit", "gpt-image-1")
	if err := writer.WriteField("model", imgEditModel); err != nil {
		return "", err
	}
	if err := writer.WriteField("prompt", prompt); err != nil {
		return "", err
	}
	if err := writer.WriteField("size", "1024x1024"); err != nil {
		return "", err
	}

	// Kaynak gorseli multipart form ile gonder
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

	client := &http.Client{Timeout: 180 * time.Second}
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
		return "", fmt.Errorf("openai bos yanit")
	}

	return saveProductStudioResult(editResp.Data[0].B64JSON, editResp.Data[0].URL, "OpenAI gpt-image-1")
}

// saveProductStudioResult -- b64 veya URL'den gorseli Bunny CDN'e veya diske kaydeder
func saveProductStudioResult(b64Data, imageURL, engine string) (string, error) {
	fileName := fmt.Sprintf("studio_%d.png", time.Now().UnixNano())

	var imageData []byte

	if b64Data != "" {
		decoded, err := base64.StdEncoding.DecodeString(b64Data)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(b64Data)
			if err != nil {
				return "", fmt.Errorf("base64 decode basarisiz: %v", err)
			}
		}
		imageData = decoded
	} else if imageURL != "" {
		dlClient := &http.Client{Timeout: 60 * time.Second}
		dlResp, err := dlClient.Get(imageURL)
		if err != nil {
			return "", fmt.Errorf("gorsel indirme hatasi: %v", err)
		}
		defer dlResp.Body.Close()
		dlBytes, _ := io.ReadAll(dlResp.Body)
		imageData = dlBytes
	} else {
		return "", fmt.Errorf("gorsel verisi yok")
	}

	// Bunny CDN'e yukle
	if IsBunnyCDNConfigured() {
		cdnURL, err := UploadToBunnyCDN(imageData, "product-studio", fileName)
		if err != nil {
			log.Printf("⚠️ Bunny CDN upload basarisiz, lokale kaydediliyor: %v", err)
		} else {
			log.Printf("✅ Gorsel uretildi (kaynak gorsel + prompt, motor: %s): %s", engine, cdnURL)
			return cdnURL, nil
		}
	}

	// Fallback: lokale kaydet
	uploadDir := "./assets/uploads/product-studio"
	os.MkdirAll(uploadDir, 0755)
	outPath := filepath.Join(uploadDir, fileName)
	if wErr := os.WriteFile(outPath, imageData, 0644); wErr != nil {
		return "", fmt.Errorf("dosya yazma hatasi: %v", wErr)
	}

	publicURL := "/uploads/product-studio/" + fileName
	log.Printf("✅ Gorsel uretildi (kaynak gorsel + prompt, motor: %s): %s", engine, publicURL)
	return publicURL, nil
}

// callGeminiImageWithSource -- Gemini multimodal API ile kaynak gorseli + prompt gonderir
// ve yeni gorsel uretir. Son care fallback olarak kullanilir.
func callGeminiImageWithSource(apiKey, imageBase64, mimeType, prompt string) (string, error) {
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

	// Gemini response parse
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

	// Yanittaki gorsel verisini bul
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

	return saveProductStudioResult(resultBase64, "", "Gemini")
}

