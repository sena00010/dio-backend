package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/services"
)

// ══════════════════════════════════════════════════
// TikTok Ads Entegrasyon Durumu
// ══════════════════════════════════════════════════

// TikTokIntegrationStatus — TikTok bağlantı durumunu kontrol eder
// GET /api/tiktok/status?company_id=X
func TikTokIntegrationStatus(c *gin.Context) {
	userID, _ := c.Get("userID")

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

	var ttAccount models.UserTikTokAccount
	err := database.DB.Where("user_id = ? AND company_id = ? AND access_token != ''", userID, companyID).
		First(&ttAccount).Error

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"connected":     false,
			"advertiser_id": nil,
			"account_name":  nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connected":     true,
		"advertiser_id": ttAccount.AdvertiserID,
		"account_name":  ttAccount.AccountName,
	})
}

// ══════════════════════════════════════════════════
// TikTok Dashboard
// ══════════════════════════════════════════════════

// TikTokDashboardHandler — TikTok Ads dashboard verileri
// GET /api/tiktok/dashboard?company_id=X
func TikTokDashboardHandler(c *gin.Context) {
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

	// TikTok account bilgileri
	ttAccount, err := getTikTokAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "TikTok hesabı bulunamadı. Lütfen önce TikTok hesabınızı bağlayın."})
		return
	}

	// TikTok API'den insights çek
	var advId string
	if ttAccount.AdvertiserID != nil && *ttAccount.AdvertiserID != "" {
		advId = *ttAccount.AdvertiserID
	}
	
	if advId == "" {
		// Advertiser ID yoksa patlamaması için error dön
		c.JSON(http.StatusOK, gin.H{
			"insights": nil,
			"error": "Bu hesaba bağlı bir reklam veren (Advertiser ID) bulunamadı. Lütfen yetkileri kontrol edin.",
		})
		return
	}

	insights, err := services.FetchTikTokAccountInsights(ttAccount.AccessToken, advId)
	if err != nil {
		log.Printf("[TIKTOK] Dashboard hatası (user=%v): %v", userID, err)
		c.JSON(http.StatusOK, gin.H{
			"insights": nil,
			"error": "TikTok Ads verileri alınamadı: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"insights": gin.H{
			"spend":           insights.Spend,
			"clicks":          insights.Clicks,
			"conversions":     insights.Conversions,
			"cpa":             insights.CPA,
			"conversion_rate": insights.ConversionRate,
		},
	})
}

// ══════════════════════════════════════════════════
// TikTok Bağlantısını Kes
// ══════════════════════════════════════════════════

// TikTokDisconnect — TikTok hesap bağlantısını kaldırır
// DELETE /api/tiktok/disconnect?company_id=X
func TikTokDisconnect(c *gin.Context) {
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

	result := database.DB.Where("user_id = ? AND company_id = ?", userID, companyID).Delete(&models.UserTikTokAccount{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "TikTok bağlantısı bulunamadı"})
		return
	}

	log.Printf("[TIKTOK] ✅ Bağlantı kesildi: user_id=%v, company_id=%d", userID, companyID)
	c.JSON(http.StatusOK, gin.H{"message": "TikTok bağlantısı başarıyla kaldırıldı"})
}

// ══════════════════════════════════════════════════
// Helper
// ══════════════════════════════════════════════════

func getTikTokAccount(userID interface{}, companyID int) (*models.UserTikTokAccount, error) {
	var account models.UserTikTokAccount
	err := database.DB.
		Where("user_id = ? AND company_id = ? AND access_token != ''", userID, companyID).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// TikTokDebugHandler — TikTok API raw debug endpoint
// GET /api/tiktok/debug?company_id=X
func TikTokDebugHandler(c *gin.Context) {
	userID, _ := c.Get("userID")
	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}
	companyID, _ := strconv.Atoi(companyIDStr)

	ttAccount, err := getTikTokAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"error": "TikTok hesabı bağlı değil"})
		return
	}

	// Token'a ait reklam verenleri sorgulamayı deneyelim (OAuth2 Advertiser Endpoint)
	appID := config.Get("TIKTOK_APP_ID")
	if appID == "" {
		appID = "7630748577213644801"
	}
	secret := config.Get("TIKTOK_APP_SECRET")
	if secret == "" {
		secret = "0a6a68cdc3eebd32f0d748f1836b18878a87f8b2"
	}

	debugURL := "https://business-api.tiktok.com/open_api/v1.3/oauth2/advertiser/get/?app_id=" + appID + "&secret=" + secret
	req, _ := http.NewRequest("GET", debugURL, nil)
	req.Header.Set("Access-Token", ttAccount.AccessToken)

	client := &http.Client{}
	resp, reqErr := client.Do(req)

	var apiResponse map[string]interface{}
	if reqErr == nil {
		defer resp.Body.Close()
		json.NewDecoder(resp.Body).Decode(&apiResponse)
	}

	// İkinci bir deneme: Acaba Marketing API'de user/info diye bir endpoint var mı?
	userInfoURL := "https://business-api.tiktok.com/open_api/v1.3/user/info/"
	req2, _ := http.NewRequest("GET", userInfoURL, nil)
	req2.Header.Set("Access-Token", ttAccount.AccessToken)
	resp2, _ := client.Do(req2)
	var userInfoResponse map[string]interface{}
	if resp2 != nil {
		defer resp2.Body.Close()
		json.NewDecoder(resp2.Body).Decode(&userInfoResponse)
	}

	// Üçüncü bir deneme: Acaba Business Get endpoint'i ile Business Account bilgisi mi çekilmeli?
	bizURL := "https://business-api.tiktok.com/open_api/v1.3/business/get/"
	req3, _ := http.NewRequest("GET", bizURL, nil)
	req3.Header.Set("Access-Token", ttAccount.AccessToken)
	resp3, _ := client.Do(req3)
	var bizResponse map[string]interface{}
	if resp3 != nil {
		defer resp3.Body.Close()
		json.NewDecoder(resp3.Body).Decode(&bizResponse)
	}

	// Eğer advertiser çekiliyorsa ama bizim veritabanında yoksa, DB'ye ilkini yazalım
	if apiResponse != nil && apiResponse["code"] != nil && apiResponse["code"].(float64) == 0 {
		if data, ok := apiResponse["data"].(map[string]interface{}); ok {
			if list, ok := data["list"].([]interface{}); ok && len(list) > 0 {
				if firstAdv, ok := list[0].(map[string]interface{}); ok && firstAdv["advertiser_id"] != nil {
					advID := firstAdv["advertiser_id"].(string)
					advName, _ := firstAdv["advertiser_name"].(string)
					
					// Güncelle
					ttAccount.AdvertiserID = &advID
					ttAccount.AccountName = &advName
					database.DB.Save(&ttAccount)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token":  ttAccount.AccessToken[:5] + "...",
		"advertiser_endpoint_response": apiResponse,
		"user_info_response": userInfoResponse,
		"business_info_response": bizResponse,
		"request_error": reqErr,
        "saved_advertiser_id": ttAccount.AdvertiserID,
	})
}
