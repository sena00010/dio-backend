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
	siteContent := ""
	if req.ProductCategories != "" {
		siteContent += fmt.Sprintf("Ürün/Hizmet Kategorileri: %s\n", req.ProductCategories)
	}
	if req.Goal != "" {
		siteContent += fmt.Sprintf("Hedef: %s\n", req.Goal)
	}
	if req.Slogan != "" {
		siteContent += fmt.Sprintf("Slogan: %s\n", req.Slogan)
	}

	prompt, err := utils.GetRenderedPrompt("brand_description_generator", map[string]interface{}{
		"companyName": req.CompanyName,
		"sector":      req.Sector,
		"website":     req.Website,
		"siteContent": siteContent,
	})
	if err != nil || prompt == "" {
		// Fallback failsafe
		log.Printf("⚠️ fallback to legacy string")
		return "Lütfen marka hakkında kısa bir özet çıkar."
	}
	return prompt
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
		"model":       utils.GetAiModel("gpt_fast_general", "gpt-4o-mini"),
		"temperature": 0.7,
		"max_tokens":  500,
		"messages": []map[string]interface{}{
			{"role": "system", "content": "Sen profesyonel bir marka stratejistisin. Verilen marka bilgilerini analiz ederek kısa, profesyonel şirket açıklamaları yazıyorsun. SADECE düz metin paragraf ver."},
			{"role": "user", "content": prompt},
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(utils.AdaptChatBody(bodyBytes)))
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
