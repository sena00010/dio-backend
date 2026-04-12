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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ──────────────────────────────────────────────
// Ortak tipler
// ──────────────────────────────────────────────

// ContentItem — eski madde öğesi (geriye uyumlu)
type ContentItem struct {
	Icon  string `json:"icon"`
	Title string `json:"title"`
}

// DesignLayer — Canva-tarzı bir tasarım katmanı
type DesignLayer struct {
	ID            string  `json:"id"`
	Type          string  `json:"type"` // headline, description, cta, badge
	Text          string  `json:"text"`
	X             float64 `json:"x"`         // % from left (0-100)
	Y             float64 `json:"y"`         // % from top (0-100)
	MaxWidth      float64 `json:"max_width"` // % width (20-100)
	FontFamily    string  `json:"font_family"`
	FontSize      int     `json:"font_size"`
	FontWeight    int     `json:"font_weight"` // 400, 600, 700, 800
	Color         string  `json:"color"`       // hex
	TextAlign     string  `json:"text_align"`  // left, center, right
	BgColor       string  `json:"bg_color,omitempty"`
	BorderRadius  int     `json:"border_radius,omitempty"`
	Padding       int     `json:"padding,omitempty"`
	LineHeight    float64 `json:"line_height,omitempty"`
	Opacity       float64 `json:"opacity,omitempty"`         // 0.0-1.0 (default 1.0)
	LetterSpacing float64 `json:"letter_spacing,omitempty"`  // px (e.g. 1.5, 3.0)
	TextTransform string  `json:"text_transform,omitempty"`  // uppercase, capitalize, none
	TextShadow    bool    `json:"text_shadow,omitempty"`     // enable/disable text shadow
	Rotation      float64 `json:"rotation,omitempty"`        // degrees (-45 to 45)
}

// IdeaResult — AI'dan dönen tek bir fikir
type IdeaResult struct {
	Title                 string        `json:"title"`
	Description           string        `json:"description"`
	Hashtags              string        `json:"hashtags"`
	ImageSuggestionPrompt string        `json:"image_suggestion_prompt"`
	SuggestedLayout       string        `json:"suggested_layout"`
	SuggestedFont         string        `json:"suggested_font"`
	LogoPosition          string        `json:"logo_position"`
	ContentItems          []ContentItem `json:"content_items"`
	DesignLayers          []DesignLayer `json:"design_layers"`
}

// ──────────────────────────────────────────────
// Ana fonksiyon — Gemini birincil, OpenAI yedek
// ──────────────────────────────────────────────

func GenerateIdeas(topic, platform, campaignType string, count int, companyContext string) ([]IdeaResult, error) {
	// Önce Gemini dene
	results, err := generateWithGemini(topic, platform, campaignType, count, companyContext)
	if err == nil {
		// Post-process: her fikrin design_layers'ında çakışma düzelt
		for i := range results {
			results[i].DesignLayers = resolveLayerCollisions(results[i].DesignLayers)
		}
		return results, nil
	}
	log.Printf("⚠️ Gemini hatası, OpenAI'ye geçiliyor: %v", err)

	// OpenAI fallback
	results, err = generateWithOpenAI(topic, platform, campaignType, count, companyContext)
	if err != nil {
		return nil, fmt.Errorf("hem Gemini hem OpenAI başarısız oldu: %v", err)
	}
	// Post-process: her fikrin design_layers'ında çakışma düzelt
	for i := range results {
		results[i].DesignLayers = resolveLayerCollisions(results[i].DesignLayers)
	}
	return results, nil
}

// ──────────────────────────────────────────────
// Prompt — Canva AI Tasarım Sistemi
// ──────────────────────────────────────────────

func buildPrompt(topic, platform, campaignType string, count int, companyContext string) string {
	return fmt.Sprintf(`You are a legendary art director with 20 years of experience designing for Vogue, Apple, and Netflix. You create social media layouts where every design choice — font, color, position — is driven by the IMAGE and CONTENT mood. No two designs should ever look alike.

Generate exactly %d unique content ideas for:

**Topic:** %s
**Platform:** %s
**Campaign Type:** %s
**Brand Context:** %s

For each idea, provide ALL fields:

1. "title": Short, impactful post title (brand's language, max 8 words)
2. "description": Full post caption/description (brand's language). If social media usernames are provided in the brand context, casually mention them or use them as CTA.
3. "hashtags": Relevant hashtags as a single string
4. "image_suggestion_prompt": Detailed English prompt for AI image generation. Describe scenes, colors, lighting. **CRITICAL: You MUST explicitly instruct the photo's background, lighting, subjects, or props to match the brand colors defined in the system prompt. E.g., 'bathed in [BRAND HEX] cinematic lighting' or 'wearing a [BRAND HEX] jacket'.** Do NOT mention any text/letters/logos.
5. "suggested_layout": One of: "top_left","top_center","top_right","center_left","center","center_right","bottom_left","bottom_center","bottom_right"
6. "suggested_font": Choose the PERFECT Google Font for this specific content. Think like a master typographer:
   - ANALYZE the content's emotion: Is it warm? Bold? Elegant? Playful? Serious? Nostalgic?
   - ANALYZE the image you described: Is it cinematic? Bright? Dark? Food? People? Nature? Product?
   - MATCH the font to BOTH: A food festival needs a different font than a tech launch
   - USE VARIETY: Available fonts include Cormorant Garamond, Playfair Display, Spectral, Lora, Crimson Text, Libre Baskerville, Abril Fatface, Cinzel, Oswald, Kanit, Archivo Black, Space Grotesk, Sora, Outfit, Lexend, Caveat, Satisfy, Pacifico, Montserrat, Poppins, Raleway, DM Sans, Quicksand, Merriweather, Bebas Neue, Righteous, Inter, Roboto, Fredoka One, and many more
   - NEVER default to the same font repeatedly — every post deserves its own typographic personality
7. "logo_position": "auto" — always set to "auto". Logo placement is determined dynamically by AI vision analysis after image generation.
8. "content_items": Bullet points array (can be empty [])
9. "design_layers": (CRITICAL — Canva-like layout) A JSON array of ALL visual text elements to place on the poster. Each element:
   - "id": unique string (e.g. "headline", "desc", "cta", "badge_platform")
   - "type": "headline" | "description" | "cta" | "badge"
   - "text": the actual text content to display
   - "x": horizontal position 0-100 (percentage from left)
   - "y": vertical position 0-100 (percentage from top)
   - "max_width": element width as percentage 20-90
   - "font_family": font name from the EXPANDED list above — match font to element type (headline can be bold/creative, description should be readable)
   - "font_size": pixels (headline: 28-42, description: 12-15, badge: 10-12, cta: 12-14)
   - "font_weight": 400 (regular), 600 (semibold), 700 (bold), 800 (extrabold)
   - "color": hex color (e.g. "#ffffff", "#000000", "#ffffffcc")
   - "text_align": "left" | "center" | "right"
   - "bg_color": (optional) background for badges/CTAs, e.g. "rgba(0,0,0,0.5)" or "rgba(255,255,255,0.9)"
   - "border_radius": (optional) for rounded badges, in pixels
   - "padding": (optional) inner spacing in pixels
   - "line_height": (optional) e.g. 1.3, 1.5

DESIGN_LAYERS RULES (THINK LIKE A 20-YEAR SENIOR ART DIRECTOR):
- Every post MUST have at minimum a "headline" layer
- Text must feel EMBEDDED in the image — not stuck on top. Use colors that harmonize with the image mood
- Description: POSTER-SHORT version (max 12-15 words). This is NOT the full caption
- Maximum 3 layers total. Less is more — 2 impactful layers beats 4 cluttered ones

🚨 CRITICAL: LAYERS MUST NEVER OVERLAP 🚨
- Think of the canvas as a VERTICAL STACK. Each layer has a vertical zone.
- Headline occupies ~18%% height, description ~8%%, CTA/badge ~6%%
- MINIMUM 5%% vertical gap between each layer's y coordinate
- Layers must flow top-to-bottom: headline first, then description, then CTA/badge
- NEVER place two layers at similar y values (within 15%% of each other)
- Example: headline y=30, description y=55, CTA y=72 ← correct spacing

- VARY COMPOSITION DRAMATICALLY between posts:
  * Cinematic bottom (headline y:55, desc y:73) — movie poster feel
  * Editorial top (headline y:12, desc y:32) — magazine cover feel
  * Dramatic center (headline y:35, desc y:58) — bold, unmissable
- FONT SELECTION by mood:
  * Emotional/warm → Serif: "Cormorant Garamond", "Playfair Display", "Spectral", "Lora"
  * Powerful/bold → Display: "Archivo Black", "Oswald", "Kanit"
  * Elegant/luxury → "Abril Fatface", "Cinzel", "Cormorant SC"
  * Modern → "Space Grotesk", "Sora", "Outfit", "Lexend"
  * Playful → "Caveat", "Satisfy", "Pacifico"
- COLOR HARMONY — Do NOT always use pure white #ffffff:
  * Warm image → cream (#F5E6CA, #FFF8F0) or gold (#D4A574)
  * Cool image → silver (#E0E8F0) or ice (#F0F4F8)
  * Dark/moody → soft white #ffffffDD with opacity 0.9
- Use text_shadow:true for gentle glow, NOT harsh shadows
- Do NOT add "hashtags" layers — hashtags belong in the caption
- Logo placement is handled separately by AI
- CTA only for promotional content, badge only if it adds value
- NEVER use the same layout for every post

IMPORTANT:
- Respond ONLY with a valid JSON array. No markdown, no explanation.
- "design_layers" must have at least 2 elements (headline + one more)
- "content_items" can be empty [] — design_layers replaces it as the primary layout system
- Do NOT put hashtags in design_layers

Example:
[{"title":"Kalpten Kalbe Destek","description":"Filistinli çocuklar için...","hashtags":"#destek #güç","image_suggestion_prompt":"A heartwarming scene of children playing in sunlight...","suggested_layout":"bottom_left","suggested_font":"Playfair Display","logo_position":"auto","content_items":[],"design_layers":[{"id":"headline","type":"headline","text":"Kalpten Kalbe Destek","x":10,"y":40,"max_width":55,"font_family":"Cormorant Garamond","font_size":40,"font_weight":700,"color":"#FFF8F0","text_align":"left","line_height":1.1,"opacity":0.95,"text_shadow":true,"letter_spacing":1},{"id":"desc","type":"description","text":"Filistinli çocuklar için kalbinizi birleştirin.","x":10,"y":75,"max_width":60,"font_family":"DM Sans","font_size":13,"font_weight":400,"color":"#ffffffBB","text_align":"left","line_height":1.4,"opacity":0.8}]}]`, count, topic, platform, campaignType, companyContext)
}

