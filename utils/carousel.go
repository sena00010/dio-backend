package utils

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// CarouselSlideText — AI'ın ürettiği slayt metni
type CarouselSlideText struct {
	Role     string `json:"role"`
	Headline string `json:"headline"`
	Body     string `json:"body"`
}

// GenerateCarouselText — gönderi için kapak → değer slaytları → CTA yapısında carousel metni üretir.
// Model, içerik fikir promptuyla aynı (admin panelden content_idea_generator ayarı).
func GenerateCarouselText(title, caption, platform, companyContext string, slideCount int) ([]CarouselSlideText, error) {
	prompt := fmt.Sprintf(`Sen Türkiye'nin en iyi sosyal medya içerik stratejistisin. Aşağıdaki gönderi için TAM OLARAK %d slaytlık bir %s carousel metni yaz.

GÖNDERİ BAŞLIĞI: %s
GÖNDERİ METNİ: %s
MARKA BAĞLAMI: %s

YAPI:
- 1. slayt "cover": kaydırmaya zorlayan kanca başlık (en fazla 7 kelime; soru, sayı veya cesur iddia). body: merak uyandıran kısa alt başlık (en fazla 10 kelime).
- Aradaki slaytlar "value": her biri TEK somut fikir. headline en fazla 6 kelime; body 15-30 kelime, uygulanabilir ve net. Slaytlar mantıklı bir sırayla ilerlesin, birbirini tekrar etmesin.
- Son slayt "cta": headline en fazla 5 kelime (örn. "Kaydet, sonra lazım olacak"); body en fazla 15 kelime, tek net yönlendirme (DM, yorum, kaydet, link).

KURALLAR:
- Doğru ve doğal Türkçe, Türkçe karakterlerle (ç, ğ, ı, İ, ö, ş, ü). Emoji kullanma.
- Uydurma rakam, istatistik, müşteri adı veya yorum YOK.
- Hashtag YOK.

Sadece geçerli bir JSON dizisi döndür, başka hiçbir şey yazma:
[{"role":"cover","headline":"...","body":"..."},{"role":"value","headline":"...","body":"..."},{"role":"cta","headline":"...","body":"..."}]`,
		slideCount, platform, title, caption, companyContext)

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
		return nil, fmt.Errorf("carousel metni üretilemedi: %v", err)
	}

	start, end := strings.Index(text, "["), strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("carousel yanıtında JSON bulunamadı")
	}
	var slides []CarouselSlideText
	if err := json.Unmarshal([]byte(fixAIJSON(text[start:end+1])), &slides); err != nil {
		return nil, fmt.Errorf("carousel JSON ayrıştırılamadı: %v", err)
	}
	if len(slides) < 3 {
		return nil, fmt.Errorf("carousel için yeterli slayt üretilmedi (%d)", len(slides))
	}

	// Rolleri garanti et: ilk kapak, son CTA, aradakiler değer
	for i := range slides {
		switch {
		case i == 0:
			slides[i].Role = "cover"
		case i == len(slides)-1:
			slides[i].Role = "cta"
		default:
			slides[i].Role = "value"
		}
	}
	return slides, nil
}
