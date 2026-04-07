package utils

// ──────────────────────────────────────────────
// MERKEZ MODEL KONFIG — Tüm AI model adları TEK YERDEN yönetilir
// Değişiklik yapmak için SADECE bu dosyayı düzenleyin
// ──────────────────────────────────────────────

const (
	// GeminiTextModel — Metin üretimi için kullanılan Gemini modeli
	// Kullanım: Kreatif metin, konu önerileri, trend radar, SEO analizi vs.
	GeminiTextModel = "gemini-2.5-flash"

	// GeminiImageModel — Görsel üretimi için kullanılan Gemini modeli (Nano Banana)
	// Kullanım: Kreatif görsel üretimi, marka görselleri, sosyal medya görselleri
	GeminiImageModel = "gemini-2.5-flash-image"

	// GeminiAPIBase — Gemini API base URL
	GeminiAPIBase = "https://generativelanguage.googleapis.com/v1beta/models"
)

// GeminiTextURL — Metin üretimi için tam URL oluşturur
func GeminiTextURL(apiKey string) string {
	return GeminiAPIBase + "/" + GeminiTextModel + ":generateContent?key=" + apiKey
}

// GeminiImageURL — Görsel üretimi için tam URL oluşturur
func GeminiImageURL(apiKey string) string {
	return GeminiAPIBase + "/" + GeminiImageModel + ":generateContent?key=" + apiKey
}
