package services

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
)

const metaGraphBaseURL = "https://graph.facebook.com/v19.0"

// ══════════════════════════════════════════════════
// Response Struct'ları
// ══════════════════════════════════════════════════

// MetaAccountInsights — Hesap seviyesindeki son 30 günlük reklam özeti
type MetaAccountInsights struct {
	Spend       float64 `json:"spend"`        // Toplam harcama
	Revenue     float64 `json:"revenue"`      // Satış geliri (action_values -> purchase)
	ROAS        float64 `json:"roas"`         // Purchase ROAS
	Conversions int     `json:"conversions"`  // Satış adedi (actions -> purchase)
}

// MetaCampaignItem — Tek bir kampanyanın bilgisi
type MetaCampaignItem struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`       // ACTIVE, PAUSED, ARCHIVED
	DailyBudget float64 `json:"daily_budget"` // Kuruş → TL dönüşümü yapılmış
	Spend       float64 `json:"spend"`        // Kampanya harcaması
	ROAS        float64 `json:"roas"`         // Kampanya ROAS
}

// ══════════════════════════════════════════════════
// Graph API Raw Response Struct'ları
// ══════════════════════════════════════════════════

type graphInsightsResponse struct {
	Data   []graphInsightsData `json:"data"`
	Paging *json.RawMessage   `json:"paging,omitempty"`
	Error  *graphError        `json:"error,omitempty"`
}

type graphInsightsData struct {
	Spend        string             `json:"spend"`
	PurchaseROAS []graphActionValue `json:"purchase_roas,omitempty"`
	Actions      []graphAction      `json:"actions,omitempty"`
	ActionValues []graphActionValue `json:"action_values,omitempty"`
	DateStart    string             `json:"date_start"`
	DateStop     string             `json:"date_stop"`
}

type graphAction struct {
	ActionType string `json:"action_type"`
	Value      string `json:"value"`
}

type graphActionValue struct {
	ActionType string `json:"action_type,omitempty"`
	Value      string `json:"value"`
}

type graphCampaignsResponse struct {
	Data   []graphCampaignData `json:"data"`
	Paging *json.RawMessage    `json:"paging,omitempty"`
	Error  *graphError         `json:"error,omitempty"`
}

type graphCampaignData struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	DailyBudget string `json:"daily_budget"` // Kuruş cinsinden string
	Insights    *struct {
		Data []graphInsightsData `json:"data"`
	} `json:"insights,omitempty"`
}

type graphError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    int    `json:"code"`
}

// ══════════════════════════════════════════════════
// Public API Fonksiyonları
// ══════════════════════════════════════════════════

// FetchMetaAccountInsights — Hesabın istenen tarih aralığındaki verisi
func FetchMetaAccountInsights(accessToken, adAccountID, since, until string) (*MetaAccountInsights, error) {
	if since == "" || until == "" {
		now := time.Now()
		since = now.AddDate(0, 0, -30).Format("2006-01-02")
		until = now.Format("2006-01-02")
	}

	// ad_account_id "act_" prefix kontrolü
	if !strings.HasPrefix(adAccountID, "act_") {
		adAccountID = "act_" + adAccountID
	}

	endpoint := fmt.Sprintf("%s/%s/insights", metaGraphBaseURL, adAccountID)

	params := url.Values{}
	params.Set("access_token", accessToken)
	params.Set("fields", "spend,purchase_roas,actions,action_values")
	params.Set("time_range", fmt.Sprintf(`{"since":"%s","until":"%s"}`, since, until))
	params.Set("level", "account")

	fullURL := endpoint + "?" + params.Encode()
	log.Printf("[META] Account Insights isteği: %s/****/insights?fields=spend,purchase_roas,actions,action_values", adAccountID)

	resp, err := http.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("Meta Graph API isteği başarısız: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("yanıt okunamadı: %w", err)
	}

	var result graphInsightsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %w", err)
	}

	// DEBUG: Meta API ham yanıtını logla
	log.Printf("[META DEBUG] Ham API yanıtı: %s", string(body))

	if result.Error != nil {
		return nil, fmt.Errorf("Meta API hatası [%d]: %s", result.Error.Code, result.Error.Message)
	}

	insights := &MetaAccountInsights{}

	if len(result.Data) > 0 {
		data := result.Data[0]

		// Spend
		insights.Spend, _ = strconv.ParseFloat(data.Spend, 64)

		// Purchase ROAS
		for _, r := range data.PurchaseROAS {
			insights.ROAS, _ = strconv.ParseFloat(r.Value, 64)
			break // İlk değeri al
		}

		// Conversions (actions -> purchase)
		for _, a := range data.Actions {
			if a.ActionType == "purchase" || a.ActionType == "offsite_conversion.fb_pixel_purchase" {
				insights.Conversions, _ = strconv.Atoi(a.Value)
				break
			}
		}

		// Revenue (action_values -> purchase)
		for _, av := range data.ActionValues {
			if av.ActionType == "purchase" || av.ActionType == "offsite_conversion.fb_pixel_purchase" {
				insights.Revenue, _ = strconv.ParseFloat(av.Value, 64)
				break
			}
		}

		// ROAS fallback: eğer purchase_roas boşsa ve revenue varsa hesapla
		if insights.ROAS == 0 && insights.Spend > 0 && insights.Revenue > 0 {
			insights.ROAS = insights.Revenue / insights.Spend
		}
	}

	log.Printf("[META] Account Insights sonuç: spend=%.2f, revenue=%.2f, roas=%.2f, conversions=%d",
		insights.Spend, insights.Revenue, insights.ROAS, insights.Conversions)

	return insights, nil
}

// FetchMetaCampaigns — Kampanyaları limitle ve tarih aralığıyla getir
func FetchMetaCampaigns(accessToken, adAccountID string, limit int, since, until string) ([]MetaCampaignItem, error) {
	if limit <= 0 {
		limit = 10
	}

	if since == "" || until == "" {
		now := time.Now()
		since = now.AddDate(0, 0, -30).Format("2006-01-02")
		until = now.Format("2006-01-02")
	}

	if !strings.HasPrefix(adAccountID, "act_") {
		adAccountID = "act_" + adAccountID
	}

	endpoint := fmt.Sprintf("%s/%s/campaigns", metaGraphBaseURL, adAccountID)

	// insights alt alanını da çekiyoruz (nested request)
	insightsParam := fmt.Sprintf(
		`insights.fields(spend,purchase_roas).time_range({"since":"%s","until":"%s"})`,
		since, until,
	)

	params := url.Values{}
	params.Set("access_token", accessToken)
	params.Set("fields", fmt.Sprintf("name,status,daily_budget,%s", insightsParam))
	params.Set("limit", strconv.Itoa(limit))
	// Sadece aktif/duraklatılmış kampanyaları getir (arşivlenmiş hariç)
	params.Set("filtering", `[{"field":"effective_status","operator":"IN","value":["ACTIVE","PAUSED"]}]`)

	fullURL := endpoint + "?" + params.Encode()
	log.Printf("[META] Campaigns isteği: %s/****/campaigns?limit=%d", adAccountID, limit)

	resp, err := http.Get(fullURL)
	if err != nil {
		return nil, fmt.Errorf("Meta Graph API campaigns isteği başarısız: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("yanıt okunamadı: %w", err)
	}

	var result graphCampaignsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON parse hatası: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("Meta API hatası [%d]: %s", result.Error.Code, result.Error.Message)
	}

	campaigns := make([]MetaCampaignItem, 0, len(result.Data))
	for _, c := range result.Data {
		item := MetaCampaignItem{
			ID:     c.ID,
			Name:   c.Name,
			Status: c.Status,
		}

		// Daily budget: kuruş → TL (Meta hep kuruş gönderir)
		if c.DailyBudget != "" {
			budgetCents, _ := strconv.ParseFloat(c.DailyBudget, 64)
			item.DailyBudget = budgetCents / 100.0
		}

		// Kampanya insights
		if c.Insights != nil && len(c.Insights.Data) > 0 {
			iData := c.Insights.Data[0]
			item.Spend, _ = strconv.ParseFloat(iData.Spend, 64)

			for _, r := range iData.PurchaseROAS {
				item.ROAS, _ = strconv.ParseFloat(r.Value, 64)
				break
			}
		}

		campaigns = append(campaigns, item)
	}

	log.Printf("[META] Campaigns sonuç: %d kampanya bulundu", len(campaigns))
	return campaigns, nil
}
