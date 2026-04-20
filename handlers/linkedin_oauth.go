package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// ══════════════════════════════════════════════════
// LinkedIn OAuth2 Config
// ══════════════════════════════════════════════════

// getLinkedInRedirectURI — ortama göre (production vs local) LinkedIn redirect URI döner
// LinkedIn Developer Portal'da kayıtlı URL'ler frontend domain'ine aittir:
//   - http://localhost:3000/api/auth/callback/linkedin
//   - https://app.dijitalisortagi.com/api/auth/callback/linkedin
// Bu yüzden frontend URL kullanılır; frontend Next.js API route ile callback'i backend'e proxy eder.
func getLinkedInRedirectURI(c *gin.Context) string {
	_, frontendURL := getBaseURLs(c)
	return frontendURL + "/api/auth/callback/linkedin"
}

// getLinkedInOAuthURL — LinkedIn OAuth2 giriş URL'sini oluşturur
func getLinkedInOAuthURL(c *gin.Context, state string) string {
	redirectURI := getLinkedInRedirectURI(c)

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", config.Get("LINKEDIN_CLIENT_ID"))
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)
	// LinkedIn Marketing API — reklam + organizasyon + profil + sosyal erişimi
	params.Set("scope", "openid profile r_organization_admin r_organization_social r_ads r_ads_reporting")

	return "https://www.linkedin.com/oauth/v2/authorization?" + params.Encode()
}

// ══════════════════════════════════════════════════
// GET /api/linkedin/auth/login?company_id=X
// LinkedIn OAuth2 giriş URL'sini döner
// ══════════════════════════════════════════════════

func LinkedInOAuthLogin(c *gin.Context) {
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

	// state: prefix + user_id + company_id
	state := fmt.Sprintf("linkedin_%v_%s", userID, companyID)

	oauthURL := getLinkedInOAuthURL(c, state)

	log.Printf("[LINKEDIN OAuth] Login URL oluşturuldu: user_id=%v, company_id=%s", userID, companyID)

	c.JSON(http.StatusOK, gin.H{
		"url": oauthURL,
	})
}

// ══════════════════════════════════════════════════
// GET /api/auth/callback/linkedin (PUBLIC)
// LinkedIn'den dönen kodu alır, token'a çevirir, ad accounts çeker, kaydeder
// ══════════════════════════════════════════════════

