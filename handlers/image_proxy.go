package handlers

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// ──────────────────────────────────────────────
// GET /api/protected/ai-image?prompt=...&width=480&height=480
// Pollinations.ai proxy — avoids CORS/referrer issues
// ──────────────────────────────────────────────

func ProxyAIImage(c *gin.Context) {
	prompt := c.Query("prompt")
	if prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prompt parametresi gerekli"})
		return
	}

	width := c.DefaultQuery("width", "480")
	height := c.DefaultQuery("height", "480")
	seed := c.DefaultQuery("seed", "42")

	// Pollinations.ai URL'si
	url := fmt.Sprintf(
		"https://image.pollinations.ai/prompt/%s?width=%s&height=%s&nologo=true&seed=%s",
		prompt, width, height, seed,
	)

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "istek oluşturulamadı"})
		return
	}

	// Pollinations.ai direkt sunucu isteği gibi görünsün
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; DIOPanel/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "görsel servisi yanıt vermedi"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{"error": fmt.Sprintf("görsel servisi hata döndü: %d", resp.StatusCode)})
		return
	}

	// Content-Type'ı aktar
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	// Cache headers — görseller 24 saat cache'lenebilir
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=86400")
	c.Header("Access-Control-Allow-Origin", "*")

	c.Status(http.StatusOK)
	io.Copy(c.Writer, resp.Body)
}
