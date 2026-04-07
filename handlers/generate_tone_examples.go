package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// POST /api/protected/generate-tone-examples
// Marka bilgilerine göre ton örnekleri üret
// ──────────────────────────────────────────────

type ToneExampleRequest struct {
	CompanyName       string   `json:"company_name"`
	Sector            string   `json:"sector"`
	ProductCategories string   `json:"product_categories"`
	CompetitiveEdge   []string `json:"competitive_edge"`
	Description       string   `json:"description"`
}

type ToneExample struct {
	ID      string   `json:"id"`
	Example string   `json:"example"`
	Tags    []string `json:"tags"`
}

func GenerateToneExamples(c *gin.Context) {
	var req ToneExampleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.CompanyName == "" && req.Sector == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Marka adı veya sektör gerekli"})
		return
	}

	prompt := buildToneExamplesPrompt(req)

	// Gemini dene
	examples, err := callGeminiForToneExamples(prompt)
	if err != nil {
		log.Printf("⚠️ Gemini ton örnekleri hatası: %v, OpenAI'ye geçiliyor", err)
		examples, err = callOpenAIForToneExamples(prompt)
		if err != nil {
			log.Printf("❌ OpenAI ton örnekleri hatası: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ton örnekleri üretilemedi"})
			return
		}
	}

	log.Printf("✅ %d ton örneği üretildi — %s", len(examples), req.CompanyName)
	c.JSON(http.StatusOK, gin.H{
		"tone_examples": examples,
	})
}

func buildToneExamplesPrompt(req ToneExampleRequest) string {
	context := ""
	if req.CompanyName != "" {
		context += fmt.Sprintf("Marka: %s\n", req.CompanyName)
	}
	if req.Sector != "" {
		context += fmt.Sprintf("Sektör: %s\n", req.Sector)
	}
	if req.ProductCategories != "" {
		context += fmt.Sprintf("Ürünler: %s\n", req.ProductCategories)
	}
	if len(req.CompetitiveEdge) > 0 {
		context += fmt.Sprintf("Rekabet Avantajları: %s\n", strings.Join(req.CompetitiveEdge, ", "))
	}
	if req.Description != "" {
		context += fmt.Sprintf("Açıklama: %s\n", req.Description)
	}

	return fmt.Sprintf(`Sen bir marka iletişim uzmanısın. Aşağıdaki markanın sektörüne ve ürünlerine özel, 6 farklı iletişim tonunda ÖRNEK SOSYAL MEDYA PAYLAŞIM CÜMLELERİ üret.

%s

Her ton için bu MARKAYA ÖZGÜ, gerçekçi bir sosyal medya paylaşım cümlesi yaz. Cümleler markanın ürünlerini, sektörünü ve kimliğini yansıtmalı.

TONLAR VE ID'LERİ:
1. id: "genc_enerjik" — Genç & Enerjik ton. Heyecanlı, dinamik, trend takip eden dil.
2. id: "samimi_sicak" — Samimi & Sıcak ton. Arkadaşça, güven veren, konuşma dili.
3. id: "eglenceli" — Eğlenceli & Espri ton. Komik, esprili, viral potansiyelli dil.
4. id: "sik_sofistike" — Şık & Sofistike ton. Lüks, premium, zarif bir dil.
5. id: "ilhamli_hikaye" — İlhamlı & Hikaye ton. Hikaye anlatan, ilham veren, duygusal dil.
6. id: "guvenilir_kurumsal" — Güvenilir & Kurumsal ton. Profesyonel, ciddi, güven veren dil.

HER TON İÇİN:
- "id": Yukarıdaki id'lerden biri (tam eşleşmeli)
- "example": Bu tonda yazılmış, bu markaya özel 1-2 cümlelik sosyal medya paylaşım örneği (Türkçe, emoji içerebilir)
- "tags": Bu tonun 3-4 kelimelik etiketleri (ör: ["samimi", "güven verici", "sıcak"])

YANITINI SADECE GEÇERLİ BİR JSON DİZİSİ OLARAK VER:
[{"id":"genc_enerjik","example":"...","tags":["..."]}, ...]`, context)
}

// ──────────────────────────────────────────────
// API Calls
// ──────────────────────────────────────────────

func callGeminiForToneExamples(prompt string) ([]ToneExample, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY tanımlı değil")
	}

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]interface{}{{"text": prompt}}},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	url := utils.GeminiTextURL(apiKey)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API hatası %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("boş yanıt")
	}

	text := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var examples []ToneExample
	if err := json.Unmarshal([]byte(text), &examples); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %v", err)
	}
	return examples, nil
}

func callOpenAIForToneExamples(prompt string) ([]ToneExample, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY tanımlı değil")
	}

	reqBody := map[string]interface{}{
		"model":       "gpt-4o-mini",
		"temperature": 0.8,
		"max_tokens":  2000,
		"messages": []map[string]interface{}{
			{"role": "system", "content": "Sen bir marka iletişim uzmanısın. SADECE geçerli JSON döndür."},
			{"role": "user", "content": prompt},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API hatası %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("boş yanıt")
	}

	text := strings.TrimSpace(result.Choices[0].Message.Content)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var examples []ToneExample
	if err := json.Unmarshal([]byte(text), &examples); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %v", err)
	}
	return examples, nil
}
