package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/searchconsole/v1"
)

// ──────────────────────────────────────────────
// Google OAuth2 Config
// ──────────────────────────────────────────────

func getGoogleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     config.Get("GOOGLE_CLIENT_ID"),
		ClientSecret: config.Get("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  "http://localhost:8080/api/seo/auth/google/callback",
		Scopes:       []string{"https://www.googleapis.com/auth/webmasters.readonly"},
		Endpoint:     google.Endpoint,
	}
}

// ──────────────────────────────────────────────
// GET /api/seo/auth/google/login?company_id=X
// Google OAuth2 giriş URL'sini döner
// ──────────────────────────────────────────────

func SeoGoogleLogin(c *gin.Context) {
	companyID := c.Query("company_id")
	if companyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}

	// company_id'nin geçerli bir sayı olduğunu doğrula
	if _, err := strconv.ParseUint(companyID, 10, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz company_id"})
		return
	}

	cfg := getGoogleOAuthConfig()

	// company_id'yi state parametresine göm (callback'te kullanmak için)
	state := fmt.Sprintf("company_%s", companyID)

	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	c.JSON(http.StatusOK, gin.H{
		"url": url,
	})
}

// ──────────────────────────────────────────────
// GET /api/seo/auth/google/callback
// Google'dan dönen kodu alır, token'a çevirir ve kaydeder
// ──────────────────────────────────────────────

func SeoGoogleCallback(c *gin.Context) {
	frontendURL := "http://localhost:3000"

	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		log.Printf("❌ Callback: Authorization code bulunamadı. State=%s", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?error=no_auth_code", frontendURL))
		return
	}

	// State'ten company_id'yi çıkar: "company_123" → "123"
	var companyIDStr string
	if _, err := fmt.Sscanf(state, "company_%s", &companyIDStr); err != nil || companyIDStr == "" {
		log.Printf("❌ Callback: Geçersiz state parametresi: '%s', err: %v", state, err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?error=invalid_state", frontendURL))
		return
	}

	// company_id'yi uint'e çevir — güvenli parsing
	companyID64, err := strconv.ParseUint(companyIDStr, 10, 64)
	if err != nil || companyID64 == 0 {
		log.Printf("❌ Callback: company_id parse hatası: '%s', err: %v", companyIDStr, err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?error=invalid_company_id", frontendURL))
		return
	}
	companyID := uint(companyID64)

	log.Printf("🔄 OAuth callback başladı: company_id=%d, code_length=%d", companyID, len(code))

	cfg := getGoogleOAuthConfig()

	// Code → Token
	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("❌ Google token exchange hatası: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?error=token_exchange_failed", frontendURL))
		return
	}

	log.Printf("✅ Token alındı: AccessToken=%s..., RefreshToken=%s..., Expiry=%v",
		token.AccessToken[:min(10, len(token.AccessToken))],
		token.RefreshToken[:min(10, len(token.RefreshToken))],
		token.Expiry,
	)

	// GSC'den site listesini çek ve ilk site'ı otomatik seç
	client := cfg.Client(context.Background(), token)
	svc, err := searchconsole.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		log.Printf("❌ Search Console servis hatası: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?error=gsc_service_failed", frontendURL))
		return
	}

	sites, err := svc.Sites.List().Do()
	siteURL := ""
	if err != nil {
		log.Printf("⚠️ GSC site listesi alınamadı (devam ediliyor): %v", err)
	} else if len(sites.SiteEntry) > 0 {
		siteURL = sites.SiteEntry[0].SiteUrl
		log.Printf("📍 Site bulundu: %s (toplam %d site)", siteURL, len(sites.SiteEntry))
	} else {
		log.Printf("⚠️ GSC'de hiç site bulunamadı, devam ediliyor")
	}

	// Mevcut entegrasyonu güncelle veya oluştur
	var integration models.SeoIntegration
	result := database.DB.Where("company_id = ? AND provider = ?", companyID, "google").First(&integration)

	if result.Error != nil {
		// Yeni kayıt oluştur
		integration = models.SeoIntegration{
			CompanyID:    companyID,
			Provider:     "google",
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			TokenExpiry:  token.Expiry,
			SiteURL:      siteURL,
		}

		if err := database.DB.Create(&integration).Error; err != nil {
			log.Printf("❌ DB Create hatası: %v", err)
			c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?error=db_create_failed", frontendURL))
			return
		}
		log.Printf("✅ Yeni GSC entegrasyonu oluşturuldu: ID=%d, company_id=%d, site=%s", integration.ID, companyID, siteURL)
	} else {
		// Mevcut kaydı güncelle
		integration.AccessToken = token.AccessToken
		if token.RefreshToken != "" {
			integration.RefreshToken = token.RefreshToken
		}
		integration.TokenExpiry = token.Expiry
		if siteURL != "" {
			integration.SiteURL = siteURL
		}

		if err := database.DB.Save(&integration).Error; err != nil {
			log.Printf("❌ DB Save hatası: %v", err)
			c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?error=db_save_failed", frontendURL))
			return
		}
		log.Printf("✅ GSC entegrasyonu güncellendi: ID=%d, company_id=%d, site=%s", integration.ID, companyID, siteURL)
	}

	// Doğrulama: kaydın gerçekten oluştuğunu kontrol et
	var check models.SeoIntegration
	if err := database.DB.Where("company_id = ? AND provider = ?", companyID, "google").First(&check).Error; err != nil {
		log.Printf("❌ DB doğrulama hatası: Kayıt oluşturuldu ama tekrar okunamadı! err: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?error=db_verify_failed", frontendURL))
		return
	}
	log.Printf("✅ DB doğrulama başarılı: ID=%d, CompanyID=%d, AccessToken=%s...", check.ID, check.CompanyID, check.AccessToken[:min(10, len(check.AccessToken))])

	// Başarılı — Frontend'e yönlendir
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?seo_connected=true&company_id=%d", frontendURL, companyID))
}

