package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ListCompetitors - Bir firmaya ait tüm rakipleri listeler
func ListCompetitors(c *gin.Context) {
	companyID, _ := strconv.Atoi(c.Param("id"))

	var competitors []models.CompanyCompetitor
	database.DB.Where("company_id = ?", companyID).Order("created_at desc").Find(&competitors)

	c.JSON(http.StatusOK, competitors)
}

// AddCompetitors - Toplu rakip ekleme (onboarding ve panel için)
func AddCompetitors(c *gin.Context) {
	companyID, _ := strconv.Atoi(c.Param("id"))

	var req []struct {
		Name    string `json:"name"`
		Website string `json:"website"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz JSON formatı"})
		return
	}

	for _, item := range req {
		comp := models.CompanyCompetitor{
			CompanyID: uint(companyID),
			Name:      item.Name,
			Website:   item.Website,
		}
		database.DB.Create(&comp)
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Rakipler eklendi"})
}

// DeleteCompetitor - Rakip silme
func DeleteCompetitor(c *gin.Context) {
	companyID, _ := strconv.Atoi(c.Param("id"))
	competitorID, _ := strconv.Atoi(c.Param("competitorId"))

	if err := database.DB.Where("id = ? AND company_id = ?", competitorID, companyID).Delete(&models.CompanyCompetitor{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Silinemedi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Silindi"})
}

// SuggestCompetitors - AI/Scraping simülasyonu ile rakip önerme
func SuggestCompetitors(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		Website string `json:"website"`
		Sector  string `json:"sector"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz format"})
		return
	}

	// LLM'e gidecek promptu hazırlıyoruz
	prompt := fmt.Sprintf(`Bizim şirketimizin adı: "%s", faaliyet gösterdiği sektör: "%s", web sitesi: "%s".
Daha çok arama motorlarındaki pazar payı ve web trafiği açısından bizimle aynı segmentte (veya bizim için en önemli referans olabilecek) en iyi 3 rakibi bul.
Dönüşü sadece içerisi JSON Array formatında olacak şekilde metin ver. JSON HARİCİ HİÇBİR AÇIKLAMA YAZMA.
[
	{"name": "Rakip 1", "website": "rakip1.com"},
	{"name": "Rakip 2", "website": "rakip2.com"}
]`, req.Name, req.Sector, req.Website)

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "OpenAI API anahtarı bulunamadı"})
		return
	}

	// OpenAI request payload
	reqBody := map[string]interface{}{
		"model": "gpt-4o-mini", // Veya gpt-3.5-turbo
		"messages": []map[string]interface{}{
			{
				"role":    "system",
				"content": "Sen yardımcı ve analiz yapan bir SEO/Pazarlama uzmanısın. Yalnızca istenilen formatta yanıt verirsin.",
			},
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"temperature": 0.7,
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "API isteği hazırlanamadı"})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "OpenAI isteği başarısız"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("OpenAI API Hatası: %d", resp.StatusCode)})
		return
	}

	respBody, _ := io.ReadAll(resp.Body)

	// OpenAI Response'tan message okumasi
	var openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "API yanıtı okunamadı"})
		return
	}

	if len(openAIResp.Choices) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI boş yanıt döndü"})
		return
	}

	// AI'dan gelen JSON'u temizle (markdown ```json block)
	content := strings.TrimSpace(openAIResp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var competitors []map[string]string
	if err := json.Unmarshal([]byte(content), &competitors); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI yanıtı beklenen JSON formatında değil"})
		return
	}

	c.JSON(http.StatusOK, competitors)
}
