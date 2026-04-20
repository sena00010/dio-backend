package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/services"
)

// ══════════════════════════════════════════════════
// DashboardStats — Birleşik platformlardan gelen veriler
// ══════════════════════════════════════════════════

// DashboardStats — Ortak çıkış struct'ı. Tüm platformlar bu formata map edilir.
type DashboardStats struct {
	Platform    string  `json:"platform"`
	Spend       float64 `json:"spend"`
	Clicks      int     `json:"clicks"`
	Conversions float64 `json:"conversions"`
	ROAS        float64 `json:"roas"`
}

// ══════════════════════════════════════════════════
// GET /api/dashboard/stats?company_id=X&platform=all|meta|google|tiktok|linkedin
// Birden fazla platformu paralel çeker, hata olursa 0 döner (graceful fallback)
// ══════════════════════════════════════════════════

func DashboardStatsHandler(c *gin.Context) {
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

	// Platform filtresi: "all" (default), "meta", "google", "tiktok", "linkedin"
	platformFilter := strings.ToLower(c.DefaultQuery("platform", "all"))

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results []DashboardStats
	)

	// Hangi platformları çekelim
	shouldFetch := func(p string) bool {
		return platformFilter == "all" || platformFilter == p
	}

	// ── META ──
	if shouldFetch("meta") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stat := DashboardStats{Platform: "meta"}

			metaAccount, err := getMetaAccount(userID, companyID)
			if err != nil {
				log.Printf("[DASHBOARD] Meta hesap bulunamadı (user=%v): %v", userID, err)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			now := time.Now()
			since := now.AddDate(0, 0, -30).Format("2006-01-02")
			until := now.Format("2006-01-02")
			
			insights, err := services.FetchMetaAccountInsights(metaAccount.AccessToken, *metaAccount.AdAccountID, since, until)
			if err != nil {
				log.Printf("[DASHBOARD] Meta insights hatası (user=%v): %v", userID, err)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			stat.Spend = insights.Spend
			stat.Conversions = float64(insights.Conversions)
			stat.ROAS = insights.ROAS
			// Meta insights'ta clicks yok, revenue üzerinden ROAS hesabı yapılır
			// Eğer clicks alanı varsa eklenebilir, şimdilik 0

			mu.Lock()
			results = append(results, stat)
			mu.Unlock()
		}()
	}

	// ── GOOGLE ADS ──
	if shouldFetch("google") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stat := DashboardStats{Platform: "google"}

			gadsAccount, err := getGoogleAdsAccount(userID, companyID)
			if err != nil {
				log.Printf("[DASHBOARD] Google Ads hesap bulunamadı (user=%v): %v", userID, err)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			// Customer ID
			customerID := ""
			if gadsAccount.GoogleAdsCustomerID != nil {
				customerID = *gadsAccount.GoogleAdsCustomerID
			}
			if customerID == "" {
				log.Printf("[DASHBOARD] Google Ads customer_id boş (user=%v)", userID)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			// Token yenile
			accessToken, err := ensureFreshGadsToken(gadsAccount)
			if err != nil {
				log.Printf("[DASHBOARD] Google Ads token yenileme hatası (user=%v): %v", userID, err)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			insights, err := services.GetGoogleAdsDailyMetrics(accessToken, customerID)
			if err != nil {
				log.Printf("[DASHBOARD] Google Ads insights hatası (user=%v): %v", userID, err)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			stat.Spend = insights.Spend
			stat.Clicks = int(insights.Clicks)
			stat.Conversions = insights.Conversions
			// Google Ads'te ROAS -> conversions_value / cost (opsiyonel, yoksa 0)

			mu.Lock()
			results = append(results, stat)
			mu.Unlock()
		}()
	}

	// ── TIKTOK ──
	if shouldFetch("tiktok") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stat := DashboardStats{Platform: "tiktok"}

			ttAccount, err := getTikTokAccount(userID, companyID)
			if err != nil {
				log.Printf("[DASHBOARD] TikTok hesap bulunamadı (user=%v): %v", userID, err)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			insights, err := services.FetchTikTokAccountInsights(ttAccount.AccessToken, *ttAccount.AdvertiserID)
			if err != nil {
				log.Printf("[DASHBOARD] TikTok insights hatası (user=%v): %v", userID, err)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			stat.Spend = insights.Spend
			stat.Clicks = int(insights.Clicks)
			stat.Conversions = insights.Conversions

			mu.Lock()
			results = append(results, stat)
			mu.Unlock()
		}()
	}

	// ── LINKEDIN ──
	if shouldFetch("linkedin") {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stat := DashboardStats{Platform: "linkedin"}

			liAccount, err := getLinkedInAccount(userID, companyID)
			if err != nil {
				log.Printf("[DASHBOARD] LinkedIn hesap bulunamadı (user=%v): %v", userID, err)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			// AccountURN yoksa (OAuth başarılı ama ad account bulunamadı)
			if liAccount.AccountURN == nil || *liAccount.AccountURN == "" {
				log.Printf("[DASHBOARD] LinkedIn account_urn boş (user=%v), sıfır veriler", userID)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			now := time.Now()
			since := now.AddDate(0, 0, -30).Format("2006-01-02")
			until := now.Format("2006-01-02")
			insights, err := services.FetchLinkedInAccountInsights(liAccount.AccessToken, *liAccount.AccountURN, since, until)
			if err != nil {
				log.Printf("[DASHBOARD] LinkedIn insights hatası (user=%v): %v", userID, err)
				mu.Lock()
				results = append(results, stat)
				mu.Unlock()
				return
			}

			stat.Spend = insights.Spend
			stat.Clicks = int(insights.Clicks)
			stat.Conversions = insights.Conversions

			mu.Lock()
			results = append(results, stat)
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Toplam hesapla
	var totalSpend float64
	var totalClicks int
	var totalConversions float64
	var totalROAS float64
	roasCount := 0

	for _, s := range results {
		totalSpend += s.Spend
		totalClicks += s.Clicks
		totalConversions += s.Conversions
		if s.ROAS > 0 {
			totalROAS += s.ROAS
			roasCount++
		}
	}

	avgROAS := 0.0
	if roasCount > 0 {
		avgROAS = totalROAS / float64(roasCount)
	}

	c.JSON(http.StatusOK, gin.H{
		"platforms": results,
		"totals": DashboardStats{
			Platform:    "all",
			Spend:       totalSpend,
			Clicks:      totalClicks,
			Conversions: totalConversions,
			ROAS:        avgROAS,
		},
	})
}
