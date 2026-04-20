package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/services"
)

// ══════════════════════════════════════════════════
// LinkedIn Ads Entegrasyon Durumu
// ══════════════════════════════════════════════════

// LinkedInIntegrationStatus — LinkedIn bağlantı durumunu kontrol eder
// GET /api/linkedin/status?company_id=X
func LinkedInIntegrationStatus(c *gin.Context) {
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

	var liAccount models.UserLinkedInAccount
	err := database.DB.Where("user_id = ? AND company_id = ? AND access_token IS NOT NULL AND access_token != ''", userID, companyID).
		First(&liAccount).Error

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"connected":    false,
			"account_urn":  nil,
			"account_name": nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"connected":    true,
		"account_urn":  liAccount.AccountURN,
		"account_name": liAccount.AccountName,
	})
}

// ══════════════════════════════════════════════════
// LinkedIn Dashboard
// ══════════════════════════════════════════════════

// LinkedInDashboardHandler — LinkedIn Ads dashboard verileri
// GET /api/linkedin/dashboard?company_id=X
func LinkedInDashboardHandler(c *gin.Context) {
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

	// LinkedIn account bilgileri
	liAccount, err := getLinkedInAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "LinkedIn hesabı bulunamadı. Lütfen önce LinkedIn hesabınızı bağlayın."})
		return
	}

	// Account URN yoksa — OAuth başarılı ama ad account bulunamadı
	if liAccount.AccountURN == nil || *liAccount.AccountURN == "" {
		log.Printf("[LINKEDIN] Dashboard: account_urn bulunamadı (user=%v), sıfır veriler dönülüyor", userID)
		c.JSON(http.StatusOK, gin.H{
			"insights": gin.H{
				"spend":       0,
				"clicks":      0,
				"impressions": 0,
				"conversions": 0,
			},
			"warning": "LinkedIn Ad Account bulunamadı. Reklam hesabınız varsa LinkedIn Developer Portal'da gerekli izinleri kontrol edin.",
		})
		return
	}

	// Days parametresi (geri dönük uyumluluk)
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

	// LinkedIn API'den insights çek
	insights, err := services.FetchLinkedInAccountInsights(liAccount.AccessToken, *liAccount.AccountURN, since, until)
	if err != nil {
		log.Printf("[LINKEDIN] Dashboard hatası (user=%v): %v", userID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "LinkedIn Ads verileri alınamadı: " + err.Error()})
		return
	}

	campaigns, errCamp := services.FetchLinkedInCampaigns(liAccount.AccessToken, *liAccount.AccountURN, since, until)
	if errCamp != nil {
		log.Printf("[LINKEDIN] Kampanyalar alınamadı (user=%v): %v", userID, errCamp)
		// Hata olsa da dashboard insights'ı dönmeye devam et
		if campaigns == nil {
			campaigns = []services.LinkedInCampaign{}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"insights": gin.H{
			"spend":       insights.Spend,
			"clicks":      insights.Clicks,
			"impressions": insights.Impressions,
			"conversions": insights.Conversions,
		},
		"campaigns": campaigns,
	})
}

// ══════════════════════════════════════════════════
// LinkedIn Bağlantısını Kes
// ══════════════════════════════════════════════════

// LinkedInDisconnect — LinkedIn hesap bağlantısını kaldırır
// DELETE /api/linkedin/disconnect?company_id=X
func LinkedInDisconnect(c *gin.Context) {
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

	result := database.DB.Where("user_id = ? AND company_id = ?", userID, companyID).Delete(&models.UserLinkedInAccount{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "LinkedIn bağlantısı bulunamadı"})
		return
	}

	log.Printf("[LINKEDIN] ✅ Bağlantı kesildi: user_id=%v, company_id=%d", userID, companyID)
	c.JSON(http.StatusOK, gin.H{"message": "LinkedIn bağlantısı başarıyla kaldırıldı"})
}

// ══════════════════════════════════════════════════
// LinkedIn Sosyal Medya Verileri (Organization API)
// ══════════════════════════════════════════════════

