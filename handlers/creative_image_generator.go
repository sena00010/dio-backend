package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// Kreatif Görsel Üretim Sistemi
// Yüksek Eşzamanlılık + Paralel Provider Dağılımı
// ──────────────────────────────────────────────

// In-memory store for creative image generation jobs
var (
	creativeImageJobs   = make(map[string]*CreativeImageJob)
	creativeImageJobsMu sync.RWMutex
)

// Global concurrent request limiter — API rate limit koruması
// Gemini: ~10 RPM, DALL-E: ~5 RPM, toplam ~15 RPM
var concurrentSemaphore = make(chan struct{}, 3) // max 3 eşzamanlı görsel üretimi

// CreativeImageJob — Bir batch görsel üretim işi
type CreativeImageJob struct {
	JobID     string                           `json:"job_id"`
	Status    string                           `json:"status"` // "processing", "completed", "failed"
	Total     int                              `json:"total"`
	Completed int32                            `json:"completed"` // atomic counter
	Images    map[string]CreativeImageResult   `json:"images"`    // variant_id -> result
	mu        sync.RWMutex                     // image map'i korur
	CreatedAt time.Time                        `json:"created_at"`
}

// CreativeImageResult — Tek bir varyant görseli sonucu
type CreativeImageResult struct {
	VariantID    string              `json:"variant_id"`
	ImageURL     string              `json:"image_url"`
	Status       string              `json:"status"` // "pending", "generating", "done", "error"
	Error        string              `json:"error,omitempty"`
	Provider     string              `json:"provider,omitempty"`      // "gemini", "dall-e"
	DesignLayers []utils.DesignLayer `json:"design_layers,omitempty"` // AI Vision ile belirlenen metin katmanları
}

// ──────────────────────────────────────────────
// POST /api/protected/generate-creative-images
// Kreatif varyantlar için asenkron görsel üretimi başlat
// ──────────────────────────────────────────────

type GenerateCreativeImagesRequest struct {
	CompanyID uint                        `json:"company_id" binding:"required"`
	Variants  []CreativeImageVariantInput  `json:"variants" binding:"required"`
}

type CreativeImageVariantInput struct {
	VariantID         string   `json:"variant_id"`
	VisualDescription string   `json:"visual_description"`
	Headline          string   `json:"headline"`
	SubHeadline       string   `json:"sub_headline"`
	ColorPalette      []string `json:"color_palette"`
	Format            string   `json:"format"`
	Platform          string   `json:"platform"`
	Emphasis          string   `json:"emphasis"`
	IsAIRecommended   bool     `json:"is_ai_recommended"`
}

func GenerateCreativeImages(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req GenerateCreativeImagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Şirket erişim kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	// Job ID oluştur
	jobID := generateJobID()

	// Job'u başlat
	job := &CreativeImageJob{
		JobID:     jobID,
		Status:    "processing",
		Total:     len(req.Variants),
		Completed: 0,
		Images:    make(map[string]CreativeImageResult),
		CreatedAt: time.Now(),
	}

	// Her varyant için pending durumu ayarla
	for _, v := range req.Variants {
		job.Images[v.VariantID] = CreativeImageResult{
			VariantID: v.VariantID,
			Status:    "pending",
		}
	}

	// Job'u global store'a kaydet
	creativeImageJobsMu.Lock()
	creativeImageJobs[jobID] = job
	creativeImageJobsMu.Unlock()

	// Provider dağılımı belirle
	providers := detectAvailableProviders()
	log.Printf("🎨 Kreatif görsel üretimi başlatıldı — Job: %s, %d varyant, Providers: %v (marka: %s)",
		jobID, len(req.Variants), providers, company.Name)

	// ────────────────────────────────────────────
	// TÜM VARYANTLARI PARALEL OLARAK BAŞLAT
	// ────────────────────────────────────────────
	go generateImagesParallel(job, req.Variants, company, providers)

	c.JSON(http.StatusOK, gin.H{
		"job_id":    jobID,
		"status":    "processing",
		"total":     len(req.Variants),
		"providers": providers,
		"message":   fmt.Sprintf("Görsel üretimi başlatıldı (%d paralel)", len(req.Variants)),
	})
}

// ──────────────────────────────────────────────
// GET /api/protected/creative-images-status?job_id=xxx
// Polling endpoint — hangi görseller hazır?
// ──────────────────────────────────────────────

func GetCreativeImagesStatus(c *gin.Context) {
	jobID := c.Query("job_id")
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job_id parametresi gerekli"})
		return
	}

	creativeImageJobsMu.RLock()
	job, exists := creativeImageJobs[jobID]
	creativeImageJobsMu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job bulunamadı"})
		return
	}

	// Sonuçları topla
	job.mu.RLock()
	images := make([]CreativeImageResult, 0, len(job.Images))
	for _, img := range job.Images {
		images = append(images, img)
	}
	completed := atomic.LoadInt32(&job.Completed)
	status := job.Status
	job.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"job_id":    job.JobID,
		"status":    status,
		"total":     job.Total,
		"completed": completed,
		"images":    images,
	})
}