// ──────────────────────────────────────────────
// HTML'den düz metin çıkartan basit Scraper
// ──────────────────────────────────────────────
func scrapeWebsiteText(url string) string {
	if url == "" {
		return ""
	}
	if !strings.HasPrefix(url, "http") {
		url = "https://" + url
	}
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	html := string(bodyBytes)

	// Çok basit HTML temizliği — style ve script etiketlerini kaldır
	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	html = reStyle.ReplaceAllString(html, " ")
	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	html = reScript.ReplaceAllString(html, " ")
	reHTMLTags := regexp.MustCompile(`(?is)<[^>]+>`)
	html = reHTMLTags.ReplaceAllString(html, " ")
	reSpaces := regexp.MustCompile(`\s+`)
	text := reSpaces.ReplaceAllString(html, " ")

	if len(text) > 2000 {
		return text[:2000] // ilk 2000 karakter yeterli
	}
	return text
}

// ──────────────────────────────────────────────
// GenerateBrandDescription — Yalnızca marka adı
// ve web sitesinden yola çıkarak AI ile açıklama
// üretir.
// ──────────────────────────────────────────────

func GenerateBrandDescription(name, website, sector string) (string, error) {
	siteContent := scrapeWebsiteText(website)
	
	prompt := fmt.Sprintf(`Sen profesyonel bir metin yazarısın. 
Aşağıdaki bilgilere ve web sitesi içeriğine dayanarak marka için etkileyici, akılda kalıcı, profesyonel bir "Marka Açıklaması" (About Us / Brand Story) yaz. 

Marka Adı: %s
Sektör: %s
Web Sitesi: %s

[DİKKATE ALINACAK WEB SİTESİ İÇERİĞİ (Taranmış Ham Metin)]:
%s
[İÇERİK SONU]

KURALLAR:
1. Sadece marka açıklaması metnini yaz, ek açıklama (ör. "İşte metniniz:") yapma.
2. Dili kesinlikle Türkçe olmalı.
3. Web sitesinden şirketin ne yaptığına dair anahtar bilgileri çıkar.
4. 2-3 paragraf uzunluğunda, markanın vizyonunu, misyonunu ve neyi temsil ettiğini anlatan ikna edici bir ton kullan.
5. Markanın hedef kitlesine güven ve profesyonellik aşılamalı.`, name, sector, website, siteContent)

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		apiKey = "sk-proj-4M3pP0aI6g7V8-d_Z7JEXmG3s1d0_n4-TfR-U31hItd0Vv5X0T6K54wM7B1sD7h1s99wD-mKz_T3BlbkFJNw4QyvQx-h302W3H9e99X8C5y4G7V8-d_Z7JEXmG3s1d0_n4-TfR-U31hItd0Vv5X0T6K54wM" // temp fallback
	}

	reqBody := openAIRequest{
		Model:       "gpt-4o-mini",
		Temperature: 0.7,
		MaxTokens:   500,
		Messages: []openAIMessage{
			{Role: "system", Content: "Sen ödüllü bir reklam ajansı metin yazarısın."},
			{Role: "user", Content: prompt},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("request oluşturma hatası: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("API isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API hata kodu %d: %s", resp.StatusCode, string(respBody))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return "", fmt.Errorf("response parse hatası: %v", err)
	}

	if len(openAIResp.Choices) == 0 {
		return "", fmt.Errorf("API boş yanıt döndürdü")
	}

	return strings.TrimSpace(openAIResp.Choices[0].Message.Content), nil
}

// ──────────────────────────────────────────────
// GenerateDesignLayers — Her görsel üretiminde
// görselin stiline uygun katmanları yeniden tasarlar
// ──────────────────────────────────────────────

func GenerateDesignLayers(ctx BrandContext) ([]DesignLayer, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY tanımlı değil")
	}

	// Stil-spesifik tasarım yönergeleri — Organik & Gömülü Hissiyat
	styleGuide := map[string]string{
		"realistic": `REALISTIC PHOTO — CINEMATIC EDITORIAL STYLE:
- Think Vogue covers, Apple billboards, Netflix key art — text BELONGS to the image
- Typography must feel CARVED into the scene, not floating on top
- Use SCRIPT or SERIF fonts for emotional headlines: "Cormorant Garamond", "Playfair Display", "Libre Baskerville", "Crimson Text", "Spectral" — these feel warm and organic on photos
- Pair with ultra-clean sans-serif for body: "DM Sans", "Outfit", "Sora", "Manrope"
- Color strategy: Sample colors FROM the image mood. Warm photos → warm gold/cream text (#F5E6CA, #D4A574). Cool photos → silver/ice text (#E8EDF2, #C4D4E0). Dark photos → ethereal white with 80%% opacity
- Text shadow must be SUBTLE — soft diffused glow, NOT hard drop shadow
- Headline can curve or follow the image's natural flow lines
- Feel: Like the text was photographed alongside the subject`,
		"illustration": `ILLUSTRATION — ARTISTIC INTEGRATION STYLE:
- Text should feel like it was DRAWN by the same illustrator
- Use characterful fonts: "Caveat", "Satisfy", "Pacifico", "Patrick Hand", "Kalam" for playful; "Righteous", "Fredoka One" for bold fun
- Colors should pull directly from the illustration palette — match the art's vibe
- Text can overlap illustration elements slightly — creates depth and integration
- Use creative positioning: diagonal, curved around subjects, nestled in negative space
- Badge/labels should look like hand-drawn tags or stamps
- Feel: Storybook page, editorial illustration, zine aesthetic`,
		"3d-render": `3D RENDER — PREMIUM TECH AESTHETIC:
- Text should feel like a 3D element in the scene — volumetric, present
- Use geometric display fonts: "Space Grotesk", "Exo 2", "Orbitron", "Rajdhani", "Chakra Petch"
- Minimalist body fonts: "IBM Plex Sans", "Sora", "Inter"
- Glass-morphism text containers: rgba(255,255,255,0.08) bg with subtle border
- Neon accent colors for highlights: #00D4FF, #FF6B35, #A855F7
- Text positioned to interact with 3D space — beside objects, emerging from surfaces
- Feel: Product launch keynote, sci-fi title card, premium packaging`,
		"minimalist": `MINIMALIST — MUSEUM GALLERY AESTHETIC:
- Radical simplicity — maximum 2 text layers, no clutter
- Typography IS the design — use it as the primary visual element
- Elegant editorial fonts: "Cormorant Garamond" italic, "Abril Fatface", "Playfair Display SC"
- Generous white/negative space — text breathes
- Muted, desaturated color palette: charcoal (#2D2D2D), warm gray (#8B7E74), cream (#FAF7F2)
- Large headline that fills space intentionally — oversized is a design choice
- Asymmetric placement for visual tension
- Feel: Art gallery, Kinfolk magazine, Aesop packaging`,
	}

	guide, ok := styleGuide[ctx.Style]
	if !ok {
		guide = styleGuide["realistic"]
	}

	// Marka renkleri ipucu
	brandColorHint := ""
	if ctx.BrandColors != "" {
		brandColorHint = fmt.Sprintf("\nBrand Colors: %s — weave these into text accents, CTA elements, or badge backgrounds. Don't overuse — let them be accent touches.", ctx.BrandColors)
	}

	// Marka fontu ipucu
	brandFontHint := ""
	if ctx.BrandFont != "" {
		brandFontHint = fmt.Sprintf("\n\nBRAND FONT DIRECTIVE (HIGHEST PRIORITY): The brand uses \"%s\" as their primary font. You MUST use this font for the headline layer. Choose a complementary Google Font for description/body text that pairs well with it. This is a brand consistency requirement.", ctx.BrandFont)
	}

	prompt := BuildDesignLayerPrompt(ctx, guide, brandColorHint, brandFontHint)


	// 1) Önce Gemini dene
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey != "" {
		layers, err := generateDesignLayersWithGemini(geminiKey, prompt, ctx.Style)
		if err == nil {
			return resolveLayerCollisions(layers), nil
		}
		log.Printf("⚠️ Gemini katman üretme başarısız, OpenAI'ye geçiliyor: %v", err)
	}

	// 2) OpenAI fallback
	layers, err := generateDesignLayersWithOpenAI(apiKey, prompt, ctx.Style)
	if err != nil {
		return nil, fmt.Errorf("hem Gemini hem OpenAI katman üretme başarısız: %v", err)
	}
	return resolveLayerCollisions(layers), nil
}

