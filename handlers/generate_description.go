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
// POST /api/protected/generate-description
// AI ile marka açıklaması üret
// ──────────────────────────────────────────────

type GenerateDescriptionRequest struct {
	CompanyName       string `json:"company_name"`
	Website           string `json:"website"`
	Sector            string `json:"sector"`
	ProductCategories string `json:"product_categories"`
	Goal              string `json:"goal"`
	Slogan            string `json:"slogan"`
}

func GenerateDescription(c *gin.Context) {
	var req GenerateDescriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.CompanyName == "" && req.Website == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Marka adı veya website adresi gerekli"})
		return
	}

	// Prompt oluştur
	prompt := buildDescriptionPrompt(req)

	// Gemini ile üret
	description, err := callGeminiForDescription(prompt)
	if err != nil {
		log.Printf("⚠️ Gemini açıklama üretme hatası: %v", err)
		// OpenAI fallback
		description, err = callOpenAIForDescription(prompt)
		if err != nil {
			log.Printf("❌ OpenAI açıklama üretme hatası: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Açıklama üretilemedi"})
			return
		}
	}

	log.Printf("✅ AI açıklama üretildi — %s", req.CompanyName)
	c.JSON(http.StatusOK, gin.H{
		"description": description,
	})
}

func buildDescriptionPrompt(req GenerateDescriptionRequest) string {
	context := ""
	if req.CompanyName != "" {
		context += fmt.Sprintf("Marka/Şirket Adı: %s\n", req.CompanyName)
	}
	if req.Website != "" {
		context += fmt.Sprintf("Website: %s\n", req.Website)
	}
	if req.Sector != "" {
		context += fmt.Sprintf("Sektör: %s\n", req.Sector)
	}
	if req.ProductCategories != "" {
		context += fmt.Sprintf("Ürün/Hizmet Kategorileri: %s\n", req.ProductCategories)
	}
	if req.Goal != "" {
		context += fmt.Sprintf("Hedef: %s\n", req.Goal)
	}
	if req.Slogan != "" {
		context += fmt.Sprintf("Slogan: %s\n", req.Slogan)
	}

	return fmt.Sprintf(`Sen bir dijital pazarlama ve marka stratejisti olarak çalışıyorsun. Aşağıdaki websiteyi veya marka adını analiz ederek şirket hakkında kısa ve net bir şirket özeti oluştur.

%s

Aşağıdaki bilgileri çıkar ve TEK BİR PARAGRAF halinde yaz:

1. Şirket ne iş yapıyor
2. Hangi hizmetleri sunuyor
3. Kaç farklı hizmet alanı var
4. Hedef müşteri kitlesi (çalışan sayısı ve şirket tipi)
5. Reklamların ulaşması gereken kişiler (unvanlar ve departmanlar)
6. Öncelikli sektörler
7. Şirketin pazarlama diliyle yazılmış kısa bir şirket tanımı

KURALLAR:
- Çıktıyı profesyonel bir pazarlama analizi gibi yaz
- Türkçe olarak yaz
- Sadece düz metin paragraf ver — JSON, markdown, başlık, madde işareti KULLANMA
- Kısa ve öz ol — maksimum 4-5 cümle
- Markanın ses tonuna uygun, profesyonel bir dil kullan
- Website varsa mutlaka analiz et ve gerçek bilgileri kullan

SADECE açıklama metnini yaz, başka bir şey ekleme.`, context)
}

// ──────────────────────────────────────────────
// Gemini API çağrısı
// ──────────────────────────────────────────────

type geminiDescRequest struct {
	Contents []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

func callGeminiForDescription(prompt string) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY tanımlı değil")
	}

	reqBody := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	url := utils.GeminiTextURL(apiKey)

	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini API hatası %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
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
		return "", err
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("boş yanıt")
	}

	text := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
	// Markdown temizle
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	return text, nil
}

func callOpenAIForDescription(prompt string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY tanımlı değil")
	}

	reqBody := map[string]interface{}{
		"model":       "gpt-4o-mini",
		"temperature": 0.7,
		"max_tokens":  500,
		"messages": []map[string]interface{}{
			{"role": "system", "content": "Sen profesyonel bir marka stratejistisin. Verilen marka bilgilerini analiz ederek kısa, profesyonel şirket açıklamaları yazıyorsun. SADECE düz metin paragraf ver."},
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
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API hatası %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("boş yanıt")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}
