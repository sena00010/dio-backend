package handlers

import (
	"bytes"
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

// TikTok App credentials
var (
	tiktokAppID    = "7630748577213644801"
	tiktokAppSecret = "0a6a68cdc3eebd32f0d748f1836b18878a87f8b2"
)

func getTikTokRedirectURI(c *gin.Context) string {
	_, frontendURL := getBaseURLs(c)
	// Notice: TikTok requires exact match of Redirect URI
	return frontendURL + "/api/auth/tiktok/callback"
}

func getTikTokOAuthURL(c *gin.Context, state string) string {
	redirectURI := getTikTokRedirectURI(c)

	appID := config.Get("TIKTOK_APP_ID")
	if appID == "" {
		appID = tiktokAppID
	}

	params := url.Values{}
	params.Set("app_id", appID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)

	return "https://business-api.tiktok.com/portal/auth?" + params.Encode()
}

// ══════════════════════════════════════════════════
// GET /api/tiktok/auth/login?company_id=X
// ══════════════════════════════════════════════════
func TikTokOAuthLogin(c *gin.Context) {
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
	state := fmt.Sprintf("tiktok_%v_%s", userID, companyID)
	oauthURL := getTikTokOAuthURL(c, state)

	log.Printf("[TIKTOK OAuth] Login URL oluşturuldu: user_id=%v, company_id=%s", userID, companyID)

	c.JSON(http.StatusOK, gin.H{
		"url": oauthURL,
	})
}

// ══════════════════════════════════════════════════
// GET /api/auth/tiktok/callback (PUBLIC)
// ══════════════════════════════════════════════════
func TikTokOAuthCallback(c *gin.Context) {
	_, frontendURL := getBaseURLs(c)

	authCode := c.Query("auth_code")
	if authCode == "" {
		authCode = c.Query("code")
	}
	state := c.Query("state")
	errorParam := c.Query("error")

	if errorParam != "" {
		log.Printf("[TIKTOK OAuth] İzin reddedildi veya hata: %s", errorParam)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?tiktok_error=access_denied#dashboard", frontendURL))
		return
	}

	if authCode == "" {
		log.Printf("[TIKTOK OAuth] Authorization code bulunamadı. State=%s", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?tiktok_error=no_auth_code#dashboard", frontendURL))
		return
	}

	parts := splitState(state)
	if len(parts) < 3 || parts[0] != "tiktok" {
		log.Printf("[TIKTOK OAuth] Geçersiz state format: '%s'", state)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?tiktok_error=invalid_state#dashboard", frontendURL))
		return
	}

	userIDParsed, _ := strconv.ParseUint(parts[1], 10, 64)
	companyIDParsed, _ := strconv.ParseUint(parts[2], 10, 64)
	userID := uint(userIDParsed)
	companyID := uint(companyIDParsed)

	appID := config.Get("TIKTOK_APP_ID")
	if appID == "" {
		appID = tiktokAppID
	}
	appSecret := config.Get("TIKTOK_APP_SECRET")
	if appSecret == "" {
		appSecret = tiktokAppSecret
	}

	log.Printf("[TIKTOK OAuth] Callback başladı: user_id=%d, company_id=%d", userID, companyID)

	tokenURL := "https://business-api.tiktok.com/open_api/v1.3/oauth2/access_token/"

	reqBody := map[string]string{
		"app_id":     appID,
		"secret":     appSecret,
		"auth_code":  authCode,
		"grant_type": "auth_code",
	}
	jsonReqBody, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest("POST", tokenURL, bytes.NewBuffer(jsonReqBody))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	tokenResp, err := client.Do(req)

	if err != nil {
		log.Printf("[TIKTOK OAuth] Token HTTP hatası: %v", err)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?tiktok_error=token_exchange_failed#dashboard", frontendURL))
		return
	}
	defer tokenResp.Body.Close()

	tokenBody, _ := io.ReadAll(tokenResp.Body)

	var tokenData struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AccessToken   string   `json:"access_token"`
			AdvertiserIDs []string `json:"advertiser_ids"`
		} `json:"data"`
	}

	if err := json.Unmarshal(tokenBody, &tokenData); err != nil {
		log.Printf("[TIKTOK OAuth] Token JSON parse hatası: %v — body: %s", err, string(tokenBody))
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?tiktok_error=token_parse_failed#dashboard", frontendURL))
		return
	}

	if tokenData.Code != 0 || tokenData.Data.AccessToken == "" {
		log.Printf("[TIKTOK OAuth] Token alınamadı: Code=%d Message=%s", tokenData.Code, tokenData.Message)
		c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?tiktok_error=token_failed#dashboard", frontendURL))
		return
	}

	accessToken := tokenData.Data.AccessToken
	var accountURN string
	if len(tokenData.Data.AdvertiserIDs) > 0 {
		accountURN = tokenData.Data.AdvertiserIDs[0]
	}

	log.Printf("[TIKTOK OAuth] ✅ Token alındı: %s...", accessToken[:min(15, len(accessToken))])

	var ttAccount models.UserTikTokAccount
	result := database.DB.Where("user_id = ? AND company_id = ?", userID, companyID).First(&ttAccount)

	if result.Error != nil {
		ttAccount = models.UserTikTokAccount{
			UserID:       userID,
			CompanyID:    companyID,
			AccessToken:  accessToken,
		}
		if accountURN != "" {
			ttAccount.AdvertiserID = strPtr(accountURN)
		}
		if err := database.DB.Create(&ttAccount).Error; err != nil {
			log.Printf("[TIKTOK OAuth] DB Create hatası: %v", err)
			c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?tiktok_error=db_failed#dashboard", frontendURL))
			return
		}
	} else {
		ttAccount.AccessToken = accessToken
		if accountURN != "" {
			ttAccount.AdvertiserID = strPtr(accountURN)
		}
		if err := database.DB.Save(&ttAccount).Error; err != nil {
			log.Printf("[TIKTOK OAuth] DB Save hatası: %v", err)
			c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?tiktok_error=db_failed#dashboard", frontendURL))
			return
		}
	}

	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/?tiktok_connected=true#dashboard", frontendURL))
}
