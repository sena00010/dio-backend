package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ══════════════════════════════════════════════════
// LinkedIn Marketing API — Service Layer
// ══════════════════════════════════════════════════

const linkedinAPIBaseURL = "https://api.linkedin.com/rest"
const linkedinAPIVersion = "202601"

// ── Response Struct'ları ──

// LinkedInAccountInsights — Hesap seviyesindeki son 30 günlük reklam özeti
type LinkedInAccountInsights struct {
	Spend       float64 `json:"spend"`
	Clicks      int64   `json:"clicks"`
	Impressions int64   `json:"impressions"`
	Conversions float64 `json:"conversions"`
}

// LinkedInCampaign — Kampanya performansı
type LinkedInCampaign struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Spend       float64 `json:"spend"`
	Clicks      int64   `json:"clicks"`
	Impressions int64   `json:"impressions"`
	Status      string  `json:"status"`
}

// ── LinkedIn API Raw Response ──

type linkedinAnalyticsResponse struct {
	Elements []linkedinAnalyticsElement `json:"elements"`
	Paging   *json.RawMessage           `json:"paging,omitempty"`
}

type linkedinAnalyticsElement struct {
	PivotValueRaw          json.RawMessage `json:"pivotValue,omitempty"`
	PivotValuesRaw         []string        `json:"pivotValues,omitempty"`
	CostInLocalCurrency    string          `json:"costInLocalCurrency,omitempty"`
	Clicks                 json.Number     `json:"clicks"`
	Impressions            json.Number     `json:"impressions"`
	ExternalWebsiteConversions json.Number `json:"externalWebsiteConversions"`
	DateRange              *json.RawMessage `json:"dateRange,omitempty"`
}

func extractAccountID(urn string) string {
	parts := strings.Split(urn, ":")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// ══════════════════════════════════════════════════
// Public API Fonksiyonları
// ══════════════════════════════════════════════════

// FetchLinkedInAccountInsights — Account seviyesinde harcama (days gün)
func FetchLinkedInAccountInsights(accessToken, accountURN string, sinceStr, untilStr string) (*LinkedInAccountInsights, error) {
	accountID := extractAccountID(accountURN)
	if accountID == "" {
		return nil, fmt.Errorf("geçersiz account URN: %s", accountURN)
	}

	escapedURN := "urn%3Ali%3AsponsoredAccount%3A" + accountID

	var startDate, endDate time.Time
	var err error
	if sinceStr != "" && untilStr != "" {
		startDate, err = time.Parse("2006-01-02", sinceStr)
		endDate, _ = time.Parse("2006-01-02", untilStr)
		if err != nil {
			startDate = time.Now().AddDate(0, 0, -30)
			endDate = time.Now()
		}
	} else {
		endDate = time.Now()
		startDate = endDate.AddDate(0, 0, -30)
	}

	fullURL := fmt.Sprintf(
		"%s/adAnalytics?q=analytics&pivot=ACCOUNT&timeGranularity=ALL"+
			"&dateRange=(start:(year:%d,month:%d,day:%d),end:(year:%d,month:%d,day:%d))"+
			"&accounts=List(%s)"+
			"&fields=costInLocalCurrency,clicks,impressions,externalWebsiteConversions",
		linkedinAPIBaseURL,
		startDate.Year(), int(startDate.Month()), startDate.Day(),
		endDate.Year(), int(endDate.Month()), endDate.Day(),
		escapedURN,
	)

	log.Printf("[LINKEDIN] adAnalytics URL: %s", fullURL)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("LinkedIn API request oluşturma hatası: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", linkedinAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LinkedIn API istek hatası: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("LinkedIn API yanıt okunamadı: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LinkedIn API hatası (status %d): %s", resp.StatusCode, string(body))
	}

	var result linkedinAnalyticsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("LinkedIn JSON parse hatası: %w", err)
	}

	insights := &LinkedInAccountInsights{}

	for _, elem := range result.Elements {
		clicks, _ := elem.Clicks.Int64()
		impressions, _ := elem.Impressions.Int64()
		conversions, _ := elem.ExternalWebsiteConversions.Float64()

		insights.Clicks += clicks
		insights.Impressions += impressions
		insights.Conversions += conversions

		if elem.CostInLocalCurrency != "" {
			var cost float64
			fmt.Sscanf(elem.CostInLocalCurrency, "%f", &cost)
			insights.Spend += cost
		}
	}

	log.Printf("[LINKEDIN] ✅ Insights: spend=%.2f, clicks=%d, impressions=%d, conversions=%.0f",
		insights.Spend, insights.Clicks, insights.Impressions, insights.Conversions)

	return insights, nil
}