func LinkedInOAuthCallback(c *gin.Context) {
	_, frontendURL := getBaseURLs(c)

	code := c.Query("code")
	state := c.Query("state")
	errorParam := c.Query("error")
	errorDesc := c.Query("error_description")

	// Kullanıcı izni reddetti
	if errorParam != "" {
		log.Printf("[LINKEDIN OAuth] Kullanıcı izni reddetti: %s — %s", errorParam, errorDesc)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_error=access_denied#dashboard", frontendURL))
		return
	}

	if code == "" {
		log.Printf("[LINKEDIN OAuth] Authorization code bulunamadı. State=%s", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_error=no_auth_code#dashboard", frontendURL))
		return
	}

	// State'ten user_id ve company_id çıkar: "linkedin_123_456"
	parts := splitState(state)
	if len(parts) < 3 || parts[0] != "linkedin" {
		log.Printf("[LINKEDIN OAuth] Geçersiz state format: '%s'", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_error=invalid_state#dashboard", frontendURL))
		return
	}

	userIDParsed, err1 := strconv.ParseUint(parts[1], 10, 64)
	companyIDParsed, err2 := strconv.ParseUint(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		log.Printf("[LINKEDIN OAuth] State parse hatası: uid='%s', cid='%s'", parts[1], parts[2])
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_error=invalid_state#dashboard", frontendURL))
		return
	}
	userID := uint(userIDParsed)
	companyID := uint(companyIDParsed)

	log.Printf("[LINKEDIN OAuth] Callback başladı: user_id=%d, company_id=%d", userID, companyID)

	// ── Code → Access Token ──
	redirectURI := getLinkedInRedirectURI(c)

	tokenURL := "https://www.linkedin.com/oauth/v2/accessToken"
	tokenParams := url.Values{}
	tokenParams.Set("grant_type", "authorization_code")
	tokenParams.Set("code", code)
	tokenParams.Set("client_id", config.Get("LINKEDIN_CLIENT_ID"))
	tokenParams.Set("client_secret", config.Get("LINKEDIN_CLIENT_SECRET"))
	tokenParams.Set("redirect_uri", redirectURI)

	tokenResp, err := http.PostForm(tokenURL, tokenParams)
	if err != nil {
		log.Printf("[LINKEDIN OAuth] Token exchange HTTP hatası: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_error=token_exchange_failed#dashboard", frontendURL))
		return
	}
	defer tokenResp.Body.Close()

	tokenBody, _ := io.ReadAll(tokenResp.Body)

	var tokenData struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token,omitempty"`
		Scope        string `json:"scope,omitempty"`
		Error        string `json:"error,omitempty"`
		ErrorDesc    string `json:"error_description,omitempty"`
	}
	if err := json.Unmarshal(tokenBody, &tokenData); err != nil {
		log.Printf("[LINKEDIN OAuth] Token JSON parse hatası: %v — body: %s", err, string(tokenBody))
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_error=token_parse_failed#dashboard", frontendURL))
		return
	}

	if tokenData.Error != "" || tokenData.AccessToken == "" {
		log.Printf("[LINKEDIN OAuth] Token alınamadı: %s — %s", tokenData.Error, tokenData.ErrorDesc)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_error=token_failed#dashboard", frontendURL))
		return
	}

	log.Printf("[LINKEDIN OAuth] ✅ Token alındı: %s..., expires_in=%d saniye, scope=%s",
		tokenData.AccessToken[:min(15, len(tokenData.AccessToken))], tokenData.ExpiresIn, tokenData.Scope)

	// ── LinkedIn Ad Accounts Listesi ──
	// LinkedIn Marketing API: GET /rest/adAccounts
	accountURN, accountName := fetchLinkedInAdAccount(tokenData.AccessToken)

	// ── Token Süresini Hesapla ──
	var tokenExpiry *time.Time
	if tokenData.ExpiresIn > 0 {
		exp := time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second)
		tokenExpiry = &exp
	}

	// ── DB'ye Kaydet (company_id bazlı) ──
	var liAccount models.UserLinkedInAccount
	result := database.DB.Where("user_id = ? AND company_id = ?", userID, companyID).First(&liAccount)

	if result.Error != nil {
		// Yeni kayıt
		liAccount = models.UserLinkedInAccount{
			UserID:         userID,
			CompanyID:      companyID,
			AccessToken:    tokenData.AccessToken,
			AccountURN:     strPtr(accountURN),
			AccountName:    strPtr(accountName),
			TokenExpiresAt: tokenExpiry,
		}
		if err := database.DB.Create(&liAccount).Error; err != nil {
			log.Printf("[LINKEDIN OAuth] DB Create hatası: %v", err)
			c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_error=db_failed#dashboard", frontendURL))
			return
		}
		log.Printf("[LINKEDIN OAuth] ✅ Yeni LinkedIn hesabı kaydedildi: user_id=%d, company_id=%d, account_urn=%s", userID, companyID, accountURN)
	} else {
		// Mevcut kaydı güncelle
		liAccount.AccessToken = tokenData.AccessToken
		liAccount.TokenExpiresAt = tokenExpiry
		if accountURN != "" {
			liAccount.AccountURN = strPtr(accountURN)
			liAccount.AccountName = strPtr(accountName)
		}
		if err := database.DB.Save(&liAccount).Error; err != nil {
			log.Printf("[LINKEDIN OAuth] DB Save hatası: %v", err)
			c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_error=db_failed#dashboard", frontendURL))
			return
		}
		log.Printf("[LINKEDIN OAuth] ✅ LinkedIn hesabı güncellendi: user_id=%d, company_id=%d, account_urn=%s", userID, companyID, accountURN)
	}

	// Başarılı — Frontend dashboard'a yönlendir
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?linkedin_connected=true#dashboard", frontendURL))
}

