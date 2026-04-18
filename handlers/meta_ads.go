package handlers

import (
	"log"
	"net/http"
	"strconv"

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

	// company_id bazlı Meta hesabını ara
	var metaAccount models.UserMetaAccount
	err := database.DB.Where("user_id = ? AND company_id = ? AND ad_account_id IS NOT NULL AND ad_account_id != ''", userID, companyID).
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

	// Graph API'den insights çek
	insights, err := services.FetchMetaAccountInsights(metaAccount.AccessToken, *metaAccount.AdAccountID)
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

	// Meta account bilgileri
	metaAccount, err := getMetaAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Meta hesabı bulunamadı. Lütfen önce Meta hesabınızı bağlayın."})
		return
	}

	// Graph API'den kampanyaları çek
	campaigns, err := services.FetchMetaCampaigns(metaAccount.AccessToken, *metaAccount.AdAccountID, limit)
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
		data, err := services.FetchMetaAccountInsights(metaAccount.AccessToken, *metaAccount.AdAccountID)
		insightsCh <- insightsResult{data, err}
	}()

	go func() {
		data, err := services.FetchMetaCampaigns(metaAccount.AccessToken, *metaAccount.AdAccountID, 10)
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
