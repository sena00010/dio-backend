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
	err := database.DB.Where("user_id = ? AND company_id = ? AND advertiser_id IS NOT NULL AND advertiser_id != '' AND access_token != ''", userID, companyID).
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
	insights, err := services.FetchTikTokAccountInsights(ttAccount.AccessToken, *ttAccount.AdvertiserID)
	if err != nil {
		log.Printf("[TIKTOK] Dashboard hatası (user=%v): %v", userID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "TikTok Ads verileri alınamadı: " + err.Error()})
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
		Where("user_id = ? AND company_id = ? AND advertiser_id IS NOT NULL AND advertiser_id != '' AND access_token != ''", userID, companyID).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}
