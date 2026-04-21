package services

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ══════════════════════════════════════════════════
// TikTok Marketing API — Service Layer
// ══════════════════════════════════════════════════

const tiktokAPIBaseURL = "https://business-api.tiktok.com/open_api/v1.3"

// ── Response Struct'ları ──

// TikTokAccountInsights — Hesap seviyesindeki son 30 günlük reklam özeti
type TikTokAccountInsights struct {
	Spend          float64 `json:"spend"`
	Clicks         int64   `json:"clicks"`
	Conversions    float64 `json:"conversions"`
	CPA            float64 `json:"cpa"`
	ConversionRate float64 `json:"conversion_rate"`
}

// ── TikTok API Raw Response ──

type tiktokReportResponse struct {
	Code    int                 `json:"code"`
	Message string              `json:"message"`
	Data    tiktokReportData    `json:"data"`
}

type tiktokReportData struct {
	List     []tiktokReportRow `json:"list"`
	PageInfo *json.RawMessage  `json:"page_info,omitempty"`
}

type tiktokReportRow struct {
	Dimensions json.RawMessage     `json:"dimensions"`
	Metrics    tiktokReportMetrics `json:"metrics"`
}

type tiktokReportMetrics struct {
	Spend          string `json:"spend"`
	Clicks         string `json:"clicks"`
	Conversion     string `json:"conversion"`
	CPA            string `json:"cpa"`
	ConversionRate string `json:"conversion_rate"`
}

// ══════════════════════════════════════════════════
// Public API Fonksiyonları
// ══════════════════════════════════════════════════

// FetchTikTokAccountInsights — Son 30 günlük hesap seviyesi reklam performans verileri
func FetchTikTokAccountInsights(accessToken, advertiserID string) (*TikTokAccountInsights, error) {
	now := time.Now()
	startDate := now.AddDate(0, 0, -30).Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	endpoint := fmt.Sprintf("%s/report/integrated/get/", tiktokAPIBaseURL)

	params := url.Values{}
	params.Set("advertiser_id", advertiserID)
	params.Set("report_type", "BASIC")
	params.Set("data_level", "AUCTION_ADVERTISER")
	params.Set("dimensions", `["advertiser_id"]`)
	params.Set("metrics", `["spend","clicks","conversion","conversion_rate"]`)
	params.Set("start_date", startDate)
	params.Set("end_date", endDate)
	params.Set("page_size", "10")

	fullURL := endpoint + "?" + params.Encode()

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("TikTok API request oluşturma hatası: %w", err)
	}
	req.Header.Set("Access-Token", accessToken)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TikTok API istek hatası: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("TikTok API yanıt okunamadı: %w", err)
	}

	log.Printf("[TIKTOK] API yanıtı (status=%d): %s", resp.StatusCode, string(body))

	var result tiktokReportResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("TikTok JSON parse hatası: %w", err)
	}

	// TikTok API hata kodları: 0 = success
	if result.Code != 0 {
		return nil, fmt.Errorf("TikTok API hatası [%d]: %s", result.Code, result.Message)
	}

	insights := &TikTokAccountInsights{}

	if len(result.Data.List) > 0 {
		m := result.Data.List[0].Metrics
		insights.Spend, _ = strconv.ParseFloat(m.Spend, 64)
		clicks, _ := strconv.ParseFloat(m.Clicks, 64)
		insights.Clicks = int64(clicks)
		insights.Conversions, _ = strconv.ParseFloat(m.Conversion, 64)
		insights.ConversionRate, _ = strconv.ParseFloat(m.ConversionRate, 64)
		
		// CPA'i manuel hesapla: Harcama / Dönüşüm (bölme hatası olmaması için kontrol edelim)
		if insights.Conversions > 0 {
			insights.CPA = insights.Spend / insights.Conversions
		} else {
			insights.CPA = 0
		}
	}

	log.Printf("[TIKTOK] ✅ Insights: spend=%.2f, clicks=%d, conversions=%.0f, cpa=%.2f",
		insights.Spend, insights.Clicks, insights.Conversions, insights.CPA)

	return insights, nil
}
