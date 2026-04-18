package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/config"
)

// ══════════════════════════════════════════════════
// Google Ads API Veri Yapıları
// ══════════════════════════════════════════════════

const googleAdsAPIVersion = "v23"

// GoogleAdsSearchResponse — GAQL sorgu yanıtı
type GoogleAdsSearchResponse struct {
	Results       []GoogleAdsSearchResult `json:"results"`
	FieldMask     string                  `json:"fieldMask,omitempty"`
	RequestID     string                  `json:"requestId,omitempty"`
	QueryResourceConsumption int64        `json:"queryResourceConsumption,omitempty"`
}

// GoogleAdsSearchResult — tek sonuç satırı
type GoogleAdsSearchResult struct {
	Customer *GoogleAdsCustomer `json:"customer,omitempty"`
	Campaign *GoogleAdsCampaign `json:"campaign,omitempty"`
	Metrics  GoogleAdsMetrics   `json:"metrics"`
}

type GoogleAdsCustomer struct {
	ResourceName    string `json:"resourceName"`
	ID              string `json:"id"`
	DescriptiveName string `json:"descriptiveName"`
}

type GoogleAdsCampaign struct {
	ResourceName string `json:"resourceName"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
}

type GoogleAdsMetrics struct {
	CostMicros  json.Number `json:"costMicros"`
	Clicks      json.Number `json:"clicks"`
	Impressions json.Number `json:"impressions"`
	Conversions json.Number `json:"conversions"`
}

// ══════════════════════════════════════════════════
// Dashboard Sonuç Yapıları
// ══════════════════════════════════════════════════

type GoogleAdsAccountInsights struct {
	Spend       float64 `json:"spend"`
	Clicks      int64   `json:"clicks"`
	Impressions int64   `json:"impressions"`
	Conversions float64 `json:"conversions"`
	CPC         float64 `json:"cpc"`
	CTR         float64 `json:"ctr"`
}

type GoogleAdsCampaignItem struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Spend       float64 `json:"spend"`
	Clicks      int64   `json:"clicks"`
	Impressions int64   `json:"impressions"`
	Conversions float64 `json:"conversions"`
}

// ══════════════════════════════════════════════════
// Token Refresh — Refresh Token ile Yeni Access Token Al
// ══════════════════════════════════════════════════

type GoogleTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
}

// RefreshGoogleAccessToken — Refresh token kullanarak yeni access token alır
func RefreshGoogleAccessToken(refreshToken string) (*GoogleTokenResponse, error) {
	data := url.Values{
		"client_id":     {config.Get("GOOGLE_CLIENT_ID")},
		"client_secret": {config.Get("GOOGLE_CLIENT_SECRET")},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return nil, fmt.Errorf("token refresh HTTP hatası: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[GOOGLE ADS] Token refresh hatası (status=%d): %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("token refresh hatası (status %d)", resp.StatusCode)
	}

	var tokenResp GoogleTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("token refresh JSON parse hatası: %w", err)
	}

	log.Printf("[GOOGLE ADS] ✅ Token yenilendi, expires_in=%d saniye", tokenResp.ExpiresIn)
	return &tokenResp, nil
}

// ══════════════════════════════════════════════════
// GAQL Sorgu Fonksiyonu
// ══════════════════════════════════════════════════

func gaqlRequest(accessToken, customerID, query string) (*GoogleAdsSearchResponse, error) {
	endpoint := fmt.Sprintf("https://googleads.googleapis.com/%s/customers/%s/googleAds:search", googleAdsAPIVersion, customerID)
	devToken := config.Get("GOOGLE_DEVELOPER_TOKEN")

	bodyJSON, _ := json.Marshal(map[string]string{"query": query})

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("request oluşturma hatası: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("developer-token", devToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google Ads API istek hatası: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[GOOGLE ADS] API Hata yanıtı (status=%d): %s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("Google Ads API hatası (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result GoogleAdsSearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %w", err)
	}

	return &result, nil
}

// ══════════════════════════════════════════════════
// 1. ListGoogleAdsAccounts — Erişilebilir Müşteri Hesaplarını Listele
// ══════════════════════════════════════════════════

type GoogleAdsAccessibleCustomer struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Manager bool   `json:"manager"`
}

// ListGoogleAdsAccounts — Bu access_token ile erişilebilen müşteri hesaplarını döner
func ListGoogleAdsAccounts(accessToken string) ([]GoogleAdsAccessibleCustomer, error) {
	devToken := config.Get("GOOGLE_DEVELOPER_TOKEN")

	// Adım 1: Erişilebilir müşteri kaynak adlarını al
	endpoint := fmt.Sprintf("https://googleads.googleapis.com/%s/customers:listAccessibleCustomers", googleAdsAPIVersion)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("developer-token", devToken)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listAccessibleCustomers HTTP hatası: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[GOOGLE ADS] listAccessibleCustomers response (status=%d): %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listAccessibleCustomers hatası (status %d): %s", resp.StatusCode, string(body))
	}

	var listResp struct {
		ResourceNames []string `json:"resourceNames"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %w", err)
	}

	log.Printf("[GOOGLE ADS] %d erişilebilir müşteri bulundu: %v", len(listResp.ResourceNames), listResp.ResourceNames)

	// Adım 2: Her müşteri hakkında detay çek (GAQL ile)
	var customers []GoogleAdsAccessibleCustomer
	for _, rn := range listResp.ResourceNames {
		// resourceName format: "customers/1234567890"
		var custID string
		fmt.Sscanf(rn, "customers/%s", &custID)
		if custID == "" {
			continue
		}

		// GAQL ile müşteri bilgisi çek
		query := "SELECT customer.id, customer.descriptive_name, customer.manager FROM customer LIMIT 1"
		gaqlResp, err := gaqlRequest(accessToken, custID, query)
		if err != nil {
			log.Printf("[GOOGLE ADS] Müşteri %s bilgisi çekilemedi: %v", custID, err)
			// Hata olsa bile ID ile ekle
			customers = append(customers, GoogleAdsAccessibleCustomer{ID: custID, Name: "Hesap " + custID})
			continue
		}

		if len(gaqlResp.Results) > 0 && gaqlResp.Results[0].Customer != nil {
			c := gaqlResp.Results[0].Customer
			customers = append(customers, GoogleAdsAccessibleCustomer{
				ID:   c.ID,
				Name: c.DescriptiveName,
			})
		} else {
			customers = append(customers, GoogleAdsAccessibleCustomer{ID: custID, Name: "Hesap " + custID})
		}
	}

	log.Printf("[GOOGLE ADS] ✅ %d müşteri hesabı listelendi", len(customers))
	return customers, nil
}

