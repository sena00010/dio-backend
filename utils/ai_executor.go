package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// GenerateWithPrompt, DB'deki promptKey'i bulur, verilerle render eder
// ve prompt'un AiProvider ile AiModel ayarlarına göre ilgili AI motoruna istek atar.
func GenerateWithPrompt(promptKey string, data map[string]interface{}) (string, error) {
	// 1. Prompt'u Render Et
	renderedText, err := GetRenderedPrompt(promptKey, data)
	if err != nil {
		return "", fmt.Errorf("prompt render hatası: %w", err)
	}

	// 2. Prompt'un AI ayarlarını veritabanından çek
	var prompt models.SystemPrompt
	if err := database.DB.Where("prompt_key = ?", promptKey).First(&prompt).Error; err != nil {
		return "", fmt.Errorf("prompt DB'de bulunamadı: %v", err)
	}

	provider := strings.ToLower(prompt.AiProvider)
	model := prompt.AiModel

	if provider == "" {
		provider = "openai" // fallback
	}
	if model == "" {
		model = "gpt-4o-mini" // fallback
	}

	// 2.5 Güvenlik Katmanı: Eğer provider ve model eşleşmiyorsa, 404 önlemek için düzelt
	if provider == "gemini" && !strings.HasPrefix(model, "gemini") {
		model = "gemini-2.5-pro"
	}
	if provider == "openai" && (!strings.HasPrefix(model, "gpt") && !strings.HasPrefix(model, "o1") && !strings.HasPrefix(model, "o3") && !strings.HasPrefix(model, "dall")) {
		model = "gpt-4o-mini"
	}

	// 3. Provider'a göre yönlendir
	switch provider {
	case "gemini":
		return callGeminiText(model, renderedText)
	case "openai":
		return callOpenAIText(model, renderedText)
	default:
		return "", fmt.Errorf("bilinmeyen AiProvider: %s", provider)
	}
}

// ─────────────────────────────────────────────────────────────
// Dahili Sağlayıcı İstek Formatları
// ─────────────────────────────────────────────────────────────

// isReasoningModel — gpt-5 ailesi ve o-serisi (farklı parametre seti kullanır)
func isReasoningModel(model string) bool {
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "gpt-5") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3") || strings.HasPrefix(m, "o4")
}

func callOpenAIText(model string, prompt string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY bulunamadı")
	}

	reqBody := map[string]interface{}{
		"model":       model,
		"temperature": 0.7,
		"max_tokens":  8192,
		"messages": []map[string]interface{}{
			{"role": "user", "content": prompt},
		},
	}
	// gpt-5 / o-serisi akıl yürütme modelleri temperature ve max_tokens kabul etmez
	if isReasoningModel(model) {
		delete(reqBody, "temperature")
		delete(reqBody, "max_tokens")
		reqBody["max_completion_tokens"] = 16000
		if !strings.Contains(strings.ToLower(model), "chat") {
			reqBody["reasoning_effort"] = "low"
		}
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 150 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI HTTP %d: %s", resp.StatusCode, string(body))
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
		return "", fmt.Errorf("boş yanıt geldi")
	}

	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func callGeminiText(model string, prompt string) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY bulunamadı")
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
	
	// Gemini API URL'i model adına göre ayarlanmalı:
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)

	httpReq, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Gemini HTTP %d: %s", resp.StatusCode, string(body))
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
		return "", err
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("boş yanıt geldi")
	}

	return strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text), nil
}