// resolveLayerCollisions — AI'dan gelen katmanları sıralayıp
// üst üste binme varsa otomatik düzeltir
func resolveLayerCollisions(layers []DesignLayer) []DesignLayer {
	if len(layers) <= 1 {
		return layers
	}

	// ── Renk kontrastı düzelt ──
	for i := range layers {
		layers[i] = ensureColorContrast(layers[i])
	}

	// Tip önceliği: headline > description > cta > badge
	priority := map[string]int{
		"headline":    0,
		"description": 1,
		"cta":         2,
		"badge":       3,
	}

	// Y değerine göre sırala, aynı y ise öncelik sırasına göre
	sort.SliceStable(layers, func(i, j int) bool {
		if layers[i].Y == layers[j].Y {
			pi := priority[layers[i].Type]
			pj := priority[layers[j].Type]
			return pi < pj
		}
		return layers[i].Y < layers[j].Y
	})

	// ── Katman yükseklikleri hesapla ve çakışmaları çöz ──
	minGap := 5.0 // Minimum %5 dikey boşluk

	for i := 1; i < len(layers); i++ {
		prev := layers[i-1]
		curr := layers[i]

		// Önceki katmanın gerçek dikey alanını hesapla (metin sarma dahil)
		prevHeight := estimateLayerHeightWithWrap(prev)
		minY := prev.Y + prevHeight + minGap

		// Çakışma varsa düzelt
		if curr.Y < minY {
			log.Printf("🔧 Katman çakışma düzeltildi: '%s' (y:%.1f → y:%.1f) — '%s' ile çakışıyordu (height:%.1f)",
				curr.Type, curr.Y, minY, prev.Type, prevHeight)
			layers[i].Y = minY
		}
	}

	// ── Canvas dışına taşma kontrolü ──
	// Logo genelde alt köşede y=85-95 arasında olduğundan,
	// yazıların alt sınırı y=78'e düşürülüyor
	maxY := 78.0
	for i := len(layers) - 1; i >= 0; i-- {
		layerBottom := layers[i].Y + estimateLayerHeightWithWrap(layers[i])
		if layerBottom > maxY {
			// Layer'ı yukarı kaydır, bottom tam maxY'ye gelsin
			newY := maxY - estimateLayerHeightWithWrap(layers[i])
			if newY < 5 {
				newY = 5
			}
			layers[i].Y = newY
			// Hala taşıyorsa font küçült
			if layers[i].Y+estimateLayerHeightWithWrap(layers[i]) > maxY && layers[i].FontSize > 14 {
				layers[i].FontSize = int(float64(layers[i].FontSize) * 0.75)
			}
		}
	}

	// ── Son kontrol: aşağıdan yukarı tekrar çakışma taraması ──
	// Taşma düzeltmesi sonrası yukarıdaki katmanlarla çakışma olabilir
	for i := len(layers) - 2; i >= 0; i-- {
		below := layers[i+1]
		curr := layers[i]
		currHeight := estimateLayerHeightWithWrap(curr)
		if curr.Y+currHeight+minGap > below.Y {
			// Mevcut katmanı yukarı kaydır
			newY := below.Y - currHeight - minGap
			if newY < 2 {
				newY = 2
				// Sığmıyorsa font küçült
				if layers[i].FontSize > 14 {
					layers[i].FontSize = int(float64(layers[i].FontSize) * 0.7)
				}
			}
			layers[i].Y = newY
		}
	}

	// ── Proportional Spacing Enforcement ──
	// Katmanlar arasındaki boşluğu font boyutuna orantılı yap
	if len(layers) >= 2 {
		for i := 1; i < len(layers); i++ {
			prevFs := float64(layers[i-1].FontSize)
			// Gap = önceki katmanın font boyutunun 1.5 katı (canvas % olarak)
			proportionalGap := (prevFs * 1.5 / 600.0) * 100.0
			if proportionalGap < minGap {
				proportionalGap = minGap
			}
			prevHeight := estimateLayerHeightWithWrap(layers[i-1])
			idealY := layers[i-1].Y + prevHeight + proportionalGap
			if layers[i].Y < idealY {
				layers[i].Y = idealY
			}
		}
	}

	// ── X-Alignment Consistency ──
	// Tüm katmanların aynı x koordinatını ve text_align'ı kullanmasını sağla
	// (ilk katmanın x ve text_align'ı referans alınır)
	if len(layers) >= 2 {
		refX := layers[0].X
		refAlign := layers[0].TextAlign
		if refAlign == "" {
			refAlign = "left"
		}
		for i := 1; i < len(layers); i++ {
			// CTA ve badge hariç (bunlar farklı konumda olabilir)
			if layers[i].Type != "cta" && layers[i].Type != "badge" {
				if layers[i].X != refX {
					log.Printf("🔧 X hizalama düzeltildi: '%s' x:%.0f → x:%.0f", layers[i].Type, layers[i].X, refX)
					layers[i].X = refX
				}
				if layers[i].TextAlign != refAlign {
					layers[i].TextAlign = refAlign
				}
			}
		}
	}

	return layers
}

// estimateLayerHeightWithWrap — Metin sarma, font boyutu ve max_width
// dikkate alarak katmanın gerçek dikey alanını yüzde olarak hesaplar
func estimateLayerHeightWithWrap(layer DesignLayer) float64 {
	text := layer.Text
	fontSize := layer.FontSize
	maxWidth := layer.MaxWidth
	if maxWidth <= 0 {
		maxWidth = 60 // Varsayılan max_width
	}

	// Bir satıra yaklaşık kaç karakter sığar
	// Canvas genişliği 100% ise, maxWidth kadarını kullanıyoruz
	// Ortalama karakter genişliği ≈ fontSize * 0.55 (px cinsinden)
	// Canvas genişliği ≈ 600px varsayalım
	canvasWidthPx := 600.0
	availableWidthPx := canvasWidthPx * (maxWidth / 100.0)
	charWidthPx := float64(fontSize) * 0.55
	if charWidthPx < 1 {
		charWidthPx = 1
	}
	charsPerLine := int(availableWidthPx / charWidthPx)
	if charsPerLine < 5 {
		charsPerLine = 5
	}

	// Metin kaç satır tutar
	textLen := len([]rune(text))
	lines := (textLen + charsPerLine - 1) / charsPerLine
	if lines < 1 {
		lines = 1
	}

	// Her satırın yüksekliği (canvas yüzdesi olarak)
	// lineHeight ≈ fontSize * 1.3 (px) / canvasHeight (px) * 100
	canvasHeightPx := 600.0 // Kare canvas varsayımı
	lineHeightPx := float64(fontSize) * 1.35
	lineHeightPct := (lineHeightPx / canvasHeightPx) * 100.0

	totalHeight := lineHeightPct * float64(lines)

	// Padding varsa ekle
	if layer.Padding > 0 {
		paddingPct := (float64(layer.Padding) * 2.0 / canvasHeightPx) * 100.0
		totalHeight += paddingPct
	}

	// Minimum yükseklik
	if totalHeight < 4.0 {
		totalHeight = 4.0
	}

	return totalHeight
}

// ensureColorContrast — Katman renginin okunabilir olduğundan emin ol
func ensureColorContrast(layer DesignLayer) DesignLayer {
	// Text shadow'u headline ve description için daima aç
	if layer.Type == "headline" || layer.Type == "description" {
		layer.TextShadow = true
	}

	// CTA ve badge için arka plan rengi yoksa ekle
	if layer.Type == "cta" && layer.BgColor == "" {
		layer.BgColor = "rgba(0,0,0,0.6)"
		layer.BorderRadius = 24
		layer.Padding = 12
	}
	if layer.Type == "badge" && layer.BgColor == "" {
		layer.BgColor = "rgba(0,0,0,0.5)"
		layer.BorderRadius = 16
		layer.Padding = 8
	}

	return layer
}

func generateDesignLayersWithGemini(apiKey, prompt, style string) ([]DesignLayer, error) {
	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: "You are a legendary art director with 20 years of experience. You design text overlays that feel ORGANICALLY EMBEDDED into the image — as if the text was born inside the photograph. Output ONLY valid JSON arrays. ALL text content MUST be in TURKISH. Choose fonts with SOUL — match font personality to content mood. Use image-harmonizing colors instead of always pure white. Maximum 3 layers.\n\n" + prompt},
				},
			},
		},
	}

	jsonBody, _ := json.Marshal(reqBody)
	url := GeminiImageURL(apiKey)
	httpReq, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
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

	var layers []DesignLayer
	if err := json.Unmarshal([]byte(content), &layers); err != nil {
		return nil, fmt.Errorf("layer JSON parse: %v — raw: %s", err, content[:min(len(content), 200)])
	}

	log.Printf("✅ %d tasarım katmanı üretildi — Gemini (stil: %s)", len(layers), style)
	return layers, nil
}

func generateDesignLayersWithOpenAI(apiKey, prompt, style string) ([]DesignLayer, error) {
	reqBody := openAIRequest{
		Model:       "gpt-4o",
		Temperature: 0.9,
		MaxTokens:   1500,
		Messages: []openAIMessage{
			{Role: "system", Content: "You are a legendary art director with 20 years of experience at Pentagram and Apple Creative. You design text overlays that feel ORGANICALLY EMBEDDED into the image — as if the text was born inside the photograph. Output ONLY valid JSON arrays. ALL text MUST be in TURKISH. Choose fonts with SOUL — match font personality to content mood. Use image-harmonizing colors (warm cream, gold, silver) instead of always pure white. Maximum 3 layers. Less is more."},
			{Role: "user", Content: prompt},
		},
	}

	jsonBody, _ := json.Marshal(reqBody)
	httpReq, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
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

	var layers []DesignLayer
	if err := json.Unmarshal([]byte(content), &layers); err != nil {
		return nil, fmt.Errorf("layer JSON parse: %v — raw: %s", err, content[:min(len(content), 200)])
	}

	log.Printf("✅ %d tasarım katmanı üretildi — OpenAI fallback (stil: %s)", len(layers), style)
	return layers, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ──────────────────────────────────────────────
// OpenAI GPT-4o
// ──────────────────────────────────────────────

type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	MaxTokens   int             `json:"max_tokens"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func generateWithOpenAI(topic, platform, campaignType string, count int, companyContext string) ([]IdeaResult, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY ortam değişkeni tanımlı değil")
	}

	prompt := buildPrompt(topic, platform, campaignType, count, companyContext)

	reqBody := openAIRequest{
		Model: "gpt-4o",
		Messages: []openAIMessage{
			{Role: "system", Content: "You are a professional social media content strategist and visual designer. Always respond with valid JSON only. Never include markdown formatting."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.8,
		MaxTokens:   4096,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("request marshal hatası: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("request oluşturma hatası: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI API isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("response okuma hatası: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI API hata kodu %d: %s", resp.StatusCode, string(respBody))
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return nil, fmt.Errorf("OpenAI response parse hatası: %v", err)
	}

	if openAIResp.Error != nil {
		return nil, fmt.Errorf("OpenAI API hatası: %s", openAIResp.Error.Message)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI boş yanıt döndü")
	}

	content := openAIResp.Choices[0].Message.Content

	// JSON parse
	var ideas []IdeaResult
	if err := json.Unmarshal([]byte(content), &ideas); err != nil {
		// Bazen markdown code block içinde dönebilir, temizle
		cleaned := cleanJSONResponse(content)
		if err := json.Unmarshal([]byte(cleaned), &ideas); err != nil {
			return nil, fmt.Errorf("AI yanıtı JSON parse edilemedi: %v\nYanıt: %s", err, content)
		}
	}

	// Validate & normalize layout values
	for i := range ideas {
		ideas[i].SuggestedLayout = normalizeLayout(ideas[i].SuggestedLayout)
	}

	log.Printf("✅ OpenAI: %d fikir başarıyla üretildi", len(ideas))
	return ideas, nil
}

// ──────────────────────────────────────────────
// Gemini API (Fallback)
// ──────────────────────────────────────────────

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func generateWithGemini(topic, platform, campaignType string, count int, companyContext string) ([]IdeaResult, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY ortam değişkeni tanımlı değil")
	}

	prompt := buildPrompt(topic, platform, campaignType, count, companyContext)

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: prompt},
				},
			},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("request marshal hatası: %v", err)
	}

	url := GeminiImageURL(apiKey)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("request oluşturma hatası: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Gemini API isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("response okuma hatası: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini API hata kodu %d: %s", resp.StatusCode, string(respBody))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("Gemini response parse hatası: %v", err)
	}

	if geminiResp.Error != nil {
		return nil, fmt.Errorf("Gemini API hatası: %s", geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("Gemini boş yanıt döndü")
	}

	content := geminiResp.Candidates[0].Content.Parts[0].Text

	// JSON parse
	var ideas []IdeaResult
	if err := json.Unmarshal([]byte(content), &ideas); err != nil {
		cleaned := cleanJSONResponse(content)
		if err := json.Unmarshal([]byte(cleaned), &ideas); err != nil {
			return nil, fmt.Errorf("Gemini yanıtı JSON parse edilemedi: %v\nYanıt: %s", err, content)
		}
	}

	// Validate & normalize layout values
	for i := range ideas {
		ideas[i].SuggestedLayout = normalizeLayout(ideas[i].SuggestedLayout)
	}

	log.Printf("✅ Gemini (fallback): %d fikir başarıyla üretildi", len(ideas))
	return ideas, nil
}

// ──────────────────────────────────────────────
// Yardımcı: JSON response temizleyici
// ──────────────────────────────────────────────

func cleanJSONResponse(raw string) string {
	// ```json ... ``` blokunu temizle
	result := raw
	for _, prefix := range []string{"```json\n", "```json", "```\n", "```"} {
		if len(result) > len(prefix) && result[:len(prefix)] == prefix {
			result = result[len(prefix):]
			break
		}
	}
	// Sondaki ``` temizle
	if len(result) > 3 && result[len(result)-3:] == "```" {
		result = result[:len(result)-3]
	}
	// Sondaki whitespace/newline temizle
	for len(result) > 0 && (result[len(result)-1] == '\n' || result[len(result)-1] == '\r' || result[len(result)-1] == ' ') {
		result = result[:len(result)-1]
	}
	return result
}

