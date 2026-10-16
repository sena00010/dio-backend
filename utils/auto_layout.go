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
)

// AutoLayoutInput — görsel üzerine yerleştirilecek metinler ve marka bilgisi
type AutoLayoutInput struct {
	ImageURL    string // CDN URL'i veya /uploads/... yolu
	AspectRatio string // "4:5", "1:1" ... (tuval oranı)
	Headline    string
	Description string
	CTA         string
	BrandColors string // JSON dizi
	BrandFont   string
}

// AutoLayoutResult — AI'ın görsele bakarak önerdiği katmanlar ve logo konumu
type AutoLayoutResult struct {
	Layers  []DesignLayer       `json:"layers"`
	Logo    *LogoPositionResult `json:"logo,omitempty"`
	Subject *layoutBox          `json:"subject,omitempty"` // görseldeki ana nesnenin kutusu (yüzde)
}

type layoutBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// loadLayoutImage — http(s) URL'den ya da yerel /uploads yolundan görseli okur
func loadLayoutImage(src string) ([]byte, string, error) {
	var data []byte
	var err error
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, e := client.Get(src)
		if e != nil {
			return nil, "", fmt.Errorf("görsel indirilemedi: %v", e)
		}
		defer resp.Body.Close()
		data, err = io.ReadAll(io.LimitReader(resp.Body, 15<<20))
	} else {
		data, err = os.ReadFile("." + src)
	}
	if err != nil {
		return nil, "", fmt.Errorf("görsel okunamadı: %v", err)
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", fmt.Errorf("dosya görsel değil (%s)", mime)
	}
	return data, mime, nil
}

func autoLayoutPrompt(in AutoLayoutInput) string {
	ratio := in.AspectRatio
	if ratio == "" {
		ratio = "1:1"
	}
	font := in.BrandFont
	if font == "" {
		font = "a bold modern sans-serif (Montserrat, Poppins or Bebas Neue for headline; Inter or DM Sans for body)"
	}
	colors := in.BrandColors
	if colors == "" {
		colors = "not specified — pick white or a dark tone from the image for best contrast"
	}
	return fmt.Sprintf(`You are a senior social media designer working in a Canva-like editor. LOOK at the attached image carefully and place the given texts on it like a professional post.

CANVAS: width = 520 design px, aspect ratio %s. Coordinates are PERCENT of canvas (x from left, y from top). font_size is in design px for the 520px-wide canvas.

TEXTS (use exactly, do not translate or rewrite):
- headline: %q
- description: %q
- cta: %q

BRAND: colors %s; preferred font: %s.

RULES:
1. Put text ONLY on calm, low-detail areas (sky, wall, table surface, blurred background, dark/empty zones). NEVER cover the main subject, faces or the product.
2. Guarantee readability: choose text color with strong contrast against the area under it (white on dark, near-black on light). Set text_shadow true on photos.
3. Hierarchy: headline biggest (28-46px, weight 800), description 13-16px (weight 400-500), cta 13-15px (weight 700) as a pill button: bg_color = brand primary color (or #e6272f if none), color #ffffff, border_radius 24, padding 10.
4. All layers must stay inside the safe area: x 6-94, y 6-88, x + max_width <= 94. Layers must NOT overlap (keep at least 4%% vertical gap). Align them to a common left edge or center them as a group.
5. max_width: headline 45-85, description 45-80, cta 25-45.
6. Skip a layer if its text is empty.
7. Also pick a logo spot: a calm corner area not overlapping the texts (x,y = top-left of logo in percent, max_width 10-16).
8. FIRST identify the main subject's bounding box ("subject", percent of canvas). No text layer may intersect it.

Respond ONLY with JSON:
{"subject":{"x":0,"y":0,"w":0,"h":0},
 "layers":[{"id":"headline","type":"headline","text":"...","x":0,"y":0,"max_width":0,"font_family":"...","font_size":0,"font_weight":800,"color":"#ffffff","text_align":"left","line_height":1.1,"text_shadow":true}, {"id":"desc","type":"description",...}, {"id":"cta","type":"cta",...,"bg_color":"#e6272f","border_radius":24,"padding":10}],
 "logo":{"x":0,"y":0,"max_width":12,"max_height":12,"reasoning":"..."}}`,
		ratio, in.Headline, in.Description, in.CTA, colors, font)
}

