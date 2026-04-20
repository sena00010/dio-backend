package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/services"
)

// ══════════════════════════════════════════════════
// Meta Ads Entegrasyon Durumu
// ══════════════════════════════════════════════════

// MetaIntegrationStatus — Meta bağlantı durumunu kontrol eder
// GET /api/meta/status?company_id=X
func MetaIntegrationStatus(c *gin.Context) {
	userID, _ := c.Get("userID")

	// company_id parametresi
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}
	companyID, _ := strconv.Atoi(companyIDStr)

	// Şirkete ait kullanıcı doğrulama
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	// company_id bazlı Meta hesabını ara (token var mı kontrol)
	var metaAccount models.UserMetaAccount
	err := database.DB.Where("user_id = ? AND company_id = ? AND access_token != ''", userID, companyID).
		First(&metaAccount).Error

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"connected":     false,
			"ad_account_id": nil,
			"account_name":  nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connected":     true,
		"ad_account_id": metaAccount.AdAccountID,
		"account_name":  metaAccount.AccountName,
	})
}

// ══════════════════════════════════════════════════
// Hesap Özeti (Account Insights)
// ══════════════════════════════════════════════════

// MetaAccountInsightsHandler — Son 30 günlük hesap performans verileri
// GET /api/meta/insights?company_id=X
func MetaAccountInsightsHandler(c *gin.Context) {
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

	// Meta account bilgileri
	metaAccount, err := getMetaAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meta hesabı bulunamadı. Lütfen önce Meta hesabınızı bağlayın."})
		return
	}

	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	since := c.Query("since")
	until := c.Query("until")
	if since == "" || until == "" {
		now := time.Now()
		since = now.AddDate(0, 0, -days).Format("2006-01-02")
		until = now.Format("2006-01-02")
	}

	// Graph API'den insights çek
	insights, err := services.FetchMetaAccountInsights(metaAccount.AccessToken, *metaAccount.AdAccountID, since, until)
	if err != nil {
		log.Printf("[META] Account insights hatası (user=%v): %v", userID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Meta Ads verileri alınamadı: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"spend":       insights.Spend,
		"revenue":     insights.Revenue,
		"roas":        insights.ROAS,
		"conversions": insights.Conversions,
	})
}

// ══════════════════════════════════════════════════
// Kampanya Listesi
// ══════════════════════════════════════════════════

// MetaCampaignsHandler — Son kampanyalar + kampanya bazlı insights
// GET /api/meta/campaigns?company_id=X&limit=10
func MetaCampaignsHandler(c *gin.Context) {
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

	// Limit parametresi
	limit := 10
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Days parametresi
	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}

	since := c.Query("since")
	until := c.Query("until")
	if since == "" || until == "" {
		now := time.Now()
		since = now.AddDate(0, 0, -days).Format("2006-01-02")
		until = now.Format("2006-01-02")
	}

	// Meta account bilgileri
	metaAccount, err := getMetaAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meta hesabı bulunamadı"})
		return
	}

	// Graph API'den kampanyaları çek
	campaigns, err := services.FetchMetaCampaigns(metaAccount.AccessToken, *metaAccount.AdAccountID, limit, since, until)
	if err != nil {
		log.Printf("[META] Campaigns hatası (user=%v): %v", userID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Meta Ads kampanyaları alınamadı: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"campaigns": campaigns,
		"total":     len(campaigns),
	})
}

// ══════════════════════════════════════════════════
// Dashboard Birleşik Endpoint (insights + campaigns tek seferde)
// ══════════════════════════════════════════════════