// ──────────────────────────────────────────────
// Provider Tespiti — Hangi API key'ler mevcut?
// ──────────────────────────────────────────────

func detectAvailableProviders() []string {
	providers := []string{}
	if os.Getenv("GEMINI_API_KEY") != "" {
		providers = append(providers, "gemini")
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		providers = append(providers, "dall-e")
	}
	if os.Getenv("BFL_API_KEY") != "" {
		providers = append(providers, "bfl")
	}
	return providers
}

// ──────────────────────────────────────────────
// PARALEL GÖRSEL ÜRETİMİ — Worker Pool Mimarisi
// Tüm varyantlar aynı anda, provider'lar arası dağıtılır
// ──────────────────────────────────────────────

func generateImagesParallel(job *CreativeImageJob, variants []CreativeImageVariantInput, company models.Company, providers []string) {
	var wg sync.WaitGroup
	startTime := time.Now()

	for i, v := range variants {
		wg.Add(1)

		// Provider seçimi — round-robin dağılım
		provider := selectProvider(providers, i)

		go func(variant CreativeImageVariantInput, idx int, prov string) {
			defer wg.Done()

			// Semaphore — eşzamanlılık limiti (rate limit koruması)
			concurrentSemaphore <- struct{}{}
			defer func() { <-concurrentSemaphore }()

			// Durumu "generating" yap
			job.mu.Lock()
			result := job.Images[variant.VariantID]
			result.Status = "generating"
			result.Provider = prov
			job.Images[variant.VariantID] = result
			job.mu.Unlock()

			log.Printf("🖼️ [%d/%d] Görsel üretiliyor: %s → %s (marka: %s)",
				idx+1, len(variants), variant.VariantID, prov, company.Name)

			// Prompt oluştur — TextPosition boş bırak, AI Vision sonradan belirleyecek
			brandCtx := utils.BrandContext{
				BrandName:      company.Name,
				Sector:         company.Sector,
				TargetAudience: company.TargetAudience,
				BrandColors:    company.BrandColors,
				ToneOfVoice:    company.ToneOfVoice,
				Slogan:         company.Slogan,
				Description:    company.Description,
				BrandFont:      company.BrandFont,
				Website:        company.Website,
				Platform:       variant.Platform,
				Headline:       variant.Headline,
				SubText:        variant.SubHeadline,
				ImagePrompt:    variant.VisualDescription,
				Style:          "realistic",
				TextPosition:   "", // Boş — AI Vision görseli analiz edip belirleyecek
			}

			prompt := utils.BuildProfessionalImagePrompt(brandCtx)

			// Provider'a göre üret
			var imageURL string
			var err error

			switch prov {
			case "dall-e":
				imageURL, err = generateWithDALLE(prompt)
			case "gemini":
				imageURL, err = generateWithGemini(prompt)
			default:
				// Fallback: tam pipeline
				imageURL, err = utils.GenerateBrandImage(brandCtx)
			}

			// Birincil provider başarısız → fallback dene
			if err != nil {
				log.Printf("⚠️ [%s] %s başarısız, fallback deneniyor: %v", variant.VariantID, prov, err)
				imageURL, err = generateWithFallback(prompt, prov)
			}

			// Sonucu kaydet
			job.mu.Lock()
			result = job.Images[variant.VariantID]
			if err != nil {
				log.Printf("❌ Görsel üretilemedi (%s): %v", variant.VariantID, err)
				result.Status = "error"
				result.Error = err.Error()
			} else {
				log.Printf("✅ Görsel üretildi (%s): %s [provider: %s, süre: %v]",
					variant.VariantID, imageURL, prov, time.Since(startTime).Round(time.Second))
				result.Status = "done"
				result.ImageURL = imageURL
				atomic.AddInt32(&job.Completed, 1)

				// ── AI Vision katman üretimi devre dışı ──
				// Artık tüm metin ve logolar görselin içinde AI tarafından üretiliyor.
				// Kullanıcı isterse stüdyoda kendi katmanlarını manuel ekleyebilir.
				result.DesignLayers = []utils.DesignLayer{}
			}
			job.Images[variant.VariantID] = result
			job.mu.Unlock()

		}(v, i, provider)
	}

	// Tüm goroutine'lerin bitmesini bekle
	wg.Wait()

	// Job tamamlandı
	job.mu.Lock()
	job.Status = "completed"
	job.mu.Unlock()

	completed := atomic.LoadInt32(&job.Completed)
	elapsed := time.Since(startTime).Round(time.Second)
	log.Printf("✅ Tüm kreatif görseller üretildi — Job: %s (%d/%d) [toplam süre: %v]",
		job.JobID, completed, job.Total, elapsed)

	// 15 dakika sonra job'u temizle (memory leak önleme)
	go func() {
		time.Sleep(15 * time.Minute)
		creativeImageJobsMu.Lock()
		delete(creativeImageJobs, job.JobID)
		creativeImageJobsMu.Unlock()
	}()
}