// AutoLayout — görseli vision modeline gösterip metin katmanlarını yerleştirir.
// Önce Gemini Vision, olmazsa OpenAI vision (gpt_smart_general modeli) denenir.
func AutoLayout(in AutoLayoutInput) (*AutoLayoutResult, error) {
	img, mime, err := loadLayoutImage(in.ImageURL)
	if err != nil {
		return nil, err
	}
	b64 := base64.StdEncoding.EncodeToString(img)
	prompt := autoLayoutPrompt(in)

	var raw string
	if key := os.Getenv("GEMINI_API_KEY"); key != "" {
		raw, err = autoLayoutGemini(key, mime, b64, prompt)
		if err != nil {
			log.Printf("⚠️ Gemini Vision yerleşimi başarısız, OpenAI deneniyor: %v", err)
		}
	}
	if raw == "" {
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("yerleşim için API anahtarı yok")
		}
		raw, err = autoLayoutOpenAI(key, mime, b64, prompt)
		if err != nil {
			return nil, fmt.Errorf("AI yerleşim başarısız: %v", err)
		}
	}

	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("yerleşim yanıtında JSON yok")
	}
	var res AutoLayoutResult
	if err := json.Unmarshal([]byte(fixAIJSON(raw[start:end+1])), &res); err != nil {
		return nil, fmt.Errorf("yerleşim JSON ayrıştırılamadı: %v", err)
	}
	res.Layers = sanitizeLayout(res.Layers)
	res.Layers = avoidSubject(res.Layers, res.Subject, in.AspectRatio)
	if len(res.Layers) == 0 {
		return nil, fmt.Errorf("AI hiç katman döndürmedi")
	}
	res.Layers = resolveLayerCollisions(res.Layers)
	if res.Logo != nil {
		res.Logo.X = clampF(res.Logo.X, 2, 90)
		res.Logo.Y = clampF(res.Logo.Y, 2, 90)
		res.Logo.MaxWidth = clampF(res.Logo.MaxWidth, 8, 18)
	}
	return &res, nil
}

func isLightHex(hex string) bool {
	h := strings.TrimPrefix(hex, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) < 6 {
		return true
	}
	var r, g, b int
	if _, err := fmt.Sscanf(h[:6], "%02x%02x%02x", &r, &g, &b); err != nil {
		return true
	}
	return 0.299*float64(r)+0.587*float64(g)+0.114*float64(b) > 150
}

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// avoidSubject — ana nesneye çarpan katmanı nesnenin yanına sığacak şekilde daraltır;
// sığmıyorsa nesnenin altına/üstüne kaydırır.
func avoidSubject(layers []DesignLayer, sub *layoutBox, ratio string) []DesignLayer {
	if sub == nil || sub.W <= 0 || sub.H <= 0 {
		return layers
	}
	canvasH := 520.0
	switch ratio {
	case "4:5":
		canvasH = 650
	case "9:16":
		canvasH = 924
	case "16:9":
		canvasH = 292
	}
	for i := range layers {
		l := &layers[i]
		// Yaklaşık yükseklik: satır sayısı × font × satır aralığı
		charsPerLine := (l.MaxWidth / 100 * 520) / (float64(l.FontSize) * 0.55)
		if charsPerLine < 1 {
			charsPerLine = 1
		}
		lines := float64(int(float64(len([]rune(l.Text)))/charsPerLine) + 1)
		hPct := lines * float64(l.FontSize) * l.LineHeight / canvasH * 100
		if l.Padding > 0 {
			hPct += float64(l.Padding*2) / canvasH * 100
		}
		overlapX := l.X < sub.X+sub.W && l.X+l.MaxWidth > sub.X
		overlapY := l.Y < sub.Y+sub.H && l.Y+hPct > sub.Y
		if !overlapX || !overlapY {
			continue
		}
		switch {
		case l.X < sub.X && sub.X-2-l.X >= 25: // nesnenin solunda dar sütun
			l.MaxWidth = sub.X - 2 - l.X
		case sub.X+sub.W+2+25 <= 94: // nesnenin sağına taşı
			l.X = sub.X + sub.W + 2
			l.MaxWidth = clampF(94-l.X, 25, l.MaxWidth)
		case sub.Y+sub.H+2+hPct <= 90: // altına
			l.Y = sub.Y + sub.H + 2
		case sub.Y-2-hPct >= 6: // üstüne
			l.Y = sub.Y - 2 - hPct
		}
	}
	return layers
}