// FetchLinkedInCampaigns — Kampanyaları ve harcamalarını getirir
func FetchLinkedInCampaigns(accessToken, accountURN string, sinceStr, untilStr string) ([]LinkedInCampaign, error) {
	accountID := extractAccountID(accountURN)
	if accountID == "" {
		return nil, fmt.Errorf("geçersiz account URN, ID çıkarılamadı: %s", accountURN)
	}

	var startDate, endDate time.Time
	var err error
	if sinceStr != "" && untilStr != "" {
		startDate, err = time.Parse("2006-01-02", sinceStr)
		endDate, _ = time.Parse("2006-01-02", untilStr)
		if err != nil {
			startDate = time.Now().AddDate(0, 0, -30)
			endDate = time.Now()
		}
	} else {
		endDate = time.Now()
		startDate = endDate.AddDate(0, 0, -30)
	}
	
	reqURL := &url.URL{
		Scheme:   "https",
		Host:     "api.linkedin.com",
		Path:     fmt.Sprintf("/rest/adAccounts/%s/adCampaigns", accountID),
		RawPath:  fmt.Sprintf("/rest/adAccounts/%s/adCampaigns", accountID),
		RawQuery: "q=search&count=100",
	}

	req := &http.Request{
		Method: "GET",
		URL:    reqURL,
		Header: make(http.Header),
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", linkedinAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[LINKEDIN CAMPAIGNS] İstek hatası: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[LINKEDIN CAMPAIGNS] adCampaigns yanıtı (status=%d): %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LinkedIn adCampaigns API hatası (status %d): %s", resp.StatusCode, string(body))
	}

	var campaignsResult struct {
		Elements []struct {
			ID      int64  `json:"id"`
			Name    string `json:"name"`
			Status  string `json:"status"`
			Account string `json:"account"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &campaignsResult); err != nil {
		return nil, err
	}

	campaignMap := make(map[string]LinkedInCampaign)
	for _, c := range campaignsResult.Elements {
		campaignURN := fmt.Sprintf("urn:li:sponsoredCampaign:%d", c.ID)
		campaignMap[campaignURN] = LinkedInCampaign{
			ID:     fmt.Sprintf("%d", c.ID),
			Name:   c.Name,
			Status: c.Status,
		}
	}

	// 2. AdAnalytics endpoint'ten kampanya istatistiklerini çek
	escapedURN := url.QueryEscape(accountURN)

	analyticsURL := fmt.Sprintf(
		"%s/adAnalytics?q=analytics&pivot=CAMPAIGN&timeGranularity=ALL"+
			"&dateRange=(start:(year:%d,month:%d,day:%d),end:(year:%d,month:%d,day:%d))"+
			"&accounts=List(%s)"+
			"&fields=pivotValues,costInLocalCurrency,clicks,impressions",
		linkedinAPIBaseURL,
		startDate.Year(), int(startDate.Month()), startDate.Day(),
		endDate.Year(), int(endDate.Month()), endDate.Day(),
		escapedURN,
	)

	req2, _ := http.NewRequest("GET", analyticsURL, nil)
	req2.Header.Set("Authorization", "Bearer "+accessToken)
	req2.Header.Set("LinkedIn-Version", linkedinAPIVersion)
	req2.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp2, err := client.Do(req2)
	if err != nil {
		log.Printf("[LINKEDIN CAMPAIGNS] adAnalytics istek hatası: %v", err)
		return nil, err
	}
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	log.Printf("[LINKEDIN CAMPAIGNS] adAnalytics yanıtı (status=%d): %s", resp2.StatusCode, string(body2))

	if resp2.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LinkedIn adAnalytics (kampanyalar) API hatası (status %d): %s", resp2.StatusCode, string(body2))
	}

	var result2 linkedinAnalyticsResponse
	if err := json.Unmarshal(body2, &result2); err != nil {
		return nil, err
	}

	for _, elem := range result2.Elements {
		var pivotURN string
		if len(elem.PivotValuesRaw) > 0 {
			pivotURN = elem.PivotValuesRaw[0]
		} else if len(elem.PivotValueRaw) > 0 {
			// fallback for older API versions that might still use pivotValue
			if err := json.Unmarshal(elem.PivotValueRaw, &pivotURN); err != nil {
				var pivotObj map[string]string
				if err2 := json.Unmarshal(elem.PivotValueRaw, &pivotObj); err2 == nil {
					for _, v := range pivotObj {
						pivotURN = v
						break
					}
				}
			}
		}

		if pivotURN != "" {
			if camp, exists := campaignMap[pivotURN]; exists {
				var cost float64
				fmt.Sscanf(elem.CostInLocalCurrency, "%f", &cost)
				camp.Spend += cost

				clicks, _ := elem.Clicks.Int64()
				camp.Clicks += clicks

				impressions, _ := elem.Impressions.Int64()
				camp.Impressions += impressions

				campaignMap[pivotURN] = camp
			}
		}
	}

	var finalList []LinkedInCampaign
	for _, camp := range campaignMap {
		finalList = append(finalList, camp)
	}

	return finalList, nil
}

// FetchLinkedInAdAccounts — Kullanıcının tüm LinkedIn reklam hesaplarını listeler
func FetchLinkedInAdAccounts(accessToken string) ([]LinkedInAdAccount, error) {
	fullURL := fmt.Sprintf("%s/adAccounts?q=search&count=50", linkedinAPIBaseURL)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", linkedinAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[LINKEDIN] adAccounts yanıtı (status=%d)", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LinkedIn adAccounts hatası (status %d)", resp.StatusCode)
	}

	var result struct {
		Elements []struct {
			ID              int    `json:"id"`
			Name            string `json:"name"`
			Status          string `json:"status"`
			Currency        string `json:"currency"`
			ServingStatuses []string `json:"servingStatuses"`
			Reference       string `json:"reference"`
		} `json:"elements"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	accounts := []LinkedInAdAccount{}
	for _, elem := range result.Elements {
		urn := fmt.Sprintf("urn:li:sponsoredAccount:%d", elem.ID)
		serving := ""
		if len(elem.ServingStatuses) > 0 {
			serving = elem.ServingStatuses[0]
		}
		accounts = append(accounts, LinkedInAdAccount{
			ID:        elem.ID,
			URN:       urn,
			Name:      elem.Name,
			Status:    elem.Status,
			Currency:  elem.Currency,
			Serving:   serving,
			Reference: elem.Reference,
		})
	}

	log.Printf("[LINKEDIN] ✅ %d reklam hesabı bulundu", len(accounts))
	return accounts, nil
}

// LinkedInAdAccount — LinkedIn reklam hesabı
type LinkedInAdAccount struct {
	ID        int    `json:"id"`
	URN       string `json:"urn"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Currency  string `json:"currency"`
	Serving   string `json:"serving"`
	Reference string `json:"reference"`
}