// ══════════════════════════════════════════════════
// 2. GetGoogleAdsDailyMetrics — Son 30 Günlük Hesap Metrikleri
//    KRİTİK: cost_micros ÷ 1.000.000 = gerçek TL
// ══════════════════════════════════════════════════

func GetGoogleAdsDailyMetrics(accessToken, customerID string) (*GoogleAdsAccountInsights, error) {
	query := `SELECT
		metrics.cost_micros,
		metrics.clicks,
		metrics.impressions,
		metrics.conversions
	FROM customer
	WHERE segments.date DURING LAST_30_DAYS`

	resp, err := gaqlRequest(accessToken, customerID, query)
	if err != nil {
		return nil, err
	}

	insights := &GoogleAdsAccountInsights{}

	if len(resp.Results) > 0 {
		m := resp.Results[0].Metrics

		// ★ KRİTİK: cost_micros → gerçek para birimi (÷ 1.000.000)
		costMicros, _ := m.CostMicros.Float64()
		insights.Spend = costMicros / 1_000_000

		clicks, _ := m.Clicks.Int64()
		insights.Clicks = clicks

		impressions, _ := m.Impressions.Int64()
		insights.Impressions = impressions

		conversions, _ := m.Conversions.Float64()
		insights.Conversions = conversions

		// CPC = Toplam Harcama / Toplam Tıklama
		if clicks > 0 {
			insights.CPC = insights.Spend / float64(clicks)
		}

		// CTR = (Tıklama / Gösterim) × 100
		if impressions > 0 {
			insights.CTR = (float64(clicks) / float64(impressions)) * 100
		}
	}

	log.Printf("[GOOGLE ADS] ✅ Metrics: spend=%.2f TL, clicks=%d, impressions=%d, conversions=%.0f",
		insights.Spend, insights.Clicks, insights.Impressions, insights.Conversions)

	return insights, nil
}

// ══════════════════════════════════════════════════
// Kampanya Listesi — GAQL ile
// ══════════════════════════════════════════════════

func FetchGoogleAdsCampaigns(accessToken, customerID string, limit int) ([]GoogleAdsCampaignItem, error) {
	query := fmt.Sprintf(`SELECT
		campaign.id,
		campaign.name,
		campaign.status,
		metrics.cost_micros,
		metrics.clicks,
		metrics.impressions,
		metrics.conversions
	FROM campaign
	WHERE segments.date DURING LAST_30_DAYS
		AND campaign.status != 'REMOVED'
	ORDER BY metrics.cost_micros DESC
	LIMIT %d`, limit)

	resp, err := gaqlRequest(accessToken, customerID, query)
	if err != nil {
		return nil, err
	}

	var campaigns []GoogleAdsCampaignItem
	for _, r := range resp.Results {
		if r.Campaign == nil {
			continue
		}

		costMicros, _ := r.Metrics.CostMicros.Float64()
		clicks, _ := r.Metrics.Clicks.Int64()
		impressions, _ := r.Metrics.Impressions.Int64()
		conversions, _ := r.Metrics.Conversions.Float64()

		campaigns = append(campaigns, GoogleAdsCampaignItem{
			ID:          r.Campaign.ID,
			Name:        r.Campaign.Name,
			Status:      r.Campaign.Status,
			Spend:       costMicros / 1_000_000, // ★ micros → TL
			Clicks:      clicks,
			Impressions: impressions,
			Conversions: conversions,
		})
	}

	log.Printf("[GOOGLE ADS] ✅ %d kampanya çekildi", len(campaigns))
	return campaigns, nil
}

// Eski isimleri korumak için alias'lar
func FetchGoogleAdsAccountInsights(accessToken, customerID string) (*GoogleAdsAccountInsights, error) {
	return GetGoogleAdsDailyMetrics(accessToken, customerID)
}

func FetchAccessibleCustomers(accessToken string) ([]GoogleAdsAccessibleCustomer, error) {
	return ListGoogleAdsAccounts(accessToken)
}