// ══════════════════════════════════════════════════
// LinkedIn Ad Account Çekme
// ══════════════════════════════════════════════════

// fetchLinkedInAdAccount — LinkedIn Marketing API'den ilk aktif ad account'u çeker
func fetchLinkedInAdAccount(accessToken string) (accountURN, accountName string) {
	// LinkedIn Marketing API v2 — Ad Accounts listesi
	apiURL := "https://api.linkedin.com/rest/adAccounts?q=search&search=(status:(values:List(ACTIVE)))&count=10"

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Printf("[LINKEDIN OAuth] Ad Accounts request oluşturulamadı: %v", err)
		return "", ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", "202601")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[LINKEDIN OAuth] Ad Accounts API hatası: %v", err)
		return "", ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[LINKEDIN OAuth] Ad Accounts yanıtı (status=%d): %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("[LINKEDIN OAuth] ⚠️ Ad Accounts API başarısız (status %d)", resp.StatusCode)
		// İkinci yol: basit endpoint deneyelim
		return fetchLinkedInAdAccountSimple(accessToken)
	}

	var adAccountResp struct {
		Elements []struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Status   string `json:"status"`
			Currency string `json:"currency,omitempty"`
		} `json:"elements"`
	}

	if err := json.Unmarshal(body, &adAccountResp); err != nil {
		log.Printf("[LINKEDIN OAuth] Ad Accounts JSON parse hatası: %v", err)
		return fetchLinkedInAdAccountSimple(accessToken)
	}

	if len(adAccountResp.Elements) > 0 {
		acc := adAccountResp.Elements[0]
		urn := fmt.Sprintf("urn:li:sponsoredAccount:%d", acc.ID)
		log.Printf("[LINKEDIN OAuth] ✅ Ad Account bulundu: urn=%s, name=%s, status=%s", urn, acc.Name, acc.Status)
		return urn, acc.Name
	}

	log.Printf("[LINKEDIN OAuth] ⚠️ Aktif ad account bulunamadı, basit endpoint deneniyor...")
	return fetchLinkedInAdAccountSimple(accessToken)
}

// fetchLinkedInAdAccountSimple — Alternatif basit ad accounts endpoint'i
func fetchLinkedInAdAccountSimple(accessToken string) (accountURN, accountName string) {
	apiURL := "https://api.linkedin.com/rest/adAccounts?q=search&count=10"

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Printf("[LINKEDIN OAuth] Simple Ad Accounts request hatası: %v", err)
		return "", ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", "202601")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[LINKEDIN OAuth] Simple Ad Accounts API hatası: %v", err)
		return "", ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[LINKEDIN OAuth] Simple Ad Accounts yanıtı (status=%d): %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("[LINKEDIN OAuth] ⚠️ Simple Ad Accounts de başarısız, account olmadan devam ediliyor")
		return "", ""
	}

	var adAccountResp struct {
		Elements []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"elements"`
	}

	if err := json.Unmarshal(body, &adAccountResp); err == nil && len(adAccountResp.Elements) > 0 {
		acc := adAccountResp.Elements[0]
		urn := fmt.Sprintf("urn:li:sponsoredAccount:%d", acc.ID)
		log.Printf("[LINKEDIN OAuth] ✅ Simple endpoint'ten Ad Account bulundu: urn=%s, name=%s", urn, acc.Name)
		return urn, acc.Name
	}

	log.Printf("[LINKEDIN OAuth] ⚠️ Hiç ad account bulunamadı, OAuth hesabı account olmadan kaydedilecek")
	return "", ""
}
