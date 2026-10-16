package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ──────────────────────────────────────────────
// Kısa video üretimi: senaryo → sahne görselleri → Türkçe seslendirme → ffmpeg montaj
// ──────────────────────────────────────────────

// VideoScene — senaryonun bir sahnesi
type VideoScene struct {
	Narration   string  `json:"narration"`       // seslendirme metni
	Caption     string  `json:"caption"`         // ekran yazısı (kısa)
	ImagePrompt string  `json:"image_prompt"`    // İngilizce görsel tarifi
	Start       float64 `json:"start,omitempty"` // videodaki başlangıç saniyesi (montajdan sonra)
	End         float64 `json:"end,omitempty"`
}

// VideoPlatform — mecraya göre format, kurgu ve paylaşım kuralları
type VideoPlatform struct {
	Label   string
	Format  string // 9:16 | 16:9
	MaxSec  int
	Guide   string // senaryo kurgusu
	CTA     string
	CopyFmt string // paylaşım metni kuralları
}

var VideoPlatforms = map[string]VideoPlatform{
	"instagram": {"Instagram Reels", "9:16", 90,
		"Instagram Reels: ilk 1,5 saniyede görsel ve sözel kanca; estetik, akıcı geçişler; her sahne tek mesaj; ses kapalı izleyenler için ekran yazısı tek başına anlaşılır olmalı.",
		"kaydet, paylaş veya bize mesaj at",
		"Instagram açıklaması: ilk satır kanca, 2-3 kısa paragraf, tek net CTA; sonunda 6-10 niş hashtag."},
	"tiktok": {"TikTok", "9:16", 180,
		"TikTok: ilk 1 saniyede çok güçlü, merak uyandıran kanca; samimi, konuşur gibi, doğal ve hızlı tempo; reklam gibi değil içerik üreticisi gibi; sahneler kısa ve dinamik.",
		"takip et veya yorumlara yaz",
		"TikTok açıklaması: tek-iki kısa cümle, merak uyandıran; sonunda 3-5 hashtag (biri geniş, diğerleri niş)."},
	"youtube_shorts": {"YouTube Shorts", "9:16", 60,
		"YouTube Shorts: ilk 2 saniyede net bir değer vaadi; bilgi yoğun ama hızlı; döngüye girecek şekilde son cümle başa bağlansın.",
		"abone ol",
		"YouTube Shorts: başlık en fazla 60 karakter (merak + anahtar kelime) ve 1-2 cümlelik açıklama; sonunda #shorts dahil 3 hashtag."},
	"youtube": {"YouTube (yatay)", "16:9", 180,
		"YouTube yatay video: ilk 5 saniyede videonun ne kazandıracağını söyle; daha açıklayıcı ve sakin anlatım; sahneler mantıklı bölümler halinde ilerlesin.",
		"abone ol ve bildirimleri aç",
		"YouTube: başlık en fazla 70 karakter (anahtar kelime başta), 3-5 cümlelik açıklama; sonunda 3-5 hashtag."},
}

func videoPlatform(code string) (string, VideoPlatform) {
	if p, ok := VideoPlatforms[code]; ok {
		return code, p
	}
	return "instagram", VideoPlatforms["instagram"]
}

// VideoRequest — üretim parametreleri
type VideoRequest struct {
	Brief          string
	CompanyContext string
	BrandName      string
	Sector         string
	BrandColors    string
	DurationSec    int
	Format         string // 9:16 | 1:1
	Voice          string // kadin | erkek | yok
	Pronunciations string // isteğe bağlı "İNKA=İnka" satırları (virgül veya satır ile ayrılmış)
	Language       string // tr, en, de, fr, es, ar, ru, az, nl, it (boşsa tr)
	Platform       string // instagram | tiktok | youtube_shorts | youtube
	Subtitles      bool   // ekran yazıları
	Intro          bool   // logolu marka girişi + köşe logosu
	LogoURL        string
}

// VideoLanguages — desteklenen video dilleri (ad: senaryo ve seslendirme talimatı için)
var VideoLanguages = map[string]string{
	"tr": "Türkçe", "en": "English", "de": "Deutsch", "fr": "Français", "es": "Español",
	"ar": "العربية", "ru": "Русский", "az": "Azərbaycan dili", "nl": "Nederlands", "it": "Italiano",
}

