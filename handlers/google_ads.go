package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/services"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ══════════════════════════════════════════════════
// Google Ads OAuth2 Config
// ══════════════════════════════════════════════════

func getGoogleAdsOAuthConfig(c *gin.Context) *oauth2.Config {
	backendURL, _ := getBaseURLs(c)
	return &oauth2.Config{
		ClientID:     config.Get("GOOGLE_CLIENT_ID"),
		ClientSecret: config.Get("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  backendURL + "/api/google/auth/callback",
		Scopes:       []string{"https://www.googleapis.com/auth/adwords"},
		Endpoint:     google.Endpoint,
	}
}

// ══════════════════════════════════════════════════
// GET /api/google-ads/auth/login?company_id=X
// ══════════════════════════════════════════════════

func GoogleAdsLogin(c *gin.Context) {
	companyID := c.Query("company_id")
	if companyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}
	if _, err := strconv.ParseUint(companyID, 10, 64); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz company_id"})
		return
	}

	userID, _ := c.Get("userID")

	cfg := getGoogleAdsOAuthConfig(c)
	state := fmt.Sprintf("gads_%v_%s", userID, companyID)

	url := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	log.Printf("[GOOGLE ADS OAuth] Login URL oluşturuldu: user_id=%v, company_id=%s", userID, companyID)
	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ══════════════════════════════════════════════════
// GET /api/google/auth/callback (PUBLIC)
// ══════════════════════════════════════════════════

func GoogleAdsCallback(c *gin.Context) {
	_, frontendURL := getBaseURLs(c)

	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		log.Printf("[GOOGLE ADS OAuth] Authorization code bulunamadı. State=%s", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?gads_error=no_auth_code#dashboard", frontendURL))
		return
	}

	parts := splitState(state)
	if len(parts) < 3 || parts[0] != "gads" {
		log.Printf("[GOOGLE ADS OAuth] Geçersiz state format: '%s'", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?gads_error=invalid_state#dashboard", frontendURL))
		return
	}

	userIDParsed, err1 := strconv.ParseUint(parts[1], 10, 64)
	companyIDParsed, err2 := strconv.ParseUint(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?gads_error=invalid_state#dashboard", frontendURL))
		return
	}
	userID := uint(userIDParsed)
	companyID := uint(companyIDParsed)

	log.Printf("[GOOGLE ADS OAuth] Callback başladı: user_id=%d, company_id=%d", userID, companyID)

	// Code → Token
	cfg := getGoogleAdsOAuthConfig(c)
	token, err := cfg.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("[GOOGLE ADS OAuth] Token exchange hatası: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?gads_error=token_exchange_failed#dashboard", frontendURL))
		return
	}

	log.Printf("[GOOGLE ADS OAuth] ✅ Token alındı: AccessToken=%s..., RefreshToken=%s...",
		token.AccessToken[:min(10, len(token.AccessToken))],
		safeSubstr(token.RefreshToken, 10),
	)

	// Erişilebilir hesapları listele
	customers, err := services.ListGoogleAdsAccounts(token.AccessToken)
	var customerID, accountName string
	if err != nil {
		log.Printf("[GOOGLE ADS OAuth] ⚠️ Müşteri listesi alınamadı: %v", err)
	} else if len(customers) > 0 {
		customerID = customers[0].ID
		accountName = customers[0].Name
		log.Printf("[GOOGLE ADS OAuth] ✅ Müşteri bulundu: id=%s, name=%s (toplam %d)", customerID, accountName, len(customers))
	} else {
		log.Printf("[GOOGLE ADS OAuth] ⚠️ Hiç erişilebilir müşteri hesabı bulunamadı")
	}

	// Token süresi
	var tokenExpiry *time.Time
	if !token.Expiry.IsZero() {
		exp := token.Expiry
		tokenExpiry = &exp
	}

	// DB'ye Kaydet
	var gadsAccount models.UserGoogleAdsAccount
	result := database.DB.Where("user_id = ? AND company_id = ?", userID, companyID).First(&gadsAccount)

	if result.Error != nil {
		gadsAccount = models.UserGoogleAdsAccount{
			UserID:              userID,
			CompanyID:           companyID,
			AccessToken:         &token.AccessToken,
			RefreshToken:        &token.RefreshToken,
			TokenExpiresAt:      tokenExpiry,
			GoogleAdsCustomerID: strPtrSafe(customerID),
			AccountName:         strPtrSafe(accountName),
		}
		database.DB.Create(&gadsAccount)
		log.Printf("[GOOGLE ADS OAuth] ✅ Yeni hesap kaydedildi: customer_id=%s", customerID)
	} else {
		gadsAccount.AccessToken = &token.AccessToken
		if token.RefreshToken != "" {
			gadsAccount.RefreshToken = &token.RefreshToken
		}
		gadsAccount.TokenExpiresAt = tokenExpiry
		if customerID != "" {
			gadsAccount.GoogleAdsCustomerID = strPtrSafe(customerID)
			gadsAccount.AccountName = strPtrSafe(accountName)
		}
		database.DB.Save(&gadsAccount)
		log.Printf("[GOOGLE ADS OAuth] ✅ Hesap güncellendi: customer_id=%s", customerID)
	}

	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?gads_connected=true#dashboard", frontendURL))
}