// ──────────────────────────────────────────────
// Provider Seçimi — Akıllı Round-Robin
// Gemini ve DALL-E arasında dönüşümlü dağıtım
// ──────────────────────────────────────────────

func selectProvider(providers []string, index int) string {
	if len(providers) == 0 {
		return "gemini" // default
	}

	// AI önerisi (index 0) her zaman en hızlı provider'a git
	// Gemini genellikle daha hızlı, bu yüzden öncelikli
	hasGemini := false
	hasDalle := false
	for _, p := range providers {
		if p == "gemini" { hasGemini = true }
		if p == "dall-e" { hasDalle = true }
	}

	if hasGemini && hasDalle {
		// İkisi de var → round-robin: 0=gemini, 1=dall-e, 2=gemini, 3=dall-e...
		if index%2 == 0 {
			return "gemini"
		}
		return "dall-e"
	}

	if hasGemini {
		return "gemini"
	}
	if hasDalle {
		return "dall-e"
	}

	return providers[0]
}

// ──────────────────────────────────────────────
// Doğrudan Provider Çağrıları (Hızlı — BFL atlama)
// ──────────────────────────────────────────────

func generateWithGemini(prompt string) (string, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY tanımlı değil")
	}
	return utils.CallGeminiImageDirect(apiKey, prompt)
}

func generateWithDALLE(prompt string) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY tanımlı değil")
	}
	return utils.CallDALLEDirect(apiKey, prompt)
}

// generateWithFallback — Birincil provider başarısız olunca diğerini dene
func generateWithFallback(prompt string, failedProvider string) (string, error) {
	switch failedProvider {
	case "dall-e":
		// DALL-E başarısız → Gemini dene
		imageURL, err := generateWithGemini(prompt)
		if err == nil {
			return imageURL, nil
		}
		return "", fmt.Errorf("tüm provider'lar başarısız (dall-e + gemini)")
	case "gemini":
		// Gemini başarısız → DALL-E dene
		imageURL, err := generateWithDALLE(prompt)
		if err == nil {
			return imageURL, nil
		}
		return "", fmt.Errorf("tüm provider'lar başarısız (gemini + dall-e)")
	default:
		return "", fmt.Errorf("provider: %s — fallback mevcut değil", failedProvider)
	}
}

// ──────────────────────────────────────────────
// Yardımcılar
// ──────────────────────────────────────────────

func generateJobID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%s_%s", time.Now().Format("20060102150405"), hex.EncodeToString(b))
}

// buildFallbackLayers — AI Vision analizi başarısız olduğunda
// güvenli varsayılan katmanlar oluşturur (çakışma olmayan konumlar)
func buildFallbackLayers(variant CreativeImageVariantInput, company models.Company) []utils.DesignLayer {
	// Varsayılan font: marka fontu varsa onu kullan
	headlineFont := "Montserrat"
	if company.BrandFont != "" {
		headlineFont = company.BrandFont
	}

	layers := []utils.DesignLayer{
		{
			ID:            "headline",
			Type:          "headline",
			Text:          variant.Headline,
			X:             5,
			Y:             35,
			MaxWidth:      75,
			FontFamily:    headlineFont,
			FontSize:      36,
			FontWeight:    700,
			Color:         "#ffffff",
			TextAlign:     "left",
			LineHeight:    1.15,
			Opacity:       0.95,
			TextShadow:    true,
			LetterSpacing: 0.5,
		},
	}

	if variant.SubHeadline != "" {
		// Sub headline için kısa versiyon (max 15 kelime)
		subText := variant.SubHeadline
		words := splitWords(subText)
		if len(words) > 15 {
			subText = joinWords(words[:15]) + "…"
		}

		layers = append(layers, utils.DesignLayer{
			ID:         "desc",
			Type:       "description",
			Text:       subText,
			X:          5,
			Y:          58,
			MaxWidth:   65,
			FontFamily: "Inter",
			FontSize:   14,
			FontWeight: 400,
			Color:      "#ffffffcc",
			TextAlign:  "left",
			LineHeight: 1.4,
			Opacity:    0.85,
			TextShadow: true,
		})
	}

	return layers
}

// splitWords — Metni kelimelere böl
func splitWords(s string) []string {
	var words []string
	word := ""
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '\n' {
			if word != "" {
				words = append(words, word)
				word = ""
			}
		} else {
			word += string(c)
		}
	}
	if word != "" {
		words = append(words, word)
	}
	return words
}

// joinWords — Kelimeleri boşlukla birleştir
func joinWords(words []string) string {
	result := ""
	for i, w := range words {
		if i > 0 {
			result += " "
		}
		result += w
	}
	return result
}

// Ensure json import is used
var _ = json.Marshal
