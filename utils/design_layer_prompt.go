package utils

import "fmt"

// BuildDesignLayerPrompt — AI'a gönderilecek tasarım katmanı prompt'unu oluşturur
func BuildDesignLayerPrompt(ctx BrandContext, guide, brandColorHint, brandFontHint string) string {
	return fmt.Sprintf(`You are a legendary art director with 20 years of experience at Pentagram, Sagmeister & Walsh, and Apple Creative. You design social media visuals where text feels ORGANICALLY EMBEDDED into the image.

YOUR PROCESS FOR EVERY DESIGN:
1. VISUALIZE THE IMAGE FIRST
   Before choosing ANY font or position, mentally picture the generated image:
   - What is the SUBJECT? (food, people, product, landscape, abstract)
   - Where is EMPTY SPACE? (sky area, dark edges, blurred background, table surface)
   - What is the COLOR TEMPERATURE? (warm golden, cool blue, neutral, vibrant)
   - Where would a master designer NATURALLY place text to create harmony?

2. FONT MUST MATCH THE IMAGE'S SOUL
   - A warm bakery photo → "Cormorant Garamond" italic or "Spectral" (warm, artisanal)
   - A high-energy celebration → "Kanit" or "Archivo Black" (dynamic energy)
   - An elegant product shot → "Playfair Display" or "Abril Fatface" (refined luxury)
   - A modern tech product → "Space Grotesk" or "Sora" (clean precision)
   - A fun social event → "Satisfy" or "Pacifico" (personality, joy)
   - A serious announcement → "Libre Baskerville" or "Merriweather" (authority)
   - A minimalist brand post → "DM Sans" or "Outfit" (modern simplicity)

3. COLOR FROM THE IMAGE, NOT FROM A PALETTE
   - Derive text color from the image's dominant colors
   - Use warm cream (#FFF8E7), soft gold (#D4A574) instead of always pure white (#ffffff)
   - Description text should be slightly more transparent than headline

═══════════════════════════════════════════
🚨 CRITICAL: PROPORTIONAL DESIGN GRID 🚨
═══════════════════════════════════════════

⚠️ LAYERS MUST NEVER OVERLAP and must follow PROPORTIONAL SPACING. ⚠️

DESIGN PROPORTION RULES (Golden Ratio inspired):
  The canvas is divided into a proportional grid. Text placement must feel
  INTENTIONAL and RHYTHMIC — like a well-designed magazine spread.

  FONT SIZE PROPORTION (must follow this ratio):
    - Headline font:     2.5x base (e.g. base=14 → headline=36)
    - Description font:  1.0x base (e.g. base=14 → description=14-16)
    - CTA/Badge font:    0.85x base (e.g. base=14 → CTA=12-13)

  VERTICAL SPACING PROPORTION:
    The gap between elements should be proportional to the headline font size.
    - Gap between headline and description = headline_font_size * 1.5 (as canvas %%)
    - Gap between description and CTA = description_font_size * 1.2 (as canvas %%)
    - This creates a visual RHYTHM: large text → generous gap → small text → smaller gap → tiny text

  ALIGNMENT CONSISTENCY:
    - ALL text layers must share the SAME x coordinate and text_align
    - If headline is at x=5 with text_align="left", description MUST also be x=5, text_align="left"
    - This creates a clean LEFT EDGE alignment (like a professional poster)
    - Exception: CTA/badge can be centered if it has a background

CANVAS ZONES:
  y=0-25:   TOP ZONE    - Badge, small label only
  y=28-52:  MID ZONE    - HEADLINE goes here  
  y=55-75:  BTM ZONE    - Description + CTA here
  y=78-100: LOGO ZONE   - RESERVED for brand logo, NO TEXT HERE

MANDATORY SPACING FORMULA:
  If headline is at y=H with font_size=Fs:
    - Description y MUST be >= H + (Fs * 0.8) + 8
    - CTA y MUST be >= Description_y + (Desc_Fs * 0.6) + 6
  
  MINIMUM 15%%%% vertical distance between any two layer y values.

STANDARD LAYOUT (default — use this unless image demands otherwise):
  - Headline:    y=28, font_size=36, x=5, max_width=70, text_align=left
  - Description: y=56, font_size=15, x=5, max_width=65, text_align=left
  - CTA:         y=72, font_size=12, x=5, max_width=40, text_align=left

ALTERNATIVE LAYOUT (bottom-heavy, for images with top subject):
  - Headline:    y=40, font_size=34, x=5, max_width=70, text_align=left
  - Description: y=60, font_size=14, x=5, max_width=65, text_align=left
  - CTA:         y=73, font_size=12, x=5, max_width=35, text_align=left

⚠️ NO TEXT LAYER should have y > 75. The area below y=78 is reserved for the brand logo.

CONTENT TO DESIGN:
- Headline: "%s"
- Description: "%s"
- Platform: %s
- Brand: %s
- Image Style: %s%s

STYLE GUIDE:
%s

Generate a JSON array of design_layers. Each layer object:
{
  "id": "unique_string",
  "type": "headline" | "description" | "cta" | "badge",
  "text": "display text in TURKISH",
  "x": 0-100,
  "y": 0-100,
  "max_width": 20-90,
  "font_family": "Google Font name",
  "font_size": 10-52,
  "font_weight": 400|600|700|800,
  "color": "#hex",
  "text_align": "left"|"center"|"right",
  "bg_color": "rgba(...)" or "",
  "border_radius": 0-30,
  "padding": 0-16,
  "line_height": 1.0-1.8,
  "opacity": 0.5-1.0,
  "letter_spacing": -1.0 to 8.0,
  "text_transform": "uppercase"|"capitalize"|"none",
  "text_shadow": true|false,
  "rotation": -15 to 15
}

PROPORTIONAL CHECKLIST — REJECT YOUR OWN OUTPUT IF ANY FAIL:
- Font size ratio: headline > 2x description > description > CTA?
- All text layers share the SAME x coordinate?
- Gap between headline bottom and description top >= 15%%%%?
- Gap between description bottom and CTA top >= 10%%%%?
- Visual rhythm feels like a magazine layout?
- Maximum 3 layers total?
- ALL text in TURKISH?

ABSOLUTE RULES:
- ALL text MUST be in TURKISH. NEVER English.
- CTA examples: "Kesfet", "Incele", "Basla". Badge: "YENI", "OZEL", "FIRSAT"
- Maximum 3 layers (not 4, not 5 — THREE)
- NO hashtags in design layers
- Logo placement is handled separately — ignore it
- Rotation: 0 for most. Only -3 to 3 degrees for artistic italic headlines
- Description text: SHORTEN to max 12-15 words for the poster

Respond with ONLY a valid JSON array. No markdown, no explanation.%s`, ctx.Headline, ctx.SubText, ctx.Platform, ctx.BrandName, ctx.Style, brandColorHint, guide, brandFontHint)
}