// ══════════════════════════════════════════════════
// GET /api/google-ads/status?company_id=X
// ══════════════════════════════════════════════════

func GoogleAdsIntegrationStatus(c *gin.Context) {
	userID, _ := c.Get("userID")
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}
	companyID, _ := strconv.Atoi(companyIDStr)

	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	var gadsAccount models.UserGoogleAdsAccount
	err := database.DB.Where("user_id = ? AND company_id = ? AND access_token IS NOT NULL", userID, companyID).
		First(&gadsAccount).Error

	if err != nil {
		c.JSON(http.StatusOK, gin.H{"connected": false, "customer_id": nil, "account_name": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connected":    true,
		"customer_id":  gadsAccount.GoogleAdsCustomerID,
		"account_name": gadsAccount.AccountName,
		"has_customer": gadsAccount.GoogleAdsCustomerID != nil && *gadsAccount.GoogleAdsCustomerID != "",
	})
}

// ══════════════════════════════════════════════════
// GET /api/google-ads/accounts?company_id=X
// Erişilebilir hesapları listeler (frontend dropdown için)
// ══════════════════════════════════════════════════

func GoogleAdsListAccounts(c *gin.Context) {
	userID, _ := c.Get("userID")
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}
	companyID, _ := strconv.Atoi(companyIDStr)

	// Şirket doğrulama
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	// Google Ads hesabını bul
	gadsAccount, err := getGoogleAdsAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Google Ads bağlantısı bulunamadı"})
		return
	}

	// Token yenile gerekirse
	accessToken, err := ensureFreshGadsToken(gadsAccount)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Token yenileme hatası: " + err.Error()})
		return
	}

	// Hesapları listele
	customers, err := services.ListGoogleAdsAccounts(accessToken)
	if err != nil {
		log.Printf("[GOOGLE ADS] Hesap listesi hatası: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Google Ads hesapları alınamadı: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"accounts":           customers,
		"selected_customer":  gadsAccount.GoogleAdsCustomerID,
	})
}

// ══════════════════════════════════════════════════
// POST /api/google-ads/select-account?company_id=X
// Kullanıcı bir hesap seçer, DB'ye kaydeder
// ══════════════════════════════════════════════════