// ──────────────────────────────────────────────
// Yardımcı: Layout normalizasyonu
// ──────────────────────────────────────────────

var validLayouts = map[string]bool{
	"left_top":      true,
	"left_bottom":   true,
	"right_top":     true,
	"right_bottom":  true,
	"center_bottom": true,
}

func normalizeLayout(layout string) string {
	layout = strings.TrimSpace(strings.ToLower(layout))
	if validLayouts[layout] {
		return layout
	}
	return "left_bottom" // varsayılan
}

// ──────────────────────────────────────────────
// Profesyonel AI Görsel Üretme Sistemi
// Marka kimliği + post verileri → ajans kalitesinde prompt
// ──────────────────────────────────────────────

// BrandContext — Görsel üretimi için tüm marka/post verileri
type BrandContext struct {
	// Company (marka kimliği)
	BrandName       string
	Sector          string
	TargetAudience  string
	BrandColors     string // JSON array: ["#e6272f", "#ffffff"]
	ToneOfVoice     string // e.g. "profesyonel", "samimi", "lüks"
	LogoURL         string
	Slogan          string
	Description     string
	BrandFont       string // Primary font from brand profile
	// Post (içerik)
	Platform        string // "Instagram", "LinkedIn", etc.
	Headline        string // post.Title
	SubText         string // post.Content
	ImagePrompt     string // AI'ın ürettiği veya kullanıcının girdiği prompt
	ContentItems    []ContentItem
	// User selections (frontend)
	Style           string // "realistic", "illustration", "3d-render", "minimalist"
	TextPosition    string // "bottom_left", "center", etc.
}

// Stil → görsel stil açıklaması mapping
var visualStyleMap = map[string]string{
	"realistic":    "photorealistic, professional photography, 8K quality, studio lighting, sharp focus, cinematic depth of field",
	"illustration": "vibrant digital illustration, modern vector art, flat design, bold colors, clean lines, editorial style",
	"3d-render":    "cinematic 3D render, octane quality, volumetric lighting, realistic materials, dramatic shadows, depth",
	"minimalist":   "minimalist, ultra-clean, limited color palette, geometric forms, generous negative space, premium aesthetic",
}

// Stil → ton/mood mapping
var styleToMood = map[string]string{
	"realistic":    "professional, modern, trustworthy",
	"illustration": "creative, playful, energetic",
	"3d-render":    "futuristic, premium, innovative",
	"minimalist":   "elegant, clean, sophisticated",
}

// BuildProfessionalImagePrompt — Ajans kalitesinde, metin konumu farkında prompt
func BuildProfessionalImagePrompt(ctx BrandContext) string {
	// Görsel stili
	visualStyle, ok := visualStyleMap[ctx.Style]
	if !ok {
		visualStyle = visualStyleMap["realistic"]
	}

	// Ton/Mood
	mood := styleToMood[ctx.Style]
	if ctx.ToneOfVoice != "" {
		mood = ctx.ToneOfVoice + ", " + mood
	}

	// Renk paleti — marka renkleriyle uyumlu
	colorPalette := "sophisticated, premium corporate colors with harmonious tones"
	if ctx.BrandColors != "" {
		colorPalette = fmt.Sprintf(
			"BRAND COLOR PALETTE: %s — integrate these colors into the overall visual composition. "+
				"The dominant visual tones, lighting gels, environmental colors, and accent elements should harmonize with this palette. "+
				"Do NOT just use these colors as flat fills — weave them naturally through lighting, objects, backgrounds, and atmosphere.",
			ctx.BrandColors,
		)
	}

	// Platform boyut ipucu
	platformHint := "square 1:1 social media post"
	switch ctx.Platform {
	case "Instagram":
		platformHint = "Instagram square post (1080×1080)"
	case "LinkedIn":
		platformHint = "LinkedIn professional post (1200×627 adapted to square)"
	case "Facebook":
		platformHint = "Facebook feed post (1080×1080)"
	case "Twitter", "X":
		platformHint = "Twitter/X post (1200×675 adapted to square)"
	}

	// Konu/tema açıklaması
	themeDesc := ctx.ImagePrompt
	if ctx.Headline != "" && ctx.ImagePrompt != ctx.Headline {
		themeDesc = fmt.Sprintf("%s. The visual should complement the headline: \"%s\"", ctx.ImagePrompt, ctx.Headline)
	}

	// Text overlay zone awareness — metin nerede duracak?
	textZoneHint := ""
	pos := strings.ToLower(ctx.TextPosition)
	switch {
	case strings.Contains(pos, "bottom"):
		textZoneHint = "\n- TEXT OVERLAY ZONE: Text will be overlaid at the BOTTOM of the image. Ensure the bottom 35% has natural darker tones, deeper colors, or atmospheric depth that supports white text readability. Keep brighter, detailed elements toward the upper portion."
	case strings.Contains(pos, "top"):
		textZoneHint = "\n- TEXT OVERLAY ZONE: Text will be overlaid at the TOP of the image. Ensure the top 35% has natural darker tones or softer colors that support text readability. Keep the most detailed/bright visual elements in the lower portion."
	case strings.Contains(pos, "left"):
		textZoneHint = "\n- TEXT OVERLAY ZONE: Text will be overlaid on the LEFT side. Ensure the left 40% has naturally darker or calmer tones for text readability. Position key visual interest toward the right side."
	case strings.Contains(pos, "right"):
		textZoneHint = "\n- TEXT OVERLAY ZONE: Text will be overlaid on the RIGHT side. Ensure the right 40% has naturally darker or calmer tones for text readability. Position key visual interest toward the left side."
	case pos == "center":
		textZoneHint = "\n- TEXT OVERLAY ZONE: Text will be centered on the image. Create a composition where the center area has natural depth or atmospheric quality that supports text overlay. Distribute detailed elements around the periphery."
	}

	if ctx.TextPosition != "" {
		parts := strings.Split(ctx.TextPosition, "|")
		textPos := strings.ReplaceAll(parts[0], "_", " ")
		logoPos := ""
		if len(parts) > 1 {
			logoPos = " and " + strings.ReplaceAll(parts[1], "_", " ")
		}
		
		textZoneHint += fmt.Sprintf("\n- Spatial Design Rule: You MUST leave the %s%s area extremely clean, uncluttered, subtly darker, and completely free of any complex subject matter. This negative space is CRITICAL because a logo and promotional text will be placed exactly there.", textPos, logoPos)
	}

	// Sektör ipucu
	sectorHint := ""
	if ctx.Sector != "" {
		sectorHint = fmt.Sprintf("\n- Industry: %s — authentically reflect this industry's visual language, props, settings, and conventions.", ctx.Sector)
	}

	// Hedef kitle ipucu
	audienceHint := ""
	if ctx.TargetAudience != "" {
		audienceHint = fmt.Sprintf("\n- Target Audience: %s — the visual style, energy, and subjects should resonate with this demographic.", ctx.TargetAudience)
	}

	// Marka adı ipucu
	brandHint := ""
	if ctx.BrandName != "" {
		brandHint = fmt.Sprintf("\n- Brand: \"%s\" — the visual should feel aligned with a premium, established brand.", ctx.BrandName)
	}

	// Slogan ipucu
	sloganHint := ""
	if ctx.Slogan != "" {
		sloganHint = fmt.Sprintf("\n- Brand Essence: \"%s\" — let this spirit subtly influence the mood.", ctx.Slogan)
	}

	// Açıklama ipucu — görsel bağlam
	descHint := ""
	if ctx.Description != "" {
		descHint = fmt.Sprintf("\n- Brand DNA: %s", ctx.Description)
	}

	prompt := fmt.Sprintf(
		`You are a top-tier social media designer at a premium agency creating a scroll-stopping visual.

DESIGN BRIEF:
- Format: %s
- Visual Style: %s
- Tone & Mood: %s
- Color Direction: %s%s%s%s%s%s%s

SUBJECT / THEME:
%s

COMPOSITION & QUALITY REQUIREMENTS:
- Fill the ENTIRE canvas edge-to-edge with rich visual content — ZERO blank areas, ZERO empty zones, ZERO white space margins.
- Create professional visual hierarchy with depth, layers, and premium lighting.
- Add subtle premium details: natural shadows, reflections, bokeh, ambient lighting, or texture variations.
- Make it scroll-stopping and instantly engaging — top creative agency quality.
- The image will serve as a background for CSS text overlay — ensure contrast in text zones.

ABSOLUTE ZERO-TEXT RULE (NON-NEGOTIABLE):
- Do NOT include ANY text, letters, words, numbers, logos, watermarks, signs, labels, or typography.
- Do NOT render the brand name, headline, dates, URLs, or any writing.
- Represent content ONLY through visual metaphors and imagery.
- The output must be a PURE VISUAL with zero readable characters.`,
		platformHint, visualStyle, mood, colorPalette,
		brandHint, sectorHint, audienceHint, sloganHint, descHint, textZoneHint,
		themeDesc,
	)

	return prompt
}