// MetaDashboardHandler — Dashboard için tüm Meta Ads verilerini tek istekte döner
// GET /api/meta/dashboard?company_id=X
func MetaDashboardHandler(c *gin.Context) {
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

	// Meta account bilgileri
	metaAccount, err := getMetaAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meta hesabı bulunamadı"})
		return
	}

	// Days parametresi (eski uyumluluk veya hızlı aralık için)
	days := 30
	if d := c.Query("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}
	
	since := c.Query("since")
	until := c.Query("until")
	if since == "" || until == "" {
		now := time.Now()
		since = now.AddDate(0, 0, -days).Format("2006-01-02")
		until = now.Format("2006-01-02")
	}

	// Her iki veriyi paralel çek
	type insightsResult struct {
		data *services.MetaAccountInsights
		err  error
	}
	type campaignsResult struct {
		data []services.MetaCampaignItem
		err  error
	}

	insightsCh := make(chan insightsResult, 1)
	campaignsCh := make(chan campaignsResult, 1)

	go func() {
		data, err := services.FetchMetaAccountInsights(metaAccount.AccessToken, *metaAccount.AdAccountID, since, until)
		insightsCh <- insightsResult{data, err}
	}()

	go func() {
		data, err := services.FetchMetaCampaigns(metaAccount.AccessToken, *metaAccount.AdAccountID, 10, since, until)
		campaignsCh <- campaignsResult{data, err}
	}()

	iResult := <-insightsCh
	cResult := <-campaignsCh

	response := gin.H{}

	if iResult.err != nil {
		log.Printf("[META] Dashboard insights hatası: %v", iResult.err)
		response["insights"] = nil
		response["insights_error"] = iResult.err.Error()
	} else {
		response["insights"] = gin.H{
			"spend":       iResult.data.Spend,
			"revenue":     iResult.data.Revenue,
			"roas":        iResult.data.ROAS,
			"conversions": iResult.data.Conversions,
		}
	}

	if cResult.err != nil {
		log.Printf("[META] Dashboard campaigns hatası: %v", cResult.err)
		response["campaigns"] = nil
		response["campaigns_error"] = cResult.err.Error()
	} else {
		response["campaigns"] = cResult.data
	}

	c.JSON(http.StatusOK, response)
}

// ══════════════════════════════════════════════════
// Helper
// ══════════════════════════════════════════════════

func getMetaAccount(userID interface{}, companyID int) (*models.UserMetaAccount, error) {
	var metaAccount models.UserMetaAccount
	err := database.DB.
		Where("user_id = ? AND company_id = ? AND ad_account_id IS NOT NULL AND ad_account_id != '' AND access_token != ''", userID, companyID).
		First(&metaAccount).Error
	if err != nil {
		return nil, err
	}
	return &metaAccount, nil
}

// getMetaAccountAny — ad_account_id olmasa bile (token var ise) Meta hesabını döner
func getMetaAccountAny(userID interface{}, companyID int) (*models.UserMetaAccount, error) {
	var metaAccount models.UserMetaAccount
	err := database.DB.
		Where("user_id = ? AND company_id = ? AND access_token != ''", userID, companyID).
		First(&metaAccount).Error
	if err != nil {
		return nil, err
	}
	return &metaAccount, nil
}

// ══════════════════════════════════════════════════
// Meta Ad Hesaplarını Listele
// ══════════════════════════════════════════════════