// sanitizeLayout — model çıktısını güvenli alana ve makul değerlere çeker
func sanitizeLayout(in []DesignLayer) []DesignLayer {
	out := make([]DesignLayer, 0, len(in))
	for _, l := range in {
		if strings.TrimSpace(l.Text) == "" {
			continue
		}
		switch l.Type {
		case "headline", "description", "cta", "badge":
		default:
			l.Type = "description"
		}
		l.MaxWidth = clampF(l.MaxWidth, 20, 88)
		l.X = clampF(l.X, 4, 96-l.MaxWidth)
		l.Y = clampF(l.Y, 4, 90)
		minFS, maxFS := 12, 18
		if l.Type == "headline" {
			minFS, maxFS = 22, 52
		}
		if l.FontSize < minFS || l.FontSize > maxFS {
			l.FontSize = (minFS + maxFS) / 2
		}
		if l.FontWeight == 0 {
			l.FontWeight = 600
		}
		if l.Color == "" {
			l.Color = "#ffffff"
		}
		// Gölge yalnızca açık renkli yazıda (koyu yazıda kirli görünür); CTA butonunda gölge yok
		l.TextShadow = l.Type != "cta" && isLightHex(l.Color)
		if l.TextAlign == "" {
			l.TextAlign = "left"
		}
		if l.LineHeight == 0 {
			l.LineHeight = 1.2
		}
		if l.FontFamily == "" {
			l.FontFamily = "Montserrat"
		}
		if l.ID == "" {
			l.ID = l.Type
		}
		out = append(out, l)
	}
	return out
}

func autoLayoutGemini(apiKey, mime, b64, prompt string) (string, error) {
	body := map[string]interface{}{
		"contents": []map[string]interface{}{{
			"parts": []map[string]interface{}{
				{"inlineData": map[string]string{"mimeType": mime, "data": b64}},
				{"text": prompt},
			},
		}},
		"generationConfig": map[string]interface{}{"responseMimeType": "application/json", "temperature": 0.3},
	}
	bb, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", GeminiTextURL(apiKey), bytes.NewBuffer(bb))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini HTTP %d: %.300s", resp.StatusCode, string(rb))
	}
	var r struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(rb, &r); err != nil || len(r.Candidates) == 0 || len(r.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini boş yanıt")
	}
	return r.Candidates[0].Content.Parts[0].Text, nil
}

func autoLayoutOpenAI(apiKey, mime, b64, prompt string) (string, error) {
	body := map[string]interface{}{
		"model":       GetAiModel("gpt_smart_general", "gpt-4o"),
		"temperature": 0.3,
		"max_tokens":  1500,
		"messages": []map[string]interface{}{{
			"role": "user",
			"content": []map[string]interface{}{
				{"type": "text", "text": prompt},
				{"type": "image_url", "image_url": map[string]string{"url": "data:" + mime + ";base64," + b64}},
			},
		}},
	}
	bb, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(AdaptChatBody(bb)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI HTTP %d: %.300s", resp.StatusCode, string(rb))
	}
	var r struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rb, &r); err != nil || len(r.Choices) == 0 {
		return "", fmt.Errorf("OpenAI boş yanıt")
	}
	return r.Choices[0].Message.Content, nil
}