// LinkedInSocialDataHandler — LinkedIn şirket sayfası sosyal medya verileri
// GET /api/linkedin/social?company_id=X
func LinkedInSocialDataHandler(c *gin.Context) {
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

	// LinkedIn hesabı
	liAccount, err := getLinkedInAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "LinkedIn hesabı bulunamadı"})
		return
	}

	// Seçili reklam hesabının Organization URN'sini bul
	orgURN := ""
	if liAccount.AccountURN != nil {
		adAccounts, _ := services.FetchLinkedInAdAccounts(liAccount.AccessToken)
		for _, acc := range adAccounts {
			if acc.URN == *liAccount.AccountURN {
				orgURN = acc.Reference
				break
			}
		}
	}

	// Organization sosyal verileri çek
	socialData, err := services.FetchLinkedInOrgSocialData(liAccount.AccessToken, orgURN)
	if err != nil {
		log.Printf("[LINKEDIN] Sosyal veri hatası (user=%v): %v", userID, err)
		c.JSON(http.StatusOK, gin.H{
			"followers":  0,
			"engagement": 0,
			"posts":      0,
			"org_name":   "",
			"warning":    "LinkedIn organizasyon verisi alınamadı: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"followers":      socialData.Followers,
		"total_views":    socialData.TotalViews,
		"desktop_views":  socialData.DesktopViews,
		"mobile_views":   socialData.MobileViews,
		"overview_views": socialData.OverviewViews,
		"about_views":    socialData.AboutViews,
		"engagement":     socialData.Engagement,
		"posts":          socialData.Posts,
		"org_name":       socialData.OrgName,
	})
}

// ══════════════════════════════════════════════════
// Helper
// ══════════════════════════════════════════════════

func getLinkedInAccount(userID interface{}, companyID int) (*models.UserLinkedInAccount, error) {
	var account models.UserLinkedInAccount
	err := database.DB.
		Where("user_id = ? AND company_id = ? AND access_token != ''", userID, companyID).
		First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// ══════════════════════════════════════════════════
// LinkedIn Ad Hesaplarını Listele
// ══════════════════════════════════════════════════

// LinkedInListAdAccountsHandler — Kullanıcının tüm LinkedIn reklam hesaplarını listeler
// GET /api/linkedin/ad-accounts?company_id=X
func LinkedInListAdAccountsHandler(c *gin.Context) {
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

	liAccount, err := getLinkedInAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "LinkedIn hesabı bulunamadı"})
		return
	}

	// LinkedIn API'den tüm ad hesaplarını çek
	adAccounts, err := services.FetchLinkedInAdAccounts(liAccount.AccessToken)
	if err != nil {
		log.Printf("[LINKEDIN] Ad accounts listesi hatası: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "LinkedIn reklam hesapları alınamadı"})
		return
	}

	// Seçili hesabı belirle
	selectedURN := ""
	if liAccount.AccountURN != nil {
		selectedURN = *liAccount.AccountURN
	}
	selectedName := ""
	if liAccount.AccountName != nil {
		selectedName = *liAccount.AccountName
	}

	c.JSON(http.StatusOK, gin.H{
		"ad_accounts":   adAccounts,
		"selected_urn":  selectedURN,
		"selected_name": selectedName,
		"total":         len(adAccounts),
	})
}