func videoLang(code string) (string, string) {
	if name, ok := VideoLanguages[code]; ok {
		return code, name
	}
	return "tr", "Türkçe"
}

// ProgressFunc — aşama ve yüzde bildirimi
type ProgressFunc func(stage string, progress int)

const videoFont = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"

// VideoToolsAvailable — sunucuda ffmpeg ve font var mı
func VideoToolsAvailable() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg kurulu değil")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return fmt.Errorf("ffprobe kurulu değil")
	}
	if _, err := os.Stat(videoFont); err != nil {
		return fmt.Errorf("video fontu bulunamadı (%s)", videoFont)
	}
	return nil
}

// GenerateVideoScript — süreye göre sahnelere bölünmüş senaryo (~5 sn/sahne, doğal Türkçe seslendirme ~1.9 kelime/sn)
func GenerateVideoScript(req VideoRequest) ([]VideoScene, error) {
	scenes := int(math.Round(float64(req.DurationSec) / 5))
	if scenes < 3 {
		scenes = 3
	}
	if scenes > 36 {
		scenes = 36
	}
	words := int(float64(req.DurationSec) * 1.9)
	narrationRule := fmt.Sprintf("Seslendirme toplamı yaklaşık %d kelime olsun (sahne başına %d-%d kelime).", words, words/scenes-2, words/scenes+3)
	if req.Voice == "yok" {
		narrationRule = "Seslendirme yok; narration alanını kısa tut (sahne başına en fazla 8 kelime), asıl mesaj caption'da."
	}

	langCode, langName := videoLang(req.Language)
	_, plat := videoPlatform(req.Platform)
	orientation := "dikey (9:16)"
	if plat.Format == "16:9" {
		orientation = "yatay (16:9)"
	}
	langRule := "- Doğal, akıcı Türkçe; doğru Türkçe karakterler. Uydurma rakam, müşteri adı veya yorum YOK.\n" +
		"- narration seslendirilecek: kısaltma KULLANMA, açık yaz (İK → insan kaynakları, KDV → katma değer vergisi); \"DM'den yazın\" yerine \"bize mesaj atın\" gibi konuşma dilini kullan; marka adını tamamı büyük harfle değil normal yazımla yaz (İNKA → İnka). caption'da kısaltma kullanılabilir."
	if langCode != "tr" {
		langRule = fmt.Sprintf("- narration ve caption TAMAMEN %s dilinde, o dili ana dili olarak konuşan birinin doğal günlük ifadeleriyle yazılsın (çeviri kokmasın). Uydurma rakam, müşteri adı veya yorum YOK.\n- narration'da kısaltma kullanma, açık yaz; marka adını normal yazımla yaz.", langName)
	}
	prompt := fmt.Sprintf(`Sen dünyanın en iyi kısa video (Reels/TikTok) senaristisin. Aşağıdaki içerik için TAM OLARAK %d sahnelik, toplam yaklaşık %d saniyelik bir video senaryosu yaz. Video dili: %s.

İÇERİK / BRIEF: %s
MARKA BAĞLAMI: %s

MECRA: %s
YAPI (mecraya ve süreye göre içeriği yeniden kurgula; kaynak metni olduğu gibi okuma, özünü al):
- 1. sahne: kanca (soru, şaşırtıcı bilgi, cesur iddia).
- Ortadaki sahneler: her biri tek somut fikir; akıcı, birbirine bağlanan anlatım; süreye sığmayan detayları çıkar.
- Son sahne: net harekete çağrı (%s) ve marka adı.
- %s
- caption: ekranda görünecek kısa yazı, en fazla 6 kelime, emojisiz.
- image_prompt: o sahne için İngilizce, gerçekçi, %s kompozisyonlu görsel tarifi; görselde YAZI olmasın; sahneler görsel olarak tutarlı bir hikâye anlatsın.
%s
- narration gerçek bir insanın konuşması gibi olsun: samimi, günlük konuşma dili, kısa cümleler; reklam klişesi ve abartılı ünlem yok.

Sadece geçerli JSON dizisi döndür:
[{"narration":"...","caption":"...","image_prompt":"..."}]`,
		scenes, req.DurationSec, langName, req.Brief, req.CompanyContext, plat.Guide, plat.CTA, narrationRule, orientation, langRule)

	provider, model := "openai", "gpt-4o-mini"
	var p models.SystemPrompt
	if database.DB != nil && database.DB.Where("prompt_key = ?", "content_idea_generator").First(&p).Error == nil {
		if p.AiProvider != "" {
			provider = strings.ToLower(p.AiProvider)
		}
		if p.AiModel != "" {
			model = p.AiModel
		}
	}
	var text string
	var err error
	if provider == "gemini" {
		text, err = callGeminiText(model, prompt)
	} else {
		text, err = callOpenAIText(model, prompt)
	}
	if err != nil {
		return nil, fmt.Errorf("senaryo yazılamadı: %v", err)
	}
	start, end := strings.Index(text, "["), strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("senaryo yanıtında JSON yok")
	}
	var out []VideoScene
	if err := json.Unmarshal([]byte(fixAIJSON(text[start:end+1])), &out); err != nil {
		return nil, fmt.Errorf("senaryo ayrıştırılamadı: %v", err)
	}
	if len(out) < 2 {
		return nil, fmt.Errorf("senaryoda yeterli sahne yok")
	}
	return out, nil
}