// ──────────────────────────────────────────────
// GET /api/seo/integration/status?company_id=X
// Bağlantı durumunu döner
// ──────────────────────────────────────────────

func SeoIntegrationStatus(c *gin.Context) {
	companyID, _ := c.Get("companyID")

	var integration models.SeoIntegration
	err := database.DB.Where("company_id = ? AND provider = ?", companyID, "google").First(&integration).Error

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"connected": false,
			"provider":  "google",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connected":  true,
		"provider":   "google",
		"site_url":   integration.SiteURL,
		"created_at": integration.CreatedAt,
	})
}

// ──────────────────────────────────────────────
// GET /api/seo/dashboard/metrics?company_id=X
// Son 30 gün GSC metriklerini çeker
// ──────────────────────────────────────────────

func SeoDashboardMetrics(c *gin.Context) {
	companyID, _ := c.Get("companyID")

	// Token'ı al
	var integration models.SeoIntegration
	if err := database.DB.Where("company_id = ? AND provider = ?", companyID, "google").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "GSC entegrasyonu bulunamadı. Lütfen önce bağlantı kurun."})
		return
	}

	siteURL := c.Query("site_url")
	if siteURL == "" {
		siteURL = integration.SiteURL
	}

	if siteURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Site URL tanımlı değil"})
		return
	}

	// OAuth2 token oluştur
	token := &oauth2.Token{
		AccessToken:  integration.AccessToken,
		RefreshToken: integration.RefreshToken,
		Expiry:       integration.TokenExpiry,
		TokenType:    "Bearer",
	}

	cfg := getGoogleOAuthConfig()
	tokenSource := cfg.TokenSource(context.Background(), token)

	// Token yenilendiyse DB'ye geri yaz
	newToken, err := tokenSource.Token()
	if err != nil {
		log.Printf("❌ Token yenileme hatası: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "GSC token süresi dolmuş. Lütfen yeniden bağlanın."})
		return
	}

	if newToken.AccessToken != integration.AccessToken {
		integration.AccessToken = newToken.AccessToken
		integration.TokenExpiry = newToken.Expiry
		if newToken.RefreshToken != "" {
			integration.RefreshToken = newToken.RefreshToken
		}
		database.DB.Save(&integration)
	}

	// Search Console API çağrısı
	client := oauth2.NewClient(context.Background(), tokenSource)
	svc, err := searchconsole.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search Console servisi başlatılamadı"})
		return
	}

	endDate := time.Now().AddDate(0, 0, -1) // dün
	startDate := endDate.AddDate(0, 0, -29) // 30 gün

	// 1) Genel özet (toplam)
	summaryReq := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate: startDate.Format("2006-01-02"),
		EndDate:   endDate.Format("2006-01-02"),
	}

	summaryResp, err := svc.Searchanalytics.Query(siteURL, summaryReq).Do()
	if err != nil {
		log.Printf("❌ GSC API hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "GSC verileri alınamadı: " + err.Error()})
		return
	}

	// Özet metrikleri hesapla
	summary := map[string]interface{}{
		"clicks":      0.0,
		"impressions": 0.0,
		"ctr":         0.0,
		"position":    0.0,
	}

	if len(summaryResp.Rows) > 0 {
		row := summaryResp.Rows[0]
		summary["clicks"] = row.Clicks
		summary["impressions"] = row.Impressions
		summary["ctr"] = row.Ctr
		summary["position"] = row.Position
	}

	// 2) Günlük trend verisi
	trendReq := &searchconsole.SearchAnalyticsQueryRequest{
		StartDate:  startDate.Format("2006-01-02"),
		EndDate:    endDate.Format("2006-01-02"),
		Dimensions: []string{"date"},
	}

	trendResp, err := svc.Searchanalytics.Query(siteURL, trendReq).Do()
	if err != nil {
		log.Printf("⚠️ GSC trend verisi alınamadı: %v", err)
		// Trend alınamazsa bile özet verisini dön
		c.JSON(http.StatusOK, gin.H{
			"summary": summary,
			"trend":   []interface{}{},
			"period": gin.H{
				"start": startDate.Format("2006-01-02"),
				"end":   endDate.Format("2006-01-02"),
			},
		})
		return
	}

	type TrendPoint struct {
		Date        string  `json:"date"`
		Clicks      float64 `json:"clicks"`
		Impressions float64 `json:"impressions"`
		CTR         float64 `json:"ctr"`
		Position    float64 `json:"position"`
	}

	var trend []TrendPoint
	for _, row := range trendResp.Rows {
		if len(row.Keys) > 0 {
			trend = append(trend, TrendPoint{
				Date:        row.Keys[0],
				Clicks:      row.Clicks,
				Impressions: row.Impressions,
				CTR:         row.Ctr,
				Position:    row.Position,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"summary": summary,
		"trend":   trend,
		"period": gin.H{
			"start": startDate.Format("2006-01-02"),
			"end":   endDate.Format("2006-01-02"),
		},
		"site_url": siteURL,
	})
}

// ──────────────────────────────────────────────
// DELETE /api/seo/integration/disconnect?company_id=X
// GSC bağlantısını kaldırır
// ──────────────────────────────────────────────

func SeoDisconnect(c *gin.Context) {
	companyID, _ := c.Get("companyID")

	result := database.DB.Where("company_id = ? AND provider = ?", companyID, "google").Delete(&models.SeoIntegration{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bağlantı bulunamadı"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "GSC bağlantısı başarıyla kaldırıldı"})
}

// ──────────────────────────────────────────────
// GET /api/seo/sites?company_id=X
// GSC'deki tüm site listesini döner
// ──────────────────────────────────────────────

func SeoListSites(c *gin.Context) {
	companyID, _ := c.Get("companyID")

	var integration models.SeoIntegration
	if err := database.DB.Where("company_id = ? AND provider = ?", companyID, "google").First(&integration).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "GSC entegrasyonu bulunamadı"})
		return
	}

	token := &oauth2.Token{
		AccessToken:  integration.AccessToken,
		RefreshToken: integration.RefreshToken,
		Expiry:       integration.TokenExpiry,
		TokenType:    "Bearer",
	}

	cfg := getGoogleOAuthConfig()
	client := cfg.Client(context.Background(), token)

	svc, err := searchconsole.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Search Console servisi başlatılamadı"})
		return
	}

	sites, err := svc.Sites.List().Do()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Site listesi alınamadı: " + err.Error()})
		return
	}

	type SiteInfo struct {
		URL             string `json:"url"`
		PermissionLevel string `json:"permission_level"`
	}

	var siteList []SiteInfo
	for _, s := range sites.SiteEntry {
		siteList = append(siteList, SiteInfo{
			URL:             s.SiteUrl,
			PermissionLevel: s.PermissionLevel,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"sites":        siteList,
		"current_site": integration.SiteURL,
	})
}

// Compile-time check for json import usage
var _ = json.Marshal
