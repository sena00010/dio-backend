package utils

// ──────────────────────────────────────────────
// MERKEZ MODEL KONFIG — Admin paneldeki system_prompts tablosundan okunur.
// Artık hardcoded değil, admin ne seçtiyse o kullanılır.
// ──────────────────────────────────────────────

const (
	// GeminiAPIBase — Gemini API base URL
	GeminiAPIBase = "https://generativelanguage.googleapis.com/v1beta/models"

	// Varsayılan model isimleri — system_prompts'ta kayıt yoksa bunlar kullanılır
	DefaultGeminiTextModel  = "gemini-2.5-flash"
	DefaultGeminiImageModel = "gemini-2.5-flash-image"
)

// GeminiTextURL — Metin üretimi için tam URL oluşturur
func GeminiTextURL(apiKey string) string {
	model := DefaultGeminiTextModel
	return GeminiAPIBase + "/" + model + ":generateContent?key=" + apiKey
}

// GeminiImageURL — Görsel üretimi için tam URL oluşturur
// Admin paneldeki product_studio_edit prompt'undaki ai_model değerini kullanır
func GeminiImageURL(apiKey string) string {
	// Görsel üretimi için SADECE gemini-2.5-flash-image kullanılabilir.
	// Admin paneldeki model seçimi metin modeli olabilir, görsel üretiminde her zaman
	// doğru modeli kullanmamız gerekiyor.
	return GeminiAPIBase + "/" + DefaultGeminiImageModel + ":generateContent?key=" + apiKey
}

// GeminiImageURLWithModel — Belirli bir model ile URL oluşturur
func GeminiImageURLWithModel(apiKey, model string) string {
	if model == "" {
		model = DefaultGeminiImageModel
	}
	return GeminiAPIBase + "/" + model + ":generateContent?key=" + apiKey
}
