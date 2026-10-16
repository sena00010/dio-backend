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

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ──────────────────────────────────────────────
// Kısa video üretimi: senaryo → sahne görselleri → Türkçe seslendirme → ffmpeg montaj
// ──────────────────────────────────────────────

// VideoScene — senaryonun bir sahnesi
type VideoScene struct {
	Narration   string `json:"narration"`    // seslendirme metni
	Caption     string `json:"caption"`      // ekran yazısı (kısa)
	ImagePrompt string `json:"image_prompt"` // İngilizce görsel tarifi
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

// GenerateVideoScript — süreye göre sahnelere bölünmüş senaryo (~5 sn/sahne, Türkçe TTS ~1.8 kelime/sn)
func GenerateVideoScript(req VideoRequest) ([]VideoScene, error) {
	scenes := int(math.Round(float64(req.DurationSec) / 5))
	if scenes < 3 {
		scenes = 3
	}
	if scenes > 36 {
		scenes = 36
	}
	words := int(float64(req.DurationSec) * 1.8)
	narrationRule := fmt.Sprintf("Seslendirme toplamı yaklaşık %d kelime olsun (sahne başına %d-%d kelime).", words, words/scenes-2, words/scenes+3)
	if req.Voice == "yok" {
		narrationRule = "Seslendirme yok; narration alanını kısa tut (sahne başına en fazla 8 kelime), asıl mesaj caption'da."
	}

	prompt := fmt.Sprintf(`Sen Türkiye'nin en iyi kısa video (Reels/TikTok) senaristisin. Aşağıdaki içerik için TAM OLARAK %d sahnelik, toplam yaklaşık %d saniyelik dikey bir video senaryosu yaz.

İÇERİK / BRIEF: %s
MARKA BAĞLAMI: %s

YAPI:
- 1. sahne: ilk 2 saniyede izleyiciyi durduran güçlü bir kanca (soru, şaşırtıcı bilgi, cesur iddia).
- Ortadaki sahneler: her biri tek somut fikir; akıcı, birbirine bağlanan anlatım.
- Son sahne: net harekete çağrı (takip et, DM, kaydet, linke tıkla) ve marka adı.
- %s
- caption: ekranda görünecek kısa yazı, en fazla 6 kelime, emojisiz.
- image_prompt: o sahne için İngilizce, gerçekçi, dikey kompozisyonlu görsel tarifi; görselde YAZI olmasın; sahneler görsel olarak tutarlı bir hikâye anlatsın.
- Doğal, akıcı Türkçe; doğru Türkçe karakterler. Uydurma rakam, müşteri adı veya yorum YOK.

Sadece geçerli JSON dizisi döndür:
[{"narration":"...","caption":"...","image_prompt":"..."}]`,
		scenes, req.DurationSec, req.Brief, req.CompanyContext, narrationRule)

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

// synthesizeSpeech — OpenAI TTS ile Türkçe mp3
func synthesizeSpeech(text, voice, outPath string) error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY yok")
	}
	v := "coral"
	if voice == "erkek" {
		v = "onyx"
	}
	body, _ := json.Marshal(map[string]interface{}{
		"model":           "gpt-4o-mini-tts",
		"voice":           v,
		"input":           text,
		"instructions":    "Türkçe konuş. Doğal, sıcak ve enerjik bir sosyal medya reklamı seslendirmesi; net telaffuz, akıcı tempo.",
		"response_format": "mp3",
	})
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/audio/speech", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("TTS HTTP %d: %.200s", resp.StatusCode, string(data))
	}
	return os.WriteFile(outPath, data, 0o644)
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
	if req.Format == "1:1" {
		ratio, W, H = "1:1", 1080, 1080
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
				ImagePrompt: sc.ImagePrompt, Style: "realistic", AspectRatio: ratio, TextPosition: "bottom_center",
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
			if e := synthesizeSpeech(sc.Narration, req.Voice, p); e != nil {
				log.Printf("⚠️ Sahne %d seslendirilemedi: %v", i+1, e)
			} else if d, e := probeDuration(p); e == nil {
				audio[i] = p
				durs[i] = math.Max(d+0.35, 2.5)
			}
		}
		progress(fmt.Sprintf("Seslendirme hazırlanıyor (%d/%d)", i+1, n), 55+(i+1)*15/n)
	}

	// Seslendirme hedef süreyi aşarsa sesi perdesi değişmeden en fazla %20 hızlandır
	tempo := 1.0
	total := 0.0
	for _, d := range durs {
		total += d
	}
	if total > float64(req.DurationSec)*1.05 {
		tempo = math.Min(total/float64(req.DurationSec), 1.2)
		for i := range durs {
			durs[i] = math.Max(durs[i]/tempo, 2.2)
		}
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
	if ratio == "1:1" {
		fontSize, maxChars = 56, 24
	}

	list := &strings.Builder{}
	for i, sc := range scenes {
		frames := int(math.Ceil(durs[i] * 30))
		capFile := filepath.Join(work, fmt.Sprintf("cap_%02d.txt", i))
		_ = os.WriteFile(capFile, []byte(wrapCaption(sc.Caption, maxChars)), 0o644)
		zoomIn := i%2 == 0
		zoomExpr := "min(zoom+0.0009,1.15)"
		if !zoomIn {
			zoomExpr = "if(eq(on,0),1.15,max(zoom-0.0009,1.0))"
		}
		fadeOut := math.Max(durs[i]-0.35, 0.1)
		vf := fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,zoompan=z='%s':x='iw/2-(iw/zoom/2)':y='ih/2-(ih/zoom/2)':d=%d:s=%dx%d:fps=30,"+
				"drawtext=fontfile=%s:textfile=%s:fontsize=%d:fontcolor=white:line_spacing=14:box=1:boxcolor=%s:boxborderw=28:x=(w-text_w)/2:y=h*0.70,"+
				"fade=t=in:st=0:d=0.35,fade=t=out:st=%.2f:d=0.35,format=yuv420p",
			int(float64(W)*1.2), int(float64(H)*1.2), int(float64(W)*1.2), int(float64(H)*1.2), zoomExpr, frames, W, H,
			videoFont, capFile, fontSize, boxColor, fadeOut)

		clip := filepath.Join(work, fmt.Sprintf("clip_%02d.mp4", i))
		args := []string{"-y", "-loop", "1", "-i", imgPaths[i]}
		if audio[i] != "" {
			args = append(args, "-i", audio[i])
		} else {
			args = append(args, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100")
		}
		args = append(args, "-vf", vf, "-t", fmt.Sprintf("%.2f", durs[i]),
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "21", "-r", "30",
			"-c:a", "aac", "-b:a", "160k", "-ar", "44100", "-ac", "2",
			"-af", fmt.Sprintf("atempo=%.3f,apad,atrim=0:%.2f", tempo, durs[i]), "-movflags", "+faststart", clip)
		if out, e := exec.Command("ffmpeg", args...).CombinedOutput(); e != nil {
			return "", "", fmt.Errorf("sahne %d işlenemedi: %v — %.300s", i+1, e, lastLines(string(out)))
		}
		fmt.Fprintf(list, "file '%s'\n", clip)
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

func lastLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 4 {
		lines = lines[len(lines)-4:]
	}
	return strings.Join(lines, " | ")
}
