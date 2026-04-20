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
// Meta OAuth2 Config
// ══════════════════════════════════════════════════

const metaGraphAPIBase = "https://graph.facebook.com/v19.0"

// getMetaOAuthURL — Facebook Login URL'sini oluşturur
// getMetaRedirectURI — ortama göre (production vs local) Facebook redirect URI döner
// Facebook Developer Portal'da kayıtlı redirect URI frontend domain'ine aittir:
//   - http://localhost:3000/auth/facebook/callback
//   - https://app.dijitalisortagi.com/auth/facebook/callback
// Frontend Next.js route ile callback'i backend'e proxy eder.
func getMetaRedirectURI(c *gin.Context) string {
	_, frontendURL := getBaseURLs(c)
	return frontendURL + "/auth/facebook/callback"
}

func getMetaOAuthURL(c *gin.Context, state string) string {
	redirectURI := getMetaRedirectURI(c)

	params := url.Values{}
	params.Set("client_id", config.Get("FACEBOOK_CLIENT_ID"))
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)
	params.Set("response_type", "code")
	// Facebook Login for Business — config_id ile izinler (ads_read dahil) configuration'da tanımlı
	params.Set("config_id", config.Get("FACEBOOK_CONFIG_ID"))

	return "https://www.facebook.com/v19.0/dialog/oauth?" + params.Encode()
}

// ══════════════════════════════════════════════════
// GET /api/meta/auth/login?company_id=X
// Meta OAuth2 giriş URL'sini döner
// ══════════════════════════════════════════════════

func MetaOAuthLogin(c *gin.Context) {
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

	// state: user_id + company_id kodla
	state := fmt.Sprintf("meta_%v_%s", userID, companyID)

	oauthURL := getMetaOAuthURL(c, state)

	log.Printf("[META OAuth] Login URL oluşturuldu: user_id=%v, company_id=%s", userID, companyID)

	c.JSON(http.StatusOK, gin.H{
		"url": oauthURL,
	})
}

// ══════════════════════════════════════════════════
// GET /api/meta/auth/callback
// Facebook'tan dönen kodu alır, token'a çevirir, ad accounts listeler, kaydeder
// ══════════════════════════════════════════════════