// Türkçe harf adları — kısaltmalar harf harf okunur (İK → "i ka")
var trLetterNames = map[rune]string{
	'A': "a", 'B': "be", 'C': "ce", 'Ç': "çe", 'D': "de", 'E': "e", 'F': "fe", 'G': "ge", 'Ğ': "yumuşak ge",
	'H': "he", 'I': "ı", 'İ': "i", 'J': "je", 'K': "ka", 'L': "le", 'M': "me", 'N': "ne", 'O': "o", 'Ö': "ö",
	'P': "pe", 'R': "re", 'S': "se", 'Ş': "şe", 'T': "te", 'U': "u", 'Ü': "ü", 'V': "ve", 'Y': "ye", 'Z': "ze",
	'Q': "kü", 'W': "dabılyu", 'X': "iks",
}

// Kelime gibi okunan kısa kısaltmalar
var acronymWords = map[string]string{"SEO": "seo", "NATO": "nato", "KOBİ": "kobi", "AVM": "a ve me", "DM": "di em", "CEO": "si i o", "AI": "yapay zekâ", "DIO": "Dio"}

// ParsePronunciations — "İNKA=İnka, İK=insan kaynakları" → eşleme
func ParsePronunciations(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == ';' }) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 && strings.TrimSpace(kv[0]) != "" && strings.TrimSpace(kv[1]) != "" {
			out[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return out
}

// SpeakableText — seslendirmeden önce metni ses modelinin doğru okuyacağı hale getirir:
// kullanıcı telaffuzları uygulanır; tamamı büyük harf kelimeler 2-3 harfse harf harf,
// daha uzunsa normal yazımla okunur (İNKA → İnka, İK → i ka).
func SpeakableText(text string, overrides map[string]string) string {
	for k, v := range overrides {
		text = strings.ReplaceAll(text, k, v)
	}
	var b strings.Builder
	word := []rune{}
	flush := func() {
		if len(word) == 0 {
			return
		}
		w := string(word)
		upper := len(word) >= 2
		for _, r := range word {
			if !unicode.IsUpper(r) {
				upper = false
				break
			}
		}
		switch {
		case !upper:
			b.WriteString(w)
		case acronymWords[w] != "":
			b.WriteString(acronymWords[w])
		case len(word) <= 3:
			parts := make([]string, 0, len(word))
			for _, r := range word {
				if n, ok := trLetterNames[r]; ok {
					parts = append(parts, n)
				} else {
					parts = append(parts, string(r))
				}
			}
			b.WriteString(strings.Join(parts, " "))
		default:
			lower := []rune(strings.ToLowerSpecial(unicode.TurkishCase, w))
			b.WriteString(string(word[0]) + string(lower[1:]))
		}
		word = word[:0]
	}
	for _, r := range text {
		if unicode.IsLetter(r) {
			word = append(word, r)
			continue
		}
		flush()
		b.WriteRune(r)
	}
	flush()
	return b.String()
}

// speechInstructions — sakin tempo + marka adının net telaffuzu (testte en tutarlı sonuç)
func speechInstructions(brandName string) string {
	ins := "Türkçe konuş. Reklam spikeri gibi değil, gerçek bir insan gibi sıcak ve samimi konuş; doğal tonlama ve vurgular kullan. Doğal sohbet hızında. Her kelimeyi net ve anlaşılır telaffuz et; heceleri yutma. Cümle aralarında kısa, doğal nefes duraklamaları ver."
	if name := strings.TrimSpace(SpeakableText(brandName, nil)); name != "" {
		ins += fmt.Sprintf(" Marka adı %q özenle ve net telaffuz edilir; hecelerini yutma, başına ya da sonuna ses ekleme.", name)
	}
	return ins
}

// synthesizeSpeech — OpenAI TTS ile Türkçe mp3
func synthesizeSpeech(text, voice, brandName, outPath string) error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY yok")
	}
	// En doğal (insan gibi) sesler; desteklenmezse eski seslere düşülür
	v, fallback := "marin", "coral"
	if voice == "erkek" {
		v, fallback = "cedar", "onyx"
	}
	call := func(voiceName string) ([]byte, int, error) {
		body, _ := json.Marshal(map[string]interface{}{
			"model":           "gpt-4o-mini-tts",
			"voice":           voiceName,
			"input":           text,
			"instructions":    speechInstructions(brandName),
			"response_format": "mp3",
		})
		req, _ := http.NewRequest("POST", "https://api.openai.com/v1/audio/speech", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return data, resp.StatusCode, nil
	}
	data, status, err := call(v)
	if err == nil && status >= 400 && status < 500 {
		log.Printf("⚠️ TTS sesi %q kullanılamadı (%d), %q deneniyor", v, status, fallback)
		data, status, err = call(fallback)
	}
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("TTS HTTP %d: %.200s", status, string(data))
	}
	return os.WriteFile(outPath, data, 0o644)
}