// MetaListAdAccountsHandler — Kullanıcının tüm Meta ad hesaplarını listeler
// GET /api/meta/ad-accounts?company_id=X
func MetaListAdAccountsHandler(c *gin.Context) {
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

	// Meta hesabı (token gerekli, ad_account_id olmasına gerek yok)
	metaAccount, err := getMetaAccountAny(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meta hesabı bulunamadı. Lütfen önce Meta hesabınızı bağlayın."})
		return
	}

	// Graph API'den tüm ad hesaplarını çek
	adAccounts, err := fetchAllMetaAdAccounts(metaAccount.AccessToken)
	if err != nil {
		log.Printf("[META] Ad accounts listesi hatası (user=%v): %v", userID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Meta Ad hesapları alınamadı: " + err.Error()})
		return
	}

	// Seçili hesabı belirle
	selectedID := ""
	if metaAccount.AdAccountID != nil {
		selectedID = *metaAccount.AdAccountID
	}

	c.JSON(http.StatusOK, gin.H{
		"ad_accounts": adAccounts,
		"selected_id": selectedID,
		"total":       len(adAccounts),
	})
}

// MetaSelectAdAccountHandler — Kullanıcının seçtiği Meta ad hesabını günceller
// PUT /api/meta/select-account?company_id=X
func MetaSelectAdAccountHandler(c *gin.Context) {
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

	// Request body
	var body struct {
		AdAccountID string `json:"ad_account_id" binding:"required"`
		AccountName string `json:"account_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ad_account_id gerekli"})
		return
	}

	// Meta hesabını güncelle
	result := database.DB.Model(&models.UserMetaAccount{}).
		Where("user_id = ? AND company_id = ?", userID, companyID).
		Updates(map[string]interface{}{
			"ad_account_id": body.AdAccountID,
			"account_name":  body.AccountName,
		})

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meta hesabı bulunamadı"})
		return
	}

	log.Printf("[META] ✅ Ad account değiştirildi: user_id=%v, company_id=%d, new_account=%s (%s)",
		userID, companyID, body.AdAccountID, body.AccountName)

	c.JSON(http.StatusOK, gin.H{
		"message":       "Ad hesabı başarıyla güncellendi",
		"ad_account_id": body.AdAccountID,
		"account_name":  body.AccountName,
	})
}

// fetchAllMetaAdAccounts — Meta Graph API'den tüm ad account'ları çeker (pagination dahil)
func fetchAllMetaAdAccounts(accessToken string) ([]gin.H, error) {
	allAccounts := []gin.H{}
	nextURL := fmt.Sprintf("%s/me/adaccounts?fields=account_id,name,account_status,currency,amount_spent&limit=50&access_token=%s", metaGraphAPIBase, accessToken)

	for nextURL != "" {
		resp, err := http.Get(nextURL)
		if err != nil {
			return nil, fmt.Errorf("Meta API isteği başarısız: %w", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		var result struct {
			Data []struct {
				AccountID     string `json:"account_id"`
				Name          string `json:"name"`
				AccountStatus int    `json:"account_status"`
				Currency      string `json:"currency,omitempty"`
				AmountSpent   string `json:"amount_spent,omitempty"`
			} `json:"data"`
			Paging *struct {
				Next string `json:"next,omitempty"`
			} `json:"paging,omitempty"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}

		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("JSON parse hatası: %w", err)
		}

		if result.Error != nil {
			return nil, fmt.Errorf("Meta API: %s", result.Error.Message)
		}

		for _, acc := range result.Data {
			// account_status: 1=ACTIVE, 2=DISABLED, 3=UNSETTLED, 7=PENDING_RISK_REVIEW, 8=PENDING_SETTLEMENT, 9=IN_GRACE_PERIOD, 100=PENDING_CLOSURE, 101=CLOSED, 201=ANY_ACTIVE, 202=ANY_CLOSED
			statusLabel := "Bilinmiyor"
			switch acc.AccountStatus {
			case 1:
				statusLabel = "Aktif"
			case 2:
				statusLabel = "Devre Dışı"
			case 3:
				statusLabel = "Ödeme Bekliyor"
			case 101:
				statusLabel = "Kapatılmış"
			}

			// amount_spent kuruş cinsinden gelir
			spentCents := 0.0
			if acc.AmountSpent != "" {
				fmt.Sscanf(acc.AmountSpent, "%f", &spentCents)
			}

			allAccounts = append(allAccounts, gin.H{
				"account_id": acc.AccountID,
				"name":       acc.Name,
				"status":     statusLabel,
				"status_code": acc.AccountStatus,
				"currency":   acc.Currency,
				"amount_spent": spentCents / 100.0,
			})
		}

		// Sonraki sayfa var mı?
		if result.Paging != nil && result.Paging.Next != "" {
			nextURL = result.Paging.Next
		} else {
			nextURL = ""
		}
	}

	log.Printf("[META] ✅ %d ad hesabı bulundu", len(allAccounts))
	return allAccounts, nil
}

// ══════════════════════════════════════════════════
// Meta Bağlantısını Kes
// ══════════════════════════════════════════════════

// MetaDisconnect — Meta hesap bağlantısını kaldırır
// DELETE /api/meta/disconnect?company_id=X
func MetaDisconnect(c *gin.Context) {
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

	result := database.DB.Where("user_id = ? AND company_id = ?", userID, companyID).Delete(&models.UserMetaAccount{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meta bağlantısı bulunamadı"})
		return
	}

	log.Printf("[META] ✅ Bağlantı kesildi: user_id=%v, company_id=%d", userID, companyID)
	c.JSON(http.StatusOK, gin.H{"message": "Meta bağlantısı başarıyla kaldırıldı"})
}
