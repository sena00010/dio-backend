package utils

// ═══════════════════════════════════════════════════
// AI TEXT HELPERS — Diğer paketlerden çağrılabilir wrapper fonksiyonlar
// ═══════════════════════════════════════════════════

// CallGeminiRawText — Gemini'ye düz metin prompt gönder, düz metin yanıt al
func CallGeminiRawText(apiKey, prompt string) (string, error) {
	return callGeminiRaw(apiKey, prompt)
}

// CallOpenAIRawText — OpenAI'ye düz metin prompt gönder, düz metin yanıt al
func CallOpenAIRawText(apiKey, prompt string) (string, error) {
	return callOpenAIRaw(apiKey, prompt)
}