// synthesizeGemini — Gemini TTS: metnin dilini algılar ve ana dili konuşan biri gibi seslendirir.
// Testte Türkçe cümleler (marka adı dahil) birebir doğru anlaşıldı. Çıktı 24 kHz PCM → mp3.
func synthesizeGemini(text, voice, langName, outPath string) error {
	key := os.Getenv("GEMINI_API_KEY")
	if key == "" {
		return fmt.Errorf("GEMINI_API_KEY yok")
	}
	voiceName := "Kore"
	if voice == "erkek" {
		voiceName = "Charon"
	}
	style := fmt.Sprintf("Read the following aloud as a native %s speaker: warm, friendly and natural like a real person recommending something to a friend, at a natural conversational pace, with clear pronunciation: ", langName)
	if langName == "Türkçe" {
		style = "Ana dili Türkçe olan, İstanbul Türkçesiyle konuşan gerçek bir kişi gibi, sıcak, samimi ve doğal bir tonla, doğal konuşma hızında, net telaffuzla oku: "
	}
	body, _ := json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{{"parts": []map[string]string{{"text": style + text}}}},
		"generationConfig": map[string]interface{}{
			"responseModalities": []string{"AUDIO"},
			"speechConfig":       map[string]interface{}{"voiceConfig": map[string]interface{}{"prebuiltVoiceConfig": map[string]string{"voiceName": voiceName}}},
		},
	})
	url := GeminiAPIBase + "/gemini-2.5-flash-preview-tts:generateContent?key=" + key
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Gemini TTS HTTP %d: %.200s", resp.StatusCode, string(data))
	}
	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						Data string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &r); err != nil || len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 || r.Candidates[0].Content.Parts[0].InlineData == nil {
		return fmt.Errorf("Gemini TTS boş yanıt")
	}
	pcm, err := base64DecodeImage(r.Candidates[0].Content.Parts[0].InlineData.Data)
	if err != nil {
		return err
	}
	pcmPath := outPath + ".pcm"
	if err := os.WriteFile(pcmPath, pcm, 0o644); err != nil {
		return err
	}
	defer os.Remove(pcmPath)
	if out, err := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "s16le", "-ar", "24000", "-ac", "1", "-i", pcmPath, "-b:a", "160k", outPath).CombinedOutput(); err != nil {
		return fmt.Errorf("ses dönüştürülemedi: %v %s", err, lastLines(string(out)))
	}
	return nil
}