func GoogleAdsSelectAccount(c *gin.Context) {
	userID, _ := c.Get("userID")
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}
	companyID, _ := strconv.Atoi(companyIDStr)

	var body struct {
		CustomerID  string `json:"customer_id"`
		AccountName string `json:"account_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.CustomerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "customer_id gerekli"})
		return
	}

	// Şirket doğrulama
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	// Güncelle
	result := database.DB.Model(&models.UserGoogleAdsAccount{}).
		Where("user_id = ? AND company_id = ?", userID, companyID).
		Updates(map[string]interface{}{
			"google_ads_customer_id": body.CustomerID,
			"account_name":          body.AccountName,
		})

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Google Ads bağlantısı bulunamadı"})
		return
	}

	log.Printf("[GOOGLE ADS] ✅ Hesap seçildi: user_id=%v, company_id=%d, customer_id=%s, name=%s",
		userID, companyID, body.CustomerID, body.AccountName)
	c.JSON(http.StatusOK, gin.H{"message": "Hesap seçildi", "customer_id": body.CustomerID})
}

// ══════════════════════════════════════════════════
// GET /api/google-ads/dashboard?company_id=X
// ══════════════════════════════════════════════════

func GoogleAdsDashboardHandler(c *gin.Context) {
	userID, _ := c.Get("userID")
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}
	companyID, _ := strconv.Atoi(companyIDStr)

	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	gadsAccount, err := getGoogleAdsAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Google Ads hesabı bulunamadı"})
		return
	}

	// Customer ID kontrolü — yoksa otomatik tespit et
	customerID := ""
	if gadsAccount.GoogleAdsCustomerID != nil {
		customerID = *gadsAccount.GoogleAdsCustomerID
	}
	if customerID == "" {
		// Otomatik hesap tespiti: Token yenile → hesapları listele → ilkini seç
		log.Printf("[GOOGLE ADS] customer_id NULL, otomatik tespit başlıyor...")
		freshToken, err := ensureFreshGadsToken(gadsAccount)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Token yenileme hatası: " + err.Error()})
			return
		}

		customers, err := services.ListGoogleAdsAccounts(freshToken)
		if err != nil {
			log.Printf("[GOOGLE ADS] ⚠️ Hesap listesi alınamadı: %v", err)
			c.JSON(http.StatusOK, gin.H{
				"insights":               nil,
				"campaigns":              nil,
				"needs_account_selection": true,
				"error":                  "Google Ads hesapları listelenemedi: " + err.Error(),
			})
			return
		}

		if len(customers) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"insights":               nil,
				"campaigns":              nil,
				"needs_account_selection": true,
				"message":                "Erişilebilir Google Ads hesabı bulunamadı",
			})
			return
		}

		// İlk hesabı otomatik seç ve DB'ye kaydet
		customerID = customers[0].ID
		accountName := customers[0].Name
		gadsAccount.GoogleAdsCustomerID = strPtrSafe(customerID)
		gadsAccount.AccountName = strPtrSafe(accountName)
		database.DB.Save(gadsAccount)
		log.Printf("[GOOGLE ADS] ✅ Otomatik hesap seçildi: id=%s, name=%s", customerID, accountName)
	}

	// Token yenile
	accessToken, err := ensureFreshGadsToken(gadsAccount)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Token yenileme hatası: " + err.Error()})
		return
	}

	// Paralel veri çekme
	type insightsResult struct {
		data *services.GoogleAdsAccountInsights
		err  error
	}
	type campaignsResult struct {
		data []services.GoogleAdsCampaignItem
		err  error
	}

	insightsCh := make(chan insightsResult, 1)
	campaignsCh := make(chan campaignsResult, 1)

	go func() {
		data, err := services.GetGoogleAdsDailyMetrics(accessToken, customerID)
		insightsCh <- insightsResult{data, err}
	}()

	go func() {
		data, err := services.FetchGoogleAdsCampaigns(accessToken, customerID, 10)
		campaignsCh <- campaignsResult{data, err}
	}()

	iResult := <-insightsCh
	cResult := <-campaignsCh

	response := gin.H{}

	if iResult.err != nil {
		log.Printf("[GOOGLE ADS] Dashboard insights hatası: %v", iResult.err)
		response["insights"] = nil
		response["insights_error"] = iResult.err.Error()
	} else {
		response["insights"] = gin.H{
			"spend":       iResult.data.Spend,
			"clicks":      iResult.data.Clicks,
			"impressions": iResult.data.Impressions,
			"conversions": iResult.data.Conversions,
			"cpc":         iResult.data.CPC,
			"ctr":         iResult.data.CTR,
		}
	}

	if cResult.err != nil {
		log.Printf("[GOOGLE ADS] Dashboard campaigns hatası: %v", cResult.err)
		response["campaigns"] = nil
		response["campaigns_error"] = cResult.err.Error()
	} else {
		response["campaigns"] = cResult.data
	}

	c.JSON(http.StatusOK, response)
}

// ══════════════════════════════════════════════════
// DELETE /api/google-ads/disconnect?company_id=X
// ══════════════════════════════════════════════════

func GoogleAdsDisconnect(c *gin.Context) {
	userID, _ := c.Get("userID")
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}
	companyID, _ := strconv.Atoi(companyIDStr)

	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	result := database.DB.Where("user_id = ? AND company_id = ?", userID, companyID).Delete(&models.UserGoogleAdsAccount{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Google Ads bağlantısı bulunamadı"})
		return
	}

	log.Printf("[GOOGLE ADS] ✅ Bağlantı kesildi: user_id=%v, company_id=%d", userID, companyID)
	c.JSON(http.StatusOK, gin.H{"message": "Google Ads bağlantısı başarıyla kaldırıldı"})
}

// ══════════════════════════════════════════════════
// Helpers
// ══════════════════════════════════════════════════

func getGoogleAdsAccount(userID interface{}, companyID int) (*models.UserGoogleAdsAccount, error) {
	var account models.UserGoogleAdsAccount
	err := database.DB.
		Where("user_id = ? AND company_id = ? AND access_token IS NOT NULL", userID, companyID).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// ensureFreshGadsToken — Access token süresi dolmuşsa refresh token ile yeniler
func ensureFreshGadsToken(account *models.UserGoogleAdsAccount) (string, error) {
	// Token hâlâ geçerliyse doğrudan kullan
	if account.TokenExpiresAt != nil && account.TokenExpiresAt.After(time.Now().Add(5*time.Minute)) {
		if account.AccessToken != nil {
			return *account.AccessToken, nil
		}
	}

	// Refresh token yoksa hata
	if account.RefreshToken == nil || *account.RefreshToken == "" {
		return "", fmt.Errorf("refresh token bulunamadı, yeniden bağlantı gerekli")
	}

	// Refresh token ile yeni access token al
	log.Printf("[GOOGLE ADS] Token süresi dolmuş, yenileniyor...")
	tokenResp, err := services.RefreshGoogleAccessToken(*account.RefreshToken)
	if err != nil {
		return "", err
	}

	// DB'yi güncelle
	newExpiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	account.AccessToken = &tokenResp.AccessToken
	account.TokenExpiresAt = &newExpiry
	database.DB.Save(account)

	return tokenResp.AccessToken, nil
}

func strPtrSafe(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func safeSubstr(s string, n int) string {
	if len(s) == 0 {
		return "(boş)"
	}
	if len(s) < n {
		return s
	}
	return s[:n] + "..."
}