func MetaOAuthCallback(c *gin.Context) {
	_, frontendURL := getBaseURLs(c)

	code := c.Query("code")
	state := c.Query("state")
	errorReason := c.Query("error_reason")

	// Kullanıcı izni reddetti
	if errorReason != "" {
		log.Printf("[META OAuth] Kullanıcı izni reddetti: %s", errorReason)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=access_denied#dashboard", frontendURL))
		return
	}

	if code == "" {
		log.Printf("[META OAuth] Authorization code bulunamadı. State=%s", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=no_auth_code#dashboard", frontendURL))
		return
	}

	// State'ten user_id ve company_id çıkar: "meta_123_456"
	var userIDStr, companyIDStr string
	if _, err := fmt.Sscanf(state, "meta_%s", &userIDStr); err != nil {
		log.Printf("[META OAuth] Geçersiz state: '%s'", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=invalid_state#dashboard", frontendURL))
		return
	}

	// state formatı "meta_USERID_COMPANYID" - tekrar parse et
	parts := splitState(state)
	if len(parts) < 3 {
		log.Printf("[META OAuth] Geçersiz state format: '%s'", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=invalid_state#dashboard", frontendURL))
		return
	}
	userIDParsed, err1 := strconv.ParseUint(parts[1], 10, 64)
	companyIDParsed, err2 := strconv.ParseUint(parts[2], 10, 64)
	if err1 != nil || err2 != nil {
		log.Printf("[META OAuth] State parse hatası: uid='%s', cid='%s'", parts[1], parts[2])
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=invalid_state#dashboard", frontendURL))
		return
	}
	userID := uint(userIDParsed)
	companyID := uint(companyIDParsed)
	_ = companyIDStr

	log.Printf("[META OAuth] Callback başladı: user_id=%d, company_id=%d", userID, companyID)

	// ── Code → Access Token ──
	redirectURI := getMetaRedirectURI(c)

	tokenURL := metaGraphAPIBase + "/oauth/access_token"
	tokenParams := url.Values{}
	tokenParams.Set("client_id", config.Get("FACEBOOK_CLIENT_ID"))
	tokenParams.Set("client_secret", config.Get("FACEBOOK_CLIENT_SECRET"))
	tokenParams.Set("redirect_uri", redirectURI)
	tokenParams.Set("code", code)

	tokenResp, err := http.Get(tokenURL + "?" + tokenParams.Encode())
	if err != nil {
		log.Printf("[META OAuth] Token exchange HTTP hatası: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=token_exchange_failed#dashboard", frontendURL))
		return
	}
	defer tokenResp.Body.Close()

	tokenBody, _ := io.ReadAll(tokenResp.Body)

	var tokenData struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error,omitempty"`
	}
	if err := json.Unmarshal(tokenBody, &tokenData); err != nil {
		log.Printf("[META OAuth] Token JSON parse hatası: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=token_parse_failed#dashboard", frontendURL))
		return
	}

	if tokenData.Error != nil || tokenData.AccessToken == "" {
		errMsg := "bilinmeyen hata"
		if tokenData.Error != nil {
			errMsg = tokenData.Error.Message
		}
		log.Printf("[META OAuth] Token alınamadı: %s", errMsg)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=token_failed#dashboard", frontendURL))
		return
	}

	log.Printf("[META OAuth] Token alındı: %s..., expires_in=%d saniye", tokenData.AccessToken[:min(15, len(tokenData.AccessToken))], tokenData.ExpiresIn)

	// ── Ad Hesaplarını Listele ──
	adAccountsURL := fmt.Sprintf("%s/me/adaccounts?fields=account_id,name,account_status&access_token=%s", metaGraphAPIBase, tokenData.AccessToken)
	adResp, err := http.Get(adAccountsURL)
	if err != nil {
		log.Printf("[META OAuth] Ad accounts listesi HTTP hatası: %v", err)
	}

	var adAccountID, accountName string
	if adResp != nil {
		defer adResp.Body.Close()
		adBody, _ := io.ReadAll(adResp.Body)

		var adData struct {
			Data []struct {
				AccountID     string `json:"account_id"`
				Name          string `json:"name"`
				AccountStatus int    `json:"account_status"`
			} `json:"data"`
		}
		if err := json.Unmarshal(adBody, &adData); err == nil && len(adData.Data) > 0 {
			// İlk aktif hesabı seç (account_status=1 = aktif)
			for _, acc := range adData.Data {
				if acc.AccountStatus == 1 {
					adAccountID = acc.AccountID
					accountName = acc.Name
					break
				}
			}
			// Aktif yoksa ilk hesabı al
			if adAccountID == "" {
				adAccountID = adData.Data[0].AccountID
				accountName = adData.Data[0].Name
			}
			log.Printf("[META OAuth] Ad Account bulundu: id=%s, name=%s (toplam %d hesap)", adAccountID, accountName, len(adData.Data))
		} else {
			log.Printf("[META OAuth] Ad accounts parse hatası veya boş: %v", err)
		}
	}

	// ── Token Süresini Hesapla ──
	var tokenExpiry *time.Time
	if tokenData.ExpiresIn > 0 {
		exp := time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second)
		tokenExpiry = &exp
	}

	// ── DB'ye Kaydet (company_id bazlı) ──
	var metaAccount models.UserMetaAccount
	result := database.DB.Where("user_id = ? AND company_id = ?", userID, companyID).First(&metaAccount)

	if result.Error != nil {
		// Yeni kayıt
		metaAccount = models.UserMetaAccount{
			UserID:         userID,
			CompanyID:      companyID,
			AccessToken:    tokenData.AccessToken,
			AdAccountID:    strPtr(adAccountID),
			AccountName:    strPtr(accountName),
			TokenExpiresAt: tokenExpiry,
		}
		if err := database.DB.Create(&metaAccount).Error; err != nil {
			log.Printf("[META OAuth] DB Create hatası: %v", err)
			c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=db_failed#dashboard", frontendURL))
			return
		}
		log.Printf("[META OAuth] ✅ Yeni Meta hesabı kaydedildi: user_id=%d, company_id=%d, ad_account=%s", userID, companyID, adAccountID)
	} else {
		// Mevcut kaydı güncelle
		metaAccount.AccessToken = tokenData.AccessToken
		metaAccount.TokenExpiresAt = tokenExpiry
		if adAccountID != "" {
			metaAccount.AdAccountID = strPtr(adAccountID)
			metaAccount.AccountName = strPtr(accountName)
		}
		if err := database.DB.Save(&metaAccount).Error; err != nil {
			log.Printf("[META OAuth] DB Save hatası: %v", err)
			c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_error=db_failed#dashboard", frontendURL))
			return
		}
		log.Printf("[META OAuth] ✅ Meta hesabı güncellendi: user_id=%d, company_id=%d, ad_account=%s", userID, companyID, adAccountID)
	}

	// Başarılı — Frontend dashboard'a yönlendir
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?meta_connected=true#dashboard", frontendURL))
}

// ══════════════════════════════════════════════════
// Helpers
// ══════════════════════════════════════════════════

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func splitState(state string) []string {
	// "meta_123_456" → ["meta", "123", "456"]
	result := make([]string, 0)
	current := ""
	for _, ch := range state {
		if ch == '_' {
			result = append(result, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}