// LinkedInSelectAdAccountHandler — Seçili LinkedIn reklam hesabını günceller
// PUT /api/linkedin/select-account?company_id=X
func LinkedInSelectAdAccountHandler(c *gin.Context) {
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

	var body struct {
		AccountURN  string `json:"account_urn" binding:"required"`
		AccountName string `json:"account_name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_urn gerekli"})
		return
	}

	result := database.DB.Model(&models.UserLinkedInAccount{}).
		Where("user_id = ? AND company_id = ?", userID, companyID).
		Updates(map[string]interface{}{
			"account_urn":  body.AccountURN,
			"account_name": body.AccountName,
		})

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "LinkedIn hesabı bulunamadı"})
		return
	}

	log.Printf("[LINKEDIN] ✅ Ad account değiştirildi: user=%v, company=%d, account=%s (%s)",
		userID, companyID, body.AccountURN, body.AccountName)

	c.JSON(http.StatusOK, gin.H{
		"message":      "LinkedIn reklam hesabı güncellendi",
		"account_urn":  body.AccountURN,
		"account_name": body.AccountName,
	})
}

// ══════════════════════════════════════════════════
// LinkedIn Debug — Ham API Verileri
// ══════════════════════════════════════════════════

// LinkedInDebugHandler — LinkedIn sosyal medya verilerini ham olarak döndürür
// GET /api/linkedin/debug?company_id=X
func LinkedInDebugHandler(c *gin.Context) {
	userID, _ := c.Get("userID")

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id gerekli"})
		return
	}
	companyID, _ := strconv.Atoi(companyIDStr)

	liAccount, err := getLinkedInAccount(userID, companyID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "LinkedIn hesabı bulunamadı"})
		return
	}

	token := liAccount.AccessToken
	v := "202601"

	// 1. Organizasyonları bul
	orgResult := linkedinRawCall(token, v, "https://api.linkedin.com/rest/organizationAcls?q=roleAssignee")
	results := gin.H{
		"1_organizations": orgResult,
	}

	// İlk organizasyonun URN'ini al
	orgURN := ""
	if data, ok := orgResult.(gin.H); ok {
		if statusCode, _ := data["status"].(float64); statusCode == 200 || data["status"] == 200 {
			if rawData, ok := data["data"].(map[string]interface{}); ok {
				if elements, ok := rawData["elements"].([]interface{}); ok && len(elements) > 0 {
					if firstElem, ok := elements[0].(map[string]interface{}); ok {
						if org, ok := firstElem["organization"].(string); ok {
							orgURN = org
						}
					}
				}
			}
		}
	}

	results["2_org_urn_found"] = orgURN

	if orgURN != "" {
		// 3. Org details
		orgID := orgURN[len("urn:li:organization:"):]
		results["3_org_details"] = linkedinRawCall(token, v, "https://api.linkedin.com/rest/organizations/"+orgID)

		// 4. Followers — ENCODED URN ile (doğru yöntem)
		encodedURN := strings.ReplaceAll(orgURN, ":", "%3A")
		followersURL := "https://api.linkedin.com/rest/networkSizes/" + encodedURN + "?edgeType=COMPANY_FOLLOWED_BY_MEMBER"
		results["4_followers_encoded_url"] = followersURL
		results["4_followers_encoded"] = linkedinRawCallEncoded(token, v, orgURN)

		// 5. Stats — QueryEscape ile
		statsURL := "https://api.linkedin.com/rest/organizationPageStatistics?q=organization&organization=" + url.QueryEscape(orgURN)
		results["5_stats_url"] = statsURL
		results["5_stats"] = linkedinRawCall(token, v, statsURL)
	}

	c.JSON(http.StatusOK, results)
}

// linkedinRawCallEncoded — Followers için özel: url.URL struct ile encode koruyan istek
func linkedinRawCallEncoded(accessToken, version, orgURN string) interface{} {
	encodedURN := strings.ReplaceAll(orgURN, ":", "%3A")
	reqURL := &url.URL{
		Scheme:   "https",
		Host:     "api.linkedin.com",
		Path:     "/rest/networkSizes/" + orgURN,
		RawPath:  "/rest/networkSizes/" + encodedURN,
		RawQuery: "edgeType=COMPANY_FOLLOWED_BY_MEMBER",
	}

	log.Printf("[LINKEDIN DEBUG] Followers reqURL.String(): %s", reqURL.String())
	log.Printf("[LINKEDIN DEBUG] Followers reqURL.RawPath: %s", reqURL.RawPath)
	log.Printf("[LINKEDIN DEBUG] Followers reqURL.EscapedPath(): %s", reqURL.EscapedPath())

	req := &http.Request{
		Method: "GET",
		URL:    reqURL,
		Header: make(http.Header),
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", version)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return gin.H{"error": fmt.Sprintf("API hatası: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[LINKEDIN DEBUG] Followers yanıtı (status=%d): %s", resp.StatusCode, string(body))

	var jsonResult interface{}
	if err := json.Unmarshal(body, &jsonResult); err != nil {
		return gin.H{"status": resp.StatusCode, "raw_body": string(body)}
	}
	return gin.H{"status": resp.StatusCode, "data": jsonResult}
}

// linkedinRawCall — LinkedIn REST API'ye versiyonlu istek atar
func linkedinRawCall(accessToken, version, apiURL string) interface{} {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return gin.H{"error": fmt.Sprintf("request oluşturulamadı: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", version)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return gin.H{"error": fmt.Sprintf("API hatası: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var jsonResult interface{}
	if err := json.Unmarshal(body, &jsonResult); err != nil {
		return gin.H{"status": resp.StatusCode, "raw_body": string(body)}
	}
	return gin.H{"status": resp.StatusCode, "data": jsonResult}
}

// linkedinRawCallNoVersion — LinkedIn v2 API'ye version header OLMADAN istek atar
func linkedinRawCallNoVersion(accessToken, apiURL string) interface{} {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return gin.H{"error": fmt.Sprintf("request oluşturulamadı: %v", err)}
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return gin.H{"error": fmt.Sprintf("API hatası: %v", err)}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var jsonResult interface{}
	if err := json.Unmarshal(body, &jsonResult); err != nil {
		return gin.H{"status": resp.StatusCode, "raw_body": string(body)}
	}
	return gin.H{"status": resp.StatusCode, "data": jsonResult}
}