// ── Gemini Flash Image API tipleri (generateContent) ────────────────

type geminiImagePart struct {
	Text string `json:"text,omitempty"`
}

type geminiImageContent struct {
	Parts []geminiImagePart `json:"parts"`
}

type geminiImageGenConfig struct {
	ResponseModalities []string `json:"responseModalities"`
}

type geminiImageRequest struct {
	Contents         []geminiImageContent `json:"contents"`
	GenerationConfig geminiImageGenConfig `json:"generationConfig"`
}

type geminiImageResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text       string `json:"text,omitempty"`
				InlineData *struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// GenerateImage — Basit mod (geriye uyumluluk)
func GenerateImage(prompt string, style string) (string, error) {
	ctx := BrandContext{
		ImagePrompt: prompt,
		Style:       style,
	}
	fullPrompt := BuildProfessionalImagePrompt(ctx)
	return generateImageWithFallback(fullPrompt, style)
}

// GenerateBrandImage — Tam marka bağlamlı profesyonel görsel üretimi
func GenerateBrandImage(ctx BrandContext) (string, error) {
	fullPrompt := BuildProfessionalImagePrompt(ctx)
	log.Printf("🎨 Profesyonel prompt oluşturuldu — Marka: %s, Stil: %s, Platform: %s", ctx.BrandName, ctx.Style, ctx.Platform)
	return generateImageWithFallback(fullPrompt, ctx.Style)
}

// ──────────────────────────────────────────────
// Doğrudan Provider Çağrıları (Exported)
// Paralel üretim sistemi tarafından kullanılır
// ──────────────────────────────────────────────

// CallGeminiImageDirect — Gemini Flash Image API'yi doğrudan çağırır
func CallGeminiImageDirect(apiKey, prompt string) (string, error) {
	return callGeminiImage(apiKey, prompt, "realistic")
}

// CallDALLEDirect — OpenAI DALL-E 3 API'yi doğrudan çağırır
func CallDALLEDirect(apiKey, prompt string) (string, error) {
	return callDALLE(apiKey, prompt, "realistic")
}

// ──────────────────────────────────────────────
// Multi-Provider Fallback: BFL (Flux) -> Gemini -> DALL-E
// ──────────────────────────────────────────────

func generateImageWithFallback(prompt, style string) (string, error) {
	// 1) Black Forest Labs (BFL) Flux Models Fallback
	bflKey := os.Getenv("BFL_API_KEY")
	if bflKey != "" {
		fluxModels := []string{"flux-pro-1.1", "flux-pro", "flux-dev", "flux-schnell"}
		for _, model := range fluxModels {
			log.Printf("🚀 BFL deneniyor (Model: %s)", model)
			result, err := callBFL(bflKey, model, prompt)
			if err == nil {
				return result, nil
			}
			log.Printf("⚠️ BFL Model '%s' başarısız, bir sonraki modele geçiliyor: %v", model, err)
		}
		log.Printf("⚠️ Tüm BFL (Flux) modelleri başarısız oldu. Gemini/DALL-E'ye geçiliyor.")
	}

	// 2) Gemini Flash Image dene
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey != "" {
		result, err := callGeminiImage(geminiKey, prompt, style)
		if err == nil {
			return result, nil
		}
		log.Printf("⚠️ Gemini görsel üretme başarısız, DALL-E'ye geçiliyor: %v", err)
	}

	// 3) DALL-E 3 fallback
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey != "" {
		result, err := callDALLE(openaiKey, prompt, style)
		if err == nil {
			return result, nil
		}
		log.Printf("❌ DALL-E de başarısız oldu: %v", err)
		return "", fmt.Errorf("görsel üretme başarısız (BFL + Gemini + DALL-E): %v", err)
	}

	return "", fmt.Errorf("görsel üretme için API anahtarı bulunamadı (BFL_API_KEY, GEMINI_API_KEY veya OPENAI_API_KEY gerekli)")
}

// ──────────────────────────────────────────────
// Provider 0: Black Forest Labs (Flux API Async Polling)
// ──────────────────────────────────────────────