// speak — önce Gemini (en doğal), olmazsa OpenAI sesi
func speak(req VideoRequest, text, outPath string) error {
	code, name := videoLang(req.Language)
	if code == "tr" {
		text = SpeakableText(text, ParsePronunciations(req.Pronunciations))
	} else {
		for k, v := range ParsePronunciations(req.Pronunciations) {
			text = strings.ReplaceAll(text, k, v)
		}
		// Tamamı büyük harf marka adı (DIO, İNKA) harf harf değil kelime olarak okunsun
		for _, w := range strings.Fields(req.BrandName) {
			r := []rune(w)
			if len(r) >= 3 && strings.ToUpper(w) == w && strings.ToLower(w) != w {
				text = strings.ReplaceAll(text, w, string(r[0])+strings.ToLower(string(r[1:])))
			}
		}
	}
	if err := synthesizeGemini(text, req.Voice, name, outPath); err == nil {
		return nil
	} else {
		log.Printf("⚠️ Gemini seslendirme başarısız, OpenAI deneniyor: %v", err)
	}
	return synthesizeSpeech(text, req.Voice, req.BrandName, outPath)
}

func probeDuration(path string) (float64, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", path).Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

func downloadTo(url, path string) error {
	if strings.HasPrefix(url, "/") {
		data, err := os.ReadFile("." + url)
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("indirme HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// wrapCaption — drawtext otomatik satır kırmadığı için kelime bazlı satırlara böler
func wrapCaption(text string, maxChars int) string {
	var lines []string
	var cur string
	for _, w := range strings.Fields(text) {
		if cur == "" {
			cur = w
		} else if len([]rune(cur))+1+len([]rune(w)) <= maxChars {
			cur += " " + w
		} else {
			lines = append(lines, cur)
			cur = w
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n")
}

// RenderVideo — tüm hattı çalıştırır, CDN URL'i ve kapak görselini döndürür
func RenderVideo(req VideoRequest, scenes []VideoScene, progress ProgressFunc) (videoURL, thumbURL string, err error) {
	if err := VideoToolsAvailable(); err != nil {
		return "", "", err
	}
	work, err := os.MkdirTemp("", "dio-video-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(work)

	ratio := "9:16"
	W, H := 1080, 1920
	switch req.Format {
	case "1:1":
		ratio, W, H = "1:1", 1080, 1080
	case "16:9":
		ratio, W, H = "16:9", 1920, 1080
	}
	S := W // kısa kenar: logo/yazı ölçüleri buna göre
	if H < W {
		S = H
	}
	n := len(scenes)

	// 1) Sahne görselleri (en fazla 3 paralel)
	imgPaths := make([]string, n)
	imgURLs := make([]string, n)
	errs := make([]error, n)
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for i, sc := range scenes {
		wg.Add(1)
		go func(i int, sc VideoScene) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			url, e := GenerateBrandImage(BrandContext{
				BrandName: req.BrandName, Sector: req.Sector, BrandColors: req.BrandColors,
				ImagePrompt: sc.ImagePrompt + ". Keep the lower third calm and uncluttered.", Style: "realistic", AspectRatio: ratio,
			})
			if e == nil {
				p := filepath.Join(work, fmt.Sprintf("img_%02d.png", i))
				if e = downloadTo(url, p); e == nil {
					imgPaths[i], imgURLs[i] = p, url
				}
			}
			errs[i] = e
			mu.Lock()
			done++
			progress(fmt.Sprintf("Sahne görselleri üretiliyor (%d/%d)", done, n), 10+done*45/n)
			mu.Unlock()
		}(i, sc)
	}
	wg.Wait()
	// Başarısız sahne görseli varsa bir öncekini (yoksa ilk başarılıyı) kullan
	first := ""
	for _, p := range imgPaths {
		if p != "" {
			first = p
			break
		}
	}
	if first == "" {
		return "", "", fmt.Errorf("hiçbir sahne görseli üretilemedi: %v", errs[0])
	}
	for i := range imgPaths {
		if imgPaths[i] == "" {
			if i > 0 && imgPaths[i-1] != "" {
				imgPaths[i] = imgPaths[i-1]
			} else {
				imgPaths[i] = first
			}
		}
	}
	for _, u := range imgURLs {
		if u != "" {
			thumbURL = u
			break
		}
	}

	// 2) Seslendirme ve sahne süreleri
	durs := make([]float64, n)
	audio := make([]string, n)
	baseDur := float64(req.DurationSec) / float64(n)
	for i, sc := range scenes {
		durs[i] = math.Max(baseDur, 2.5)
		if req.Voice != "yok" && strings.TrimSpace(sc.Narration) != "" {
			p := filepath.Join(work, fmt.Sprintf("voice_%02d.mp3", i))
			if e := speak(req, sc.Narration, p); e != nil {
				log.Printf("⚠️ Sahne %d seslendirilemedi: %v", i+1, e)
			} else if d, e := probeDuration(p); e == nil {
				audio[i] = p
				durs[i] = math.Max(d+0.35, 2.5)
			}
		}
		progress(fmt.Sprintf("Seslendirme hazırlanıyor (%d/%d)", i+1, n), 55+(i+1)*15/n)
	}

	// 3) Sahne klipleri: yavaş yakınlaşma + alt yazı + yumuşak giriş/çıkış
	primary := "#e6272f"
	var colors []string
	if json.Unmarshal([]byte(req.BrandColors), &colors) == nil && len(colors) > 0 && strings.HasPrefix(colors[0], "#") && len(colors[0]) == 7 {
		primary = colors[0]
	}
	boxColor := "0x" + strings.TrimPrefix(primary, "#") + "@0.88"
	fontSize := 64
	maxChars := 18
	switch ratio {
	case "1:1":
		fontSize, maxChars = 56, 24
	case "16:9":
		fontSize, maxChars = 56, 34
	}

	// Marka logosu (şeffaf PNG) — giriş kartı ve köşe logosu için
	logoPNG := ""
	if req.Intro && req.LogoURL != "" {
		p := filepath.Join(work, "logo.png")
		if e := PrepareLogoPNG(req.LogoURL, p, 700); e != nil {
			log.Printf("⚠️ Video logosu hazırlanamadı (yalnızca marka adı kullanılacak): %v", e)
		} else {
			logoPNG = p
		}
	}
	encode := []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-r", "30", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "160k", "-ar", "44100", "-ac", "2", "-movflags", "+faststart"}

	list := &strings.Builder{}
	timeline := 0.0 // sahnelerin videodaki başlangıç saniyesi (giriş kartı dahil)

	// Giriş kartı (≈2 sn): beyaz zemin, ortada şeffaf logo, altında marka renginde ad, en altta marka şeridi
	if req.Intro {
		const introDur = 2.0
		nameFile := filepath.Join(work, "brand.txt")
		_ = os.WriteFile(nameFile, []byte(req.BrandName), 0o644)
		brandHex := "0x" + strings.TrimPrefix(primary, "#")
		nameY := "(h-text_h)/2"
		if logoPNG != "" {
			nameY = fmt.Sprintf("h*0.5+%d", int(float64(S)*0.20))
		}
		decor := fmt.Sprintf("drawbox=x=0:y=ih-%d:w=iw:h=%d:color=%s:t=fill,"+
			"drawtext=fontfile=%s:textfile=%s:fontsize=%d:fontcolor=%s:x=(w-text_w)/2:y=%s:alpha='min(1,max(0,(t-0.35)/0.4))'",
			int(float64(H)*0.06), int(float64(H)*0.06), brandHex, videoFont, nameFile, int(float64(S)*0.07), brandHex, nameY)
		intro := filepath.Join(work, "intro.mp4")
		args := []string{"-y", "-f", "lavfi", "-i", fmt.Sprintf("color=c=white:s=%dx%d:r=30:d=%.1f", W, H, introDur)}
		audioIdx := 1
		var fc string
		if logoPNG != "" {
			args = append(args, "-loop", "1", "-t", fmt.Sprint(introDur), "-i", logoPNG)
			audioIdx = 2
			fc = fmt.Sprintf("[1:v]scale=%d:-1,format=rgba,fade=t=in:st=0.1:d=0.45:alpha=1[lg];[0:v]%s[bgc];"+
				"[bgc][lg]overlay=(W-w)/2:H*0.5-h/2-%d,fade=t=out:st=%.2f:d=0.3,format=yuv420p[v]",
				int(float64(S)*0.6), decor, int(float64(S)*0.04), introDur-0.3)
		} else {
			fc = fmt.Sprintf("[0:v]%s,fade=t=out:st=%.2f:d=0.3,format=yuv420p[v]", decor, introDur-0.3)
		}
		args = append(args, "-f", "lavfi", "-t", fmt.Sprint(introDur), "-i", "anullsrc=channel_layout=stereo:sample_rate=44100",
			"-filter_complex", fc, "-map", "[v]", "-map", fmt.Sprintf("%d:a", audioIdx), "-t", fmt.Sprint(introDur))
		args = append(args, encode...)
		args = append(args, intro)
		if out, e := exec.Command("ffmpeg", args...).CombinedOutput(); e != nil {
			log.Printf("⚠️ Giriş kartı oluşturulamadı, girişsiz devam: %v — %s", e, lastLines(string(out)))
		} else {
			fmt.Fprintf(list, "file '%s'\n", intro)
			timeline = introDur
		}
	}

	for i, sc := range scenes {
		frames := int(math.Ceil(durs[i] * 30))
		zoomIn := i%2 == 0
		zoomExpr := "min(zoom+0.0009,1.15)"
		if !zoomIn {
			zoomExpr = "if(eq(on,0),1.15,max(zoom-0.0009,1.0))"
		}
		fadeOut := math.Max(durs[i]-0.35, 0.1)
		base := fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,zoompan=z='%s':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=30",
			int(float64(W)*1.2), int(float64(H)*1.2), int(float64(W)*1.2), int(float64(H)*1.2), zoomExpr, frames, W, H)
		if req.Subtitles && strings.TrimSpace(sc.Caption) != "" {
			capFile := filepath.Join(work, fmt.Sprintf("cap_%02d.txt", i))
			_ = os.WriteFile(capFile, []byte(wrapCaption(sc.Caption, maxChars)), 0o644)
			base += fmt.Sprintf(",drawtext=fontfile=%s:textfile=%s:fontsize=%d:fontcolor=white:line_spacing=14:box=1:boxcolor=%s:boxborderw=28:x=(w-text_w)/2:y=h*0.70",
				videoFont, capFile, fontSize, boxColor)
		}
		base += fmt.Sprintf(",fade=t=in:st=0:d=0.35,fade=t=out:st=%.2f:d=0.35", fadeOut)

		clip := filepath.Join(work, fmt.Sprintf("clip_%02d.mp4", i))
		args := []string{"-y", "-loop", "1", "-i", imgPaths[i]}
		if audio[i] != "" {
			args = append(args, "-i", audio[i])
		} else {
			args = append(args, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100")
		}
		fc := "[0:v]" + base + ",format=yuv420p[v]"
		if logoPNG != "" {
			// Köşe logosu: küçük ve şeffaf (arkasında kutu yok)
			lw := int(float64(S) * 0.16)
			args = append(args, "-i", logoPNG)
			fc = fmt.Sprintf("[0:v]%s[b];[2:v]scale=%d:-1,format=rgba,colorchannelmixer=aa=0.92[wm];[b][wm]overlay=40:40,format=yuv420p[v]", base, lw)
		}
		args = append(args, "-filter_complex", fc, "-map", "[v]", "-map", "1:a", "-t", fmt.Sprintf("%.2f", durs[i]),
			"-af", fmt.Sprintf("apad,atrim=0:%.2f", durs[i]))
		args = append(args, encode...)
		args = append(args, clip)
		if out, e := exec.Command("ffmpeg", args...).CombinedOutput(); e != nil {
			return "", "", fmt.Errorf("sahne %d işlenemedi: %v — %.300s", i+1, e, lastLines(string(out)))
		}
		fmt.Fprintf(list, "file '%s'\n", clip)
		scenes[i].Start = math.Round(timeline*10) / 10
		timeline += durs[i]
		scenes[i].End = math.Round(timeline*10) / 10
		progress(fmt.Sprintf("Video montajı (%d/%d)", i+1, n), 70+(i+1)*25/n)
	}

	// 4) Birleştir
	listFile := filepath.Join(work, "list.txt")
	_ = os.WriteFile(listFile, []byte(list.String()), 0o644)
	final := filepath.Join(work, "final.mp4")
	if out, e := exec.Command("ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", listFile, "-c", "copy", "-movflags", "+faststart", final).CombinedOutput(); e != nil {
		return "", "", fmt.Errorf("video birleştirilemedi: %v — %.300s", e, lastLines(string(out)))
	}
	data, err := os.ReadFile(final)
	if err != nil {
		return "", "", err
	}

	// 5) Yükle (CDN, yoksa yerel uploads)
	progress("Video yükleniyor", 97)
	name := fmt.Sprintf("video_%d.mp4", time.Now().UnixNano())
	if IsBunnyCDNConfigured() {
		if url, e := UploadToBunnyCDN(data, "videos", name); e == nil {
			return url, thumbURL, nil
		} else {
			log.Printf("⚠️ Video CDN'e yüklenemedi, yerele kaydediliyor: %v", e)
		}
	}
	dir := "./assets/uploads/videos"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return "", "", err
	}
	return "/uploads/videos/" + name, thumbURL, nil
}

// VideoCopy — mecraya hazır paylaşım metni
type VideoCopy struct {
	Title    string `json:"title"`
	Caption  string `json:"caption"`
	Hashtags string `json:"hashtags"`
}

// FormatTimestamp — 75.3 → "1:15"
func FormatTimestamp(sec float64) string {
	t := int(sec)
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

// GenerateVideoCopy — videonun senaryosundan mecraya uygun başlık/açıklama/hashtag üretir.
// Yatay YouTube videolarında açıklamaya zaman akışından bölüm işaretleri eklenir.
func GenerateVideoCopy(req VideoRequest, scenes []VideoScene) (VideoCopy, error) {
	_, plat := videoPlatform(req.Platform)
	_, langName := videoLang(req.Language)
	var b strings.Builder
	for _, sc := range scenes {
		fmt.Fprintf(&b, "- [%s] %s / %s\n", FormatTimestamp(sc.Start), sc.Caption, sc.Narration)
	}
	prompt := fmt.Sprintf(`Aşağıdaki %s videosu için paylaşım metni yaz. Dil: %s. Marka: %s.
KURALLAR: %s Uydurma rakam/yorum yok, doğal ve samimi dil.
VİDEO AKIŞI:
%s
Sadece JSON döndür: {"title":"...","caption":"...","hashtags":"#... #..."}  (Instagram/TikTok için title kısa bir kapak başlığı olsun)`,
		plat.Label, langName, req.BrandName, plat.CopyFmt, b.String())

	provider, model := "openai", "gpt-4o-mini"
	var p models.SystemPrompt
	if database.DB != nil && database.DB.Where("prompt_key = ?", "content_idea_generator").First(&p).Error == nil {
		if p.AiProvider != "" {
			provider = strings.ToLower(p.AiProvider)
		}
		if p.AiModel != "" {
			model = p.AiModel
		}
	}
	var text string
	var err error
	if provider == "gemini" {
		text, err = callGeminiText(model, prompt)
	} else {
		text, err = callOpenAIText(model, prompt)
	}
	if err != nil {
		return VideoCopy{}, err
	}
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return VideoCopy{}, fmt.Errorf("paylaşım metni JSON değil")
	}
	var c VideoCopy
	if err := json.Unmarshal([]byte(fixAIJSON(text[start:end+1])), &c); err != nil {
		return VideoCopy{}, err
	}
	// YouTube bölümleri: ilk 0:00 olmalı, en az 3 bölüm ve her biri ≥10 sn
	if req.Platform == "youtube" && len(scenes) >= 3 {
		var ch []string
		last := -10.0
		for i, sc := range scenes {
			start := sc.Start
			if i == 0 {
				start = 0
			}
			if start-last >= 10 || i == 0 {
				ch = append(ch, FormatTimestamp(start)+" "+sc.Caption)
				last = start
			}
		}
		if len(ch) >= 3 {
			c.Caption = strings.TrimSpace(c.Caption) + "\n\nBölümler:\n" + strings.Join(ch, "\n")
		}
	}
	return c, nil
}

func lastLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 4 {
		lines = lines[len(lines)-4:]
	}
	return strings.Join(lines, " | ")
}