type bflRequest struct {
	Prompt string `json:"prompt"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type bflSubmitResponse struct {
	ID string `json:"id"`
}

type bflStatusResponse struct {
	Status string `json:"status"`
	Result struct {
		Sample string `json:"sample"`
	} `json:"result"`
}

func callBFL(apiKey, model, prompt string) (string, error) {
	reqBody := bflRequest{
		Prompt: prompt,
		Width:  1024,
		Height: 1024,
	}
	bodyBytes, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("https://api.bfl.ml/v1/%s", model)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Key", apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respData, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("BFL API submit error %d: %s", resp.StatusCode, string(respData))
	}

	var submitResp bflSubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return "", err
	}

	taskID := submitResp.ID
	if taskID == "" {
		return "", fmt.Errorf("BFL empty task ID")
	}

	// 2) Poll for result (Async)
	pollURL := fmt.Sprintf("https://api.bfl.ml/v1/get_result?id=%s", taskID)
	
	maxAttempts := 15
	for attempts := 0; attempts < maxAttempts; attempts++ {
		time.Sleep(3 * time.Second)
		pReq, _ := http.NewRequest("GET", pollURL, nil)
		pReq.Header.Set("X-Key", apiKey)
		pResp, err := client.Do(pReq)
		if err != nil {
			continue // try again
		}
		
		var sResp bflStatusResponse
		if err := json.NewDecoder(pResp.Body).Decode(&sResp); err == nil {
			pResp.Body.Close()
			if sResp.Status == "Ready" && sResp.Result.Sample != "" {
				// download and save image locally
				localURL, err := downloadAndSaveImage(sResp.Result.Sample)
				if err != nil {
					return sResp.Result.Sample, nil // fallback to remote url
				}
				log.Printf("✅ Görsel üretildi (motor: BFL %s)", model)
				return localURL, nil
			} else if sResp.Status == "Failed" || sResp.Status == "Error" {
				return "", fmt.Errorf("BFL task failed: %s", sResp.Status)
			}
		} else {
			pResp.Body.Close()
		}
	}

	return "", fmt.Errorf("BFL polling timeout")
}

// ──────────────────────────────────────────────
// Provider 1: Gemini Flash Image (generateContent)
// ──────────────────────────────────────────────

func callGeminiImage(apiKey, prompt, style string) (string, error) {
	reqBody := geminiImageRequest{
		Contents: []geminiImageContent{
			{
				Parts: []geminiImagePart{
					{Text: "Generate an image based on this description. Output ONLY the image, no explanation needed. " + prompt},
				},
			},
		},
		GenerationConfig: geminiImageGenConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("request marshal hatası: %v", err)
	}

	url := GeminiImageURL(apiKey)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("request oluşturma hatası: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Gemini API isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("response okuma hatası: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini API hata %d: %s", resp.StatusCode, string(respBody))
	}

	var geminiResp geminiImageResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return "", fmt.Errorf("response parse hatası: %v", err)
	}

	if geminiResp.Error != nil {
		return "", fmt.Errorf("Gemini hatası: %s", geminiResp.Error.Message)
	}

	if len(geminiResp.Candidates) == 0 {
		return "", fmt.Errorf("Gemini boş yanıt döndü")
	}

	// Response parts içinde inlineData olan ilk part'ı bul
	var imageBase64 string
	for _, part := range geminiResp.Candidates[0].Content.Parts {
		if part.InlineData != nil && part.InlineData.Data != "" {
			imageBase64 = part.InlineData.Data
			break
		}
	}

	if imageBase64 == "" {
		return "", fmt.Errorf("Gemini yanıtında görsel verisi bulunamadı")
	}

	imageBytes, err := base64DecodeImage(imageBase64)
	if err != nil {
		return "", fmt.Errorf("base64 decode hatası: %v", err)
	}

	localURL, err := saveImageBytes(imageBytes)
	if err != nil {
		return "", fmt.Errorf("görsel kaydetme hatası: %v", err)
	}

	log.Printf("✅ Görsel üretildi (stil: %s, motor: Gemini Flash Image)", style)
	return localURL, nil
}

// ──────────────────────────────────────────────
// Provider 2: OpenAI DALL-E 3 (fallback)
// ──────────────────────────────────────────────

type dalleRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	N      int    `json:"n"`
	Size   string `json:"size"`
}

type dalleResponse struct {
	Data []struct {
		URL string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func callDALLE(apiKey, prompt, style string) (string, error) {
	reqBody := dalleRequest{
		Model:  "dall-e-3",
		Prompt: prompt,
		N:      1,
		Size:   "1024x1024",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("request marshal hatası: %v", err)
	}

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/images/generations", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("request oluşturma hatası: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("DALL-E API isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("response okuma hatası: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("DALL-E API hata %d: %s", resp.StatusCode, string(respBody))
	}

	var dalleResp dalleResponse
	if err := json.Unmarshal(respBody, &dalleResp); err != nil {
		return "", fmt.Errorf("response parse hatası: %v", err)
	}

	if dalleResp.Error != nil {
		return "", fmt.Errorf("DALL-E hatası: %s", dalleResp.Error.Message)
	}

	if len(dalleResp.Data) == 0 {
		return "", fmt.Errorf("DALL-E boş yanıt döndü")
	}

	// DALL-E URL döndürür — indir ve lokale kaydet
	imageURL := dalleResp.Data[0].URL
	localURL, err := downloadAndSaveImage(imageURL)
	if err != nil {
		log.Printf("⚠️ DALL-E görseli indirilemedi, harici URL kullanılıyor: %v", err)
		return imageURL, nil // Harici URL fallback
	}

	log.Printf("✅ Görsel üretildi (stil: %s, motor: DALL-E 3)", style)
	return localURL, nil
}

// downloadAndSaveImage — harici URL'den görseli indir, diske kaydet
func downloadAndSaveImage(imageURL string) (string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("görsel indirme hatası: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("görsel indirme hata kodu: %d", resp.StatusCode)
	}

	imageBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("görsel okuma hatası: %v", err)
	}

	return saveImageBytes(imageBytes)
}

// ──────────────────────────────────────────────
// Ortak yardımcılar
// ──────────────────────────────────────────────

// base64DecodeImage — base64 kodlanmış görsel verisini byte dizisine çevir
func base64DecodeImage(encoded string) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Bazen padding eksik olabilir, raw encoding dene
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("base64 decode edilemedi: %v", err)
		}
	}
	return decoded, nil
}

// saveImageBytes — byte dizisindeki görseli Bunny CDN'e veya diske kaydet, public URL döndür
func saveImageBytes(imageBytes []byte) (string, error) {
	fileName := fmt.Sprintf("ai_%d.png", time.Now().UnixNano())

	// Bunny CDN yapılandırılmışsa → CDN'e yükle
	if IsBunnyCDNConfigured() {
		cdnURL, err := UploadToBunnyCDN(imageBytes, "posts", fileName)
		if err != nil {
			log.Printf("⚠️ Bunny CDN upload başarısız, lokale kaydediliyor: %v", err)
		} else {
			log.Printf("✅ Görsel Bunny CDN'e kaydedildi: %s", cdnURL)
			return cdnURL, nil
		}
	}

	// Fallback: lokale kaydet
	uploadDir := "./assets/uploads/posts"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", fmt.Errorf("klasör oluşturma hatası: %v", err)
	}

	filePath := uploadDir + "/" + fileName
	if err := os.WriteFile(filePath, imageBytes, 0644); err != nil {
		return "", fmt.Errorf("dosya yazma hatası: %v", err)
	}

	publicURL := "/uploads/posts/" + fileName
	log.Printf("✅ Görsel lokal diske kaydedildi: %s", publicURL)
	return publicURL, nil
}


// ──────────────────────────────────────────────
// Logo Pozisyon Analizi — Vision API ile görseli
// analiz edip en uygun logo konumunu belirler
// ──────────────────────────────────────────────

// LogoPositionResult — AI analizi sonucu optimal logo konumu
type LogoPositionResult struct {
	X         float64 `json:"x"`          // % from left (0-100)
	Y         float64 `json:"y"`          // % from top (0-100)
	MaxWidth  float64 `json:"max_width"`  // logo max width as % (5-20)
	MaxHeight float64 `json:"max_height"` // logo max height as % (5-20)
	Reasoning string  `json:"reasoning"`  // AI'ın açıklaması
}

// AnalyzeImageForLogoPosition — Üretilen görseli GPT-4o Vision ile analiz ederek
// logo için en uygun konumu belirler. Sabit köşe yerine, görselin içeriğine göre
// dinamik pozisyon döndürür.
func AnalyzeImageForLogoPosition(imageLocalPath string, brandName string) (*LogoPositionResult, error) {
	var imageBytes []byte
	var err error

	if strings.HasPrefix(imageLocalPath, "http://") || strings.HasPrefix(imageLocalPath, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, dlErr := client.Get(imageLocalPath)
		if dlErr != nil {
			return nil, fmt.Errorf("görsel indirilemedi: %v", dlErr)
		}
		defer resp.Body.Close()
		imageBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("görsel okunamadı: %v", err)
		}
	} else {
		imagePath := "." + imageLocalPath
		imageBytes, err = os.ReadFile(imagePath)
		if err != nil {
			return nil, fmt.Errorf("görsel okunamadı: %v", err)
		}
	}

	imageBase64 := base64.StdEncoding.EncodeToString(imageBytes)

	logoPrompt := fmt.Sprintf(`You are an expert visual designer analyzing an image for optimal LOGO placement.

The brand "%s" logo needs to be placed on this image. Analyze the image and determine the BEST position for a small logo overlay.

ANALYSIS CRITERIA:
1. Find areas with LOW visual complexity (empty/calm zones, solid colors, gradients)
2. Avoid placing logo on faces, key subjects, text, or high-detail areas
3. Consider contrast — logo should be visible (prefer darker or lighter uniform areas)
4. Check ALL four corners and edges, not just bottom-left
5. Consider the overall composition and visual balance
6. The logo should enhance the design, not obstruct important content
7. Prefer positions where the logo won't compete with other design elements

RESPOND WITH ONLY a valid JSON object (no markdown):
{
  "x": <number 0-100, percentage from left>,
  "y": <number 0-100, percentage from top>,
  "max_width": <number 5-15, logo width as percentage>,
  "max_height": <number 5-15, logo height as percentage>,
  "reasoning": "<brief explanation of why this position was chosen>"
}

IMPORTANT: Do NOT default to bottom-left. Genuinely analyze the image content and pick the truly optimal position. The position can be ANY x,y coordinate that works best.`, brandName)

	// 1) Önce Gemini Vision dene
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey != "" {
		result, err := analyzeLogoWithGemini(geminiKey, imageBase64, logoPrompt)
		if err == nil {
			return result, nil
		}
		log.Printf("⚠️ Gemini Vision logo analizi başarısız, OpenAI'ye geçiliyor: %v", err)
	}

	// 2) OpenAI Vision fallback
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey != "" {
		result, err := analyzeLogoWithOpenAI(openaiKey, imageBase64, logoPrompt)
		if err == nil {
			return result, nil
		}
		log.Printf("❌ OpenAI Vision da başarısız: %v", err)
		return nil, fmt.Errorf("hem Gemini hem OpenAI logo analizi başarısız: %v", err)
	}

	return nil, fmt.Errorf("logo analizi için API anahtarı bulunamadı")
}

func parseLogoResult(content string) (*LogoPositionResult, error) {
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result LogoPositionResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("logo pozisyon JSON parse: %v — raw: %s", err, content[:min(len(content), 200)])
	}

	// Validate ranges
	if result.X < 0 || result.X > 100 {
		result.X = 85
	}
	if result.Y < 0 || result.Y > 100 {
		result.Y = 85
	}
	if result.MaxWidth < 3 || result.MaxWidth > 25 {
		result.MaxWidth = 10
	}
	if result.MaxHeight < 3 || result.MaxHeight > 25 {
		result.MaxHeight = 10
	}

	log.Printf("🎯 Logo pozisyon analizi tamamlandı: x=%.1f%%, y=%.1f%%, w=%.1f%%, h=%.1f%% — %s",
		result.X, result.Y, result.MaxWidth, result.MaxHeight, result.Reasoning)

	return &result, nil
}

// analyzeLogoWithGemini — Gemini Vision ile logo pozisyon analizi
func analyzeLogoWithGemini(apiKey, imageBase64, prompt string) (*LogoPositionResult, error) {
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

	jsonBody, _ := json.Marshal(reqBody)
	url := GeminiImageURL(apiKey)
	httpReq, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Gemini Vision isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini Vision hatası %d: %s", resp.StatusCode, string(body))
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("yanıt parse hatası: %v", err)
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("boş yanıt")
	}

	content := strings.TrimSpace(geminiResp.Candidates[0].Content.Parts[0].Text)
	log.Printf("✅ Logo pozisyon analizi — Gemini Vision")
	return parseLogoResult(content)
}

// analyzeLogoWithOpenAI — OpenAI Vision fallback
func analyzeLogoWithOpenAI(apiKey, imageBase64, prompt string) (*LogoPositionResult, error) {
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
			{Role: "system", Content: json.RawMessage(`"You are a professional graphic designer. Always respond with valid JSON only."`)},
			{Role: "user", Content: json.RawMessage(contentJSON)},
		},
		MaxTokens: 300, Temperature: 0.3,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI Vision isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI Vision hatası %d: %s", resp.StatusCode, string(body))
	}

	var oaiResp openAIResponse
	if err := json.Unmarshal(body, &oaiResp); err != nil {
		return nil, fmt.Errorf("yanıt parse hatası: %v", err)
	}
	if len(oaiResp.Choices) == 0 {
		return nil, fmt.Errorf("boş yanıt")
	}

	content := strings.TrimSpace(oaiResp.Choices[0].Message.Content)
	log.Printf("✅ Logo pozisyon analizi — OpenAI Vision fallback")
	return parseLogoResult(content)
}

// ──────────────────────────────────────────────
// Metin Yerleşim Analizi — Vision API ile üretilen
// görseli analiz edip metinler için en uygun
// koordinatları belirler (üst üste binmeyi önler)
// ──────────────────────────────────────────────

// AnalyzeImageForTextPlacement — Üretilen görseli AI Vision ile analiz ederek
// metin katmanları (headline, description, CTA) için en uygun konumları belirler.
// Bu sayede metinler görseldeki ana konunun üzerine binmez.
func AnalyzeImageForTextPlacement(imageLocalPath string, ctx BrandContext) ([]DesignLayer, error) {
	var imageBytes []byte
	var err error

	if strings.HasPrefix(imageLocalPath, "http://") || strings.HasPrefix(imageLocalPath, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, dlErr := client.Get(imageLocalPath)
		if dlErr != nil {
			return nil, fmt.Errorf("görsel indirilemedi: %v", dlErr)
		}
		defer resp.Body.Close()
		imageBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("görsel okunamadı: %v", err)
		}
	} else {
		imagePath := "." + imageLocalPath
		imageBytes, err = os.ReadFile(imagePath)
		if err != nil {
			return nil, fmt.Errorf("görsel okunamadı: %v", err)
		}
	}

	imageBase64 := base64.StdEncoding.EncodeToString(imageBytes)

	// Marka renkleri ipucu
	brandColorHint := ""
	if ctx.BrandColors != "" {
		brandColorHint = fmt.Sprintf("\nBrand Colors: %s — use these as accent touches in text colors or badge backgrounds.", ctx.BrandColors)
	}

	// Marka fontu ipucu
	brandFontHint := ""
	if ctx.BrandFont != "" {
		brandFontHint = fmt.Sprintf("\nBrand Font: \"%s\" — MUST use this for the headline layer.", ctx.BrandFont)
	}

	prompt := fmt.Sprintf(`You are a legendary art director analyzing a REAL generated image to determine the OPTIMAL text placement.

CRITICAL TASK: Look at this image and determine WHERE text should go so it does NOT overlap with the main subject.

YOUR ANALYSIS PROCESS:
1. IDENTIFY the main subject(s) — What is the focal point? Where are the people/products/objects?
2. FIND empty/calm zones — Where is there negative space, blurred background, solid color areas, gradient areas?
3. DETERMINE the best text position — Place text in the EMPTIEST, most readable area
4. ENSURE contrast — Text must be readable against the background in that zone
5. AVOID the subject — Text MUST NOT cover faces, products, key visual elements

CONTENT TO PLACE:
- Headline: "%s"
- Description: "%s"
- Platform: %s
- Brand: %s%s%s

PLACEMENT RULES:
- Headline and Description MUST be in DIFFERENT vertical zones (minimum 15%% apart)
- Text should be placed where the image has DARK or UNIFORM areas for readability
- If the subject is centered → place text at TOP or BOTTOM
- If the subject is left → place text at RIGHT
- If the subject is right → place text at LEFT  
- If the subject is everywhere → place text at BOTTOM with a gradient overlay area
- Maximum 3 layers total
- All text MUST be in TURKISH
- ALL text layers (except CTA/badge) must share the SAME x coordinate for clean alignment
- No layer should have y > 75 (reserve y=78-100 for logo)

RESPOND WITH ONLY a valid JSON array of design layers:
[
  {
    "id": "headline",
    "type": "headline",
    "text": "headline text in TURKISH",
    "x": <number 0-100>,
    "y": <number 0-75>,
    "max_width": <number 40-90>,
    "font_family": "Google Font name",
    "font_size": <28-42>,
    "font_weight": <400|600|700|800>,
    "color": "#hex",
    "text_align": "left"|"center"|"right",
    "line_height": 1.1,
    "opacity": 0.95,
    "text_shadow": true,
    "letter_spacing": 0.5
  },
  {
    "id": "desc",
    "type": "description", 
    "text": "short description in TURKISH (max 15 words)",
    "x": <same x as headline>,
    "y": <at least 20 apart from headline y>,
    "max_width": <40-75>,
    "font_family": "readable sans-serif font",
    "font_size": <12-16>,
    "font_weight": 400,
    "color": "#hex with slight transparency",
    "text_align": "same as headline",
    "line_height": 1.4,
    "opacity": 0.85,
    "text_shadow": true
  }
]

IMPORTANT: Base your positions on what you SEE in the actual image. Do NOT use default positions blindly.`, ctx.Headline, ctx.SubText, ctx.Platform, ctx.BrandName, brandColorHint, brandFontHint)

	// 1) Önce Gemini Vision dene
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey != "" {
		layers, err := analyzeTextPlacementWithGemini(geminiKey, imageBase64, prompt)
		if err == nil {
			// Post-process: çakışma düzelt
			return resolveLayerCollisions(layers), nil
		}
		log.Printf("⚠️ Gemini Vision metin yerleşim analizi başarısız, OpenAI'ye geçiliyor: %v", err)
	}

	// 2) OpenAI Vision fallback
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey != "" {
		layers, err := analyzeTextPlacementWithOpenAI(openaiKey, imageBase64, prompt)
		if err == nil {
			return resolveLayerCollisions(layers), nil
		}
		log.Printf("❌ OpenAI Vision da başarısız: %v", err)
		return nil, fmt.Errorf("hem Gemini hem OpenAI metin yerleşim analizi başarısız: %v", err)
	}

	return nil, fmt.Errorf("metin yerleşim analizi için API anahtarı bulunamadı")
}

// analyzeTextPlacementWithGemini — Gemini Vision ile metin yerleşim analizi
func analyzeTextPlacementWithGemini(apiKey, imageBase64, prompt string) ([]DesignLayer, error) {
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

	jsonBody, _ := json.Marshal(reqBody)
	url := GeminiImageURL(apiKey)
	httpReq, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Gemini Vision isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gemini Vision hatası %d: %s", resp.StatusCode, string(body))
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

	var layers []DesignLayer
	if err := json.Unmarshal([]byte(content), &layers); err != nil {
		return nil, fmt.Errorf("layer JSON parse: %v — raw: %s", err, content[:min(len(content), 300)])
	}

	log.Printf("✅ Metin yerleşim analizi tamamlandı — Gemini Vision (%d katman)", len(layers))
	return layers, nil
}

// analyzeTextPlacementWithOpenAI — OpenAI Vision ile metin yerleşim analizi
func analyzeTextPlacementWithOpenAI(apiKey, imageBase64, prompt string) ([]DesignLayer, error) {
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
			{Role: "system", Content: json.RawMessage(`"You are a professional graphic designer specializing in text overlay placement on images. Always respond with valid JSON only. ALL text must be in TURKISH."`)},
			{Role: "user", Content: json.RawMessage(contentJSON)},
		},
		MaxTokens: 800, Temperature: 0.4,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI Vision isteği başarısız: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAI Vision hatası %d: %s", resp.StatusCode, string(body))
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

	var layers []DesignLayer
	if err := json.Unmarshal([]byte(content), &layers); err != nil {
		return nil, fmt.Errorf("layer JSON parse: %v — raw: %s", err, content[:min(len(content), 300)])
	}

	log.Printf("✅ Metin yerleşim analizi tamamlandı — OpenAI Vision (%d katman)", len(layers))
	return layers, nil
}

// ──────────────────────────────────────────────
// ResearchBrandFromWebsite — Web sitesini tarayıp
// AI ile marka bilgilerini çıkartır (onboarding için)
// ──────────────────────────────────────────────

type ToneExample struct {
	ID      string   `json:"id"`
	Example string   `json:"example"`
	Tags    []string `json:"tags"`
}

type BrandResearchResult struct {
	Name              string        `json:"name"`
	Sector            string        `json:"sector"`
	Description       string        `json:"description"`
	Slogan            string        `json:"slogan"`
	CustomerType      []string      `json:"customer_type"`
	ProductCategories string        `json:"product_categories"`
	ToneOfVoice       string        `json:"tone_of_voice"`
	ToneExamples      []ToneExample `json:"tone_examples"`
	Goal              string        `json:"goal"`
	ContentLanguage   string        `json:"content_language"`
	BrandColors       []string      `json:"brand_colors"`
	CompetitiveEdge   []string      `json:"competitive_edge"`
	LogoURL           string        `json:"logo_url,omitempty"`
	Website           string        `json:"website,omitempty"`
}

func ResearchBrandFromWebsite(websiteURL string) (*BrandResearchResult, error) {
	// 1) Web sitesini tara
	siteContent := scrapeWebsiteText(websiteURL)
	if siteContent == "" {
		return nil, fmt.Errorf("web sitesine ulaşılamadı veya içerik bulunamadı: %s", websiteURL)
	}

	log.Printf("🔍 Web sitesi tarandı: %s (%d karakter)", websiteURL, len(siteContent))

	prompt := fmt.Sprintf(`Sen dünya çapında bir marka stratejisti ve araştırmacısın. Aşağıdaki web sitesinin taranan içeriğini analiz ederek marka hakkında detaylı bir profil çıkar.

WEB SİTESİ URL: %s

TARANAN İÇERİK:
%s

Yukarıdaki içeriği dikkatle analiz et ve aşağıdaki JSON formatında YALNIZCA bir JSON nesnesi döndür. Markdown veya açıklama EKLEME.

{
  "name": "Markanın tam resmi adı",
  "sector": "moda|yiyecek|guzellik|saglik|ev|teknoloji|egitim|finans|turizm",
  "description": "Profesyonel 2-3 cümlelik Türkçe açıklama",
  "slogan": "Sitede görünen slogan (yoksa boş)",
  "product_categories": "Ürün/hizmet kategorileri, virgülle ayrılmış",
  "goal": "Markanın tahmini iş hedefi (Türkçe)",
  "content_language": "tr veya en",
  "brand_colors": ["#hex1", "#hex2"],
  "competitive_edge": ["fiyat","trend","kalite","cesit","hiz"],
  "customer_type": ["impulsif","analitik","fiyat_duyarli","premium","referans","konfor"],
  "tone_of_voice": "genc_enerjik|samimi_sicak|eglenceli|sik_sofistike|ilhamli_hikaye|guvenilir_kurumsal",
  "tone_examples": [
    {"id": "genc_enerjik", "example": "Bu markaya özel kısa enerjik örnek cümle", "tags": ["Kısa cümleler", "Emoji var", "Doğrudan CTA"]},
    {"id": "samimi_sicak", "example": "Bu markaya özel samimi örnek cümle", "tags": ["Sohbet havası", "Yumuşak davet", "Az emoji"]},
    {"id": "eglenceli", "example": "Bu markaya özel eğlenceli örnek cümle", "tags": ["Espri", "Merak uyandırır", "Hafif"]},
    {"id": "sik_sofistike", "example": "Bu markaya özel sofistike örnek cümle", "tags": ["Uzun cümleler", "Emoji yok", "Dolaylı CTA"]},
    {"id": "ilhamli_hikaye", "example": "Bu markaya özel hikaye tarzı örnek cümle", "tags": ["Metafor", "Duygusal", "Marka değeri"]},
    {"id": "guvenilir_kurumsal", "example": "Bu markaya özel kurumsal örnek cümle", "tags": ["Resmi dil", "Emoji yok", "Referans vurgusu"]}
  ],
  "website": "Sitenin ana URL'si (varsa)"
}

KURALLAR:
1. YALNIZCA geçerli JSON döndür, başka bir şey yazma
2. Tüm metin alanları TÜRKÇE olsun
3. Bulamadığın alanları boş string "" veya boş dizi [] olarak bırak
4. sector: moda, yiyecek, guzellik, saglik, ev, teknoloji, egitim, finans, turizm
5. competitive_edge: fiyat, trend, kalite, cesit, hiz
6. customer_type: impulsif, analitik, fiyat_duyarli, premium, referans, konfor
7. tone_of_voice: genc_enerjik, samimi_sicak, eglenceli, sik_sofistike, ilhamli_hikaye, guvenilir_kurumsal
8. tone_examples: 6 adet ton stili için BU MARKAYA ÖZEL örnek sosyal medya paylaşım cümlesi yaz. Sektöre ve markaya uygun olsun!`, websiteURL, siteContent)

	// 2) Önce Gemini dene
	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey != "" {
		result, err := researchWithGemini(geminiKey, prompt)
		if err == nil {
			return result, nil
		}
		log.Printf("⚠️ Gemini marka araştırma başarısız, OpenAI'ye geçiliyor: %v", err)
	}

	// 3) OpenAI fallback
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY tanımlı değil")
	}

	result, err := researchWithOpenAI(openaiKey, prompt)
	if err != nil {
		return nil, fmt.Errorf("hem Gemini hem OpenAI marka araştırma başarısız: %v", err)
	}
	return result, nil
}

func researchWithGemini(apiKey, prompt string) (*BrandResearchResult, error) {
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
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
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

	content := cleanJSONResponse(geminiResp.Candidates[0].Content.Parts[0].Text)

	var result BrandResearchResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %v — raw: %s", err, content[:min(len(content), 300)])
	}

	log.Printf("✅ Marka araştırma tamamlandı — Gemini: %s", result.Name)
	return &result, nil
}

func researchWithOpenAI(apiKey, prompt string) (*BrandResearchResult, error) {
	reqBody := openAIRequest{
		Model:       "gpt-4o-mini",
		Temperature: 0.4,
		MaxTokens:   1000,
		Messages: []openAIMessage{
			{Role: "system", Content: "Sen profesyonel bir marka araştırmacısısın. Web sitelerini analiz edip marka profillerini çıkartırsın. YALNIZCA geçerli JSON döndür."},
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

	content := cleanJSONResponse(oaiResp.Choices[0].Message.Content)

	var result BrandResearchResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %v — raw: %s", err, content[:min(len(content), 300)])
	}

	log.Printf("✅ Marka araştırma tamamlandı — OpenAI: %s", result.Name)
	return &result, nil
}

// ──────────────────────────────────────────────
// ExtractLogoURL — Web sitesinden logo/favicon URL'sini
// bulur ama İNDİRMEZ (sadece URL döner)
// ──────────────────────────────────────────────

func ExtractLogoURL(websiteURL string) (string, error) {
	if websiteURL == "" {
		return "", fmt.Errorf("URL boş")
	}
	if !strings.HasPrefix(websiteURL, "http") {
		websiteURL = "https://" + websiteURL
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", websiteURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("siteye ulaşılamadı: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("içerik okunamadı: %v", err)
	}
	html := string(bodyBytes)

	logoURL := ""

	// 1) <img> tag with logo-related class/id/alt/src — this is the REAL brand logo
	// Match img tags that have "logo" anywhere in class, id, alt, or src attributes
	reImgLogo := regexp.MustCompile(`(?i)<img[^>]*(?:class|id|alt|src)\s*=\s*["'][^"']*logo[^"']*["'][^>]*>`)
	imgMatches := reImgLogo.FindAllString(html, 10)
	for _, imgTag := range imgMatches {
		// Extract src from the img tag
		reSrc := regexp.MustCompile(`(?i)src\s*=\s*["']([^"']+)["']`)
		if srcMatch := reSrc.FindStringSubmatch(imgTag); len(srcMatch) > 1 {
			src := srcMatch[1]
			// Skip tiny tracking pixels and data URIs
			if strings.Contains(src, "data:") || strings.Contains(src, "1x1") || strings.Contains(src, "pixel") {
				continue
			}
			logoURL = src
			log.Printf("🖼️ Logo <img> tag'dan bulundu: %s", logoURL)
			break
		}
	}

	// 2) <header> or <nav> içindeki ilk büyük <img>
	if logoURL == "" {
		reHeader := regexp.MustCompile(`(?is)<(?:header|nav)[^>]*>(.*?)</(?:header|nav)>`)
		if headerMatch := reHeader.FindStringSubmatch(html); len(headerMatch) > 1 {
			headerHTML := headerMatch[1]
			reHeaderImg := regexp.MustCompile(`(?i)<img[^>]+src\s*=\s*["']([^"']+)["'][^>]*>`)
			if imgMatch := reHeaderImg.FindStringSubmatch(headerHTML); len(imgMatch) > 1 {
				src := imgMatch[1]
				if !strings.Contains(src, "data:") && !strings.Contains(src, "pixel") {
					logoURL = src
					log.Printf("🖼️ Logo header/nav'dan bulundu: %s", logoURL)
				}
			}
		}
	}

	// 3) og:image (Open Graph)
	if logoURL == "" {
		reOG := regexp.MustCompile(`(?i)<meta[^>]+property=["']og:image["'][^>]+content=["']([^"']+)["']`)
		if m := reOG.FindStringSubmatch(html); len(m) > 1 {
			logoURL = m[1]
			log.Printf("🖼️ Logo og:image'dan bulundu: %s", logoURL)
		}
	}

	// 4) apple-touch-icon (yüksek çözünürlüklü ikon)
	if logoURL == "" {
		reApple := regexp.MustCompile(`(?i)<link[^>]+rel=["']apple-touch-icon["'][^>]+href=["']([^"']+)["']`)
		if m := reApple.FindStringSubmatch(html); len(m) > 1 {
			logoURL = m[1]
			log.Printf("🖼️ Logo apple-touch-icon'dan bulundu: %s", logoURL)
		}
	}

	// 5) link rel="icon"
	if logoURL == "" {
		reIcon := regexp.MustCompile(`(?i)<link[^>]+rel=["'](?:shortcut )?icon["'][^>]+href=["']([^"']+)["']`)
		if m := reIcon.FindStringSubmatch(html); len(m) > 1 {
			logoURL = m[1]
		}
	}

	// 6) Fallback: /favicon.ico
	if logoURL == "" {
		logoURL = "/favicon.ico"
	}

	// Göreceli URL'yi mutlak URL'ye çevir
	logoURL = resolveURL(websiteURL, logoURL)

	log.Printf("🖼️ Logo URL sonuç: %s", logoURL)
	return logoURL, nil
}

// ──────────────────────────────────────────────
// ExtractBrandColors — Web sitesinden CSS renklerini çeker
// ──────────────────────────────────────────────

func ExtractBrandColors(websiteURL string) []string {
	if websiteURL == "" {
		return nil
	}
	if !strings.HasPrefix(websiteURL, "http") {
		websiteURL = "https://" + websiteURL
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", websiteURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("⚠️ Renk çekme hatası: %v", err)
		return nil
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	html := string(bodyBytes)

	// Hex renkleri bul: inline style, style tag, CSS
	reHex := regexp.MustCompile(`#([0-9a-fA-F]{6})\b`)
	hexMatches := reHex.FindAllString(html, 100)

	// Renk sayaçları - en çok kullanılan renkleri bul
	colorCount := make(map[string]int)
	for _, c := range hexMatches {
		c = strings.ToLower(c)
		// Siyah, beyaz ve gri tonlarını atla (boring)
		if c == "#000000" || c == "#ffffff" || c == "#fff" || c == "#f5f5f5" || c == "#333333" || c == "#666666" || c == "#999999" || c == "#cccccc" || c == "#eeeeee" || c == "#f0f0f0" || c == "#fafafa" {
			continue
		}
		colorCount[c]++
	}

	// rgb() renklerini de bul
	reRGB := regexp.MustCompile(`rgb\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
	rgbMatches := reRGB.FindAllStringSubmatch(html, 50)
	for _, m := range rgbMatches {
		r, _ := strconv.Atoi(m[1])
		g, _ := strconv.Atoi(m[2])
		b, _ := strconv.Atoi(m[3])
		hex := fmt.Sprintf("#%02x%02x%02x", r, g, b)
		if hex != "#000000" && hex != "#ffffff" {
			colorCount[hex]++
		}
	}

	// En sık kullanılan renkleri sırala
	type colorFreq struct {
		color string
		count int
	}
	var sorted []colorFreq
	for c, n := range colorCount {
		sorted = append(sorted, colorFreq{c, n})
	}
	// Sort by frequency descending
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].count > sorted[i].count {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// En çok kullanılan 3 renk
	var result []string
	for i, cf := range sorted {
		if i >= 3 {
			break
		}
		result = append(result, cf.color)
	}

	if len(result) > 0 {
		log.Printf("🎨 Site renkleri bulundu: %v", result)
	}
	return result
}

// resolveURL — göreceli URL'yi mutlak URL'ye çevirir
func resolveURL(base, relative string) string {
	if strings.HasPrefix(relative, "http") {
		return relative
	}
	if strings.HasPrefix(relative, "//") {
		return "https:" + relative
	}
	reBase := regexp.MustCompile(`^(https?://[^/]+)`)
	if m := reBase.FindString(base); m != "" {
		if strings.HasPrefix(relative, "/") {
			return m + relative
		}
		return m + "/" + relative
	}
	return relative
}

// ──────────────────────────────────────────────
// DownloadExternalLogo — Harici bir URL'den logoyu
// indirip diske kaydeder (son adımda çağrılır)
// ──────────────────────────────────────────────

func DownloadExternalLogo(logoURL string) (string, error) {
	if logoURL == "" {
		return "", fmt.Errorf("logo URL boş")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	logoResp, err := client.Get(logoURL)
	if err != nil {
		return "", fmt.Errorf("logo indirilemedi: %v", err)
	}
	defer logoResp.Body.Close()

	if logoResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("logo HTTP hatası: %d", logoResp.StatusCode)
	}

	logoBytes, err := io.ReadAll(logoResp.Body)
	if err != nil {
		return "", fmt.Errorf("logo okunamadı: %v", err)
	}

	if len(logoBytes) < 100 {
		return "", fmt.Errorf("logo dosyası çok küçük (%d byte)", len(logoBytes))
	}

	// Uzantıyı belirle
	contentType := logoResp.Header.Get("Content-Type")
	ext := ".png"
	switch {
	case strings.Contains(contentType, "jpeg") || strings.Contains(contentType, "jpg"):
		ext = ".jpg"
	case strings.Contains(contentType, "png"):
		ext = ".png"
	case strings.Contains(contentType, "svg"):
		ext = ".svg"
	case strings.Contains(contentType, "webp"):
		ext = ".webp"
	case strings.Contains(contentType, "x-icon") || strings.Contains(contentType, "vnd.microsoft.icon"):
		ext = ".ico"
	default:
		if strings.Contains(logoURL, ".svg") {
			ext = ".svg"
		} else if strings.Contains(logoURL, ".jpg") || strings.Contains(logoURL, ".jpeg") {
			ext = ".jpg"
		} else if strings.Contains(logoURL, ".webp") {
			ext = ".webp"
		}
	}

	uploadDir := "./assets/uploads/logos"
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("dizin oluşturulamadı: %v", err)
	}

	filename := fmt.Sprintf("%d_scraped%s", time.Now().UnixNano(), ext)

	// Bunny CDN yapılandırılmışsa → CDN'e yükle
	if IsBunnyCDNConfigured() {
		cdnURL, err := UploadToBunnyCDN(logoBytes, "logos", filename)
		if err != nil {
			log.Printf("⚠️ Bunny CDN logo upload başarısız, lokale kaydediliyor: %v", err)
		} else {
			log.Printf("✅ Logo Bunny CDN'e kaydedildi: %s (%d KB)", cdnURL, len(logoBytes)/1024)
			return cdnURL, nil
		}
	}

	// Fallback: lokale kaydet
	savePath := uploadDir + "/" + filename

	if err := os.WriteFile(savePath, logoBytes, 0644); err != nil {
		return "", fmt.Errorf("logo kaydedilemedi: %v", err)
	}

	savedURL := "/uploads/logos/" + filename
	log.Printf("✅ Logo indirildi ve kaydedildi: %s (%d KB)", savedURL, len(logoBytes)/1024)
	return savedURL, nil
}
