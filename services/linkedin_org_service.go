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
// LinkedIn Organization API — Social Data
// ══════════════════════════════════════════════════

// LinkedInOrgSocialData — LinkedIn şirket sayfası sosyal medya verileri
type LinkedInOrgSocialData struct {
	Followers      int64   `json:"followers"`
	TotalViews     int64   `json:"total_views"`
	DesktopViews   int64   `json:"desktop_views"`
	MobileViews    int64   `json:"mobile_views"`
	OverviewViews  int64   `json:"overview_views"`
	AboutViews     int64   `json:"about_views"`
	Engagement     float64 `json:"engagement"`
	Posts          int64   `json:"posts"`
	OrgName        string  `json:"org_name"`
	OrgURN         string  `json:"org_urn"`
}

// FetchLinkedInOrganizations — Kullanıcının admin olduğu LinkedIn organizasyonları listele
func FetchLinkedInOrganizations(accessToken string) (orgURN, orgName string) {
	// LinkedIn Organization Access Control: admin olduğu organizasyonlar
	// NOT: projection parametresi REST API'de yasaklı — sadece q=roleAssignee kullanılmalı
	apiURL := "https://api.linkedin.com/rest/organizationAcls?q=roleAssignee"

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Printf("[LINKEDIN ORG] Request oluşturulamadı: %v", err)
		return "", ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", linkedinAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[LINKEDIN ORG] API hatası: %v", err)
		return "", ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[LINKEDIN ORG] OrganizationAcls yanıtı (status=%d): %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		log.Printf("[LINKEDIN ORG] ⚠️ OrganizationAcls başarısız (status %d)", resp.StatusCode)
		return "", ""
	}

	// Yanıtı parse et — projection olmadan sadece organization URN gelir
	var result struct {
		Elements []struct {
			Organization string `json:"organization"`
			Role         string `json:"role"`
		} `json:"elements"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[LINKEDIN ORG] JSON parse hatası: %v", err)
		return "", ""
	}

	if len(result.Elements) > 0 {
		orgURN = result.Elements[0].Organization
		log.Printf("[LINKEDIN ORG] ✅ Organization URN bulundu: %s", orgURN)

		// Organizasyon adını ayrı API çağrısıyla al
		orgName = fetchLinkedInOrgName(accessToken, orgURN)
		return orgURN, orgName
	}

	log.Printf("[LINKEDIN ORG] ⚠️ Admin olduğu organizasyon bulunamadı")
	return "", ""
}

// fetchLinkedInOrgName — Organization URN'den şirket adını çeker
func fetchLinkedInOrgName(accessToken, orgURN string) string {
	// orgURN formatı: "urn:li:organization:12345"
	apiURL := fmt.Sprintf("https://api.linkedin.com/rest/organizations/%s", extractOrgID(orgURN))

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", linkedinAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[LINKEDIN ORG] Organization details yanıtı (status=%d): %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var org struct {
		LocalizedName string `json:"localizedName"`
	}
	if err := json.Unmarshal(body, &org); err != nil {
		return ""
	}

	log.Printf("[LINKEDIN ORG] ✅ Organization name: %s", org.LocalizedName)
	return org.LocalizedName
}

// extractOrgID — "urn:li:organization:12345" -> "12345"
func extractOrgID(urn string) string {
	parts := splitURN(urn)
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return urn
}

// splitURN — URN'i ":" ile parçala
func splitURN(urn string) []string {
	result := []string{}
	current := ""
	for _, ch := range urn {
		if ch == ':' {
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

// FetchLinkedInOrgFollowers — Organizasyonun takipçi sayısını çek
func FetchLinkedInOrgFollowers(accessToken, orgURN string) int64 {
	// URN encode: urn:li:organization:123 → urn%3Ali%3Aorganization%3A123
	encodedURN := strings.ReplaceAll(orgURN, ":", "%3A")

	// url.URL struct ile doğrudan inşa — http.NewRequest URL parse'ı bypass
	reqURL := &url.URL{
		Scheme:   "https",
		Host:     "api.linkedin.com",
		Path:     "/rest/networkSizes/" + orgURN,
		RawPath:  "/rest/networkSizes/" + encodedURN,
		RawQuery: "edgeType=COMPANY_FOLLOWED_BY_MEMBER",
	}
	log.Printf("[LINKEDIN ORG] Followers URL: %s", reqURL.String())

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
		log.Printf("[LINKEDIN ORG] Followers API hatası: %v", err)
		return 0
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[LINKEDIN ORG] Followers yanıtı (status=%d): %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return 0
	}

	var result struct {
		FirstDegreeSize int64 `json:"firstDegreeSize"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0
	}

	log.Printf("[LINKEDIN ORG] ✅ Followers: %d", result.FirstDegreeSize)
	return result.FirstDegreeSize
}

// LinkedInPageStats — organizationPageStatistics parse sonuçları
type LinkedInPageStats struct {
	TotalViews    int64
	DesktopViews  int64
	MobileViews   int64
	OverviewViews int64
	AboutViews    int64
}

// FetchLinkedInOrgStats — Organizasyon sayfa görüntülenme istatistikleri
func FetchLinkedInOrgStats(accessToken, orgURN string) *LinkedInPageStats {
	encodedURN := url.QueryEscape(orgURN)
	apiURL := fmt.Sprintf(
		"https://api.linkedin.com/rest/organizationPageStatistics?q=organization&organization=%s",
		encodedURN,
	)
	log.Printf("[LINKEDIN ORG] Stats URL: %s", apiURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Printf("[LINKEDIN ORG] Stats request hatası: %v", err)
		return &LinkedInPageStats{}
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("LinkedIn-Version", linkedinAPIVersion)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[LINKEDIN ORG] Stats API hatası: %v", err)
		return &LinkedInPageStats{}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[LINKEDIN ORG] Stats yanıtı (status=%d): ilk 200 karakter: %s", resp.StatusCode, truncateStr(string(body), 200))

	if resp.StatusCode != http.StatusOK {
		return &LinkedInPageStats{}
	}

	// totalPageStatistics içinden views parse et
	var result struct {
		Elements []struct {
			TotalPageStatistics struct {
				Views struct {
					AllPageViews        struct{ PageViews int64 `json:"pageViews"` } `json:"allPageViews"`
					AllDesktopPageViews struct{ PageViews int64 `json:"pageViews"` } `json:"allDesktopPageViews"`
					AllMobilePageViews  struct{ PageViews int64 `json:"pageViews"` } `json:"allMobilePageViews"`
					OverviewPageViews   struct{ PageViews int64 `json:"pageViews"` } `json:"overviewPageViews"`
					AboutPageViews      struct{ PageViews int64 `json:"pageViews"` } `json:"aboutPageViews"`
				} `json:"views"`
			} `json:"totalPageStatistics"`
		} `json:"elements"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[LINKEDIN ORG] Stats JSON parse hatası: %v", err)
		return &LinkedInPageStats{}
	}

	if len(result.Elements) == 0 {
		return &LinkedInPageStats{}
	}

	views := result.Elements[0].TotalPageStatistics.Views
	stats := &LinkedInPageStats{
		TotalViews:    views.AllPageViews.PageViews,
		DesktopViews:  views.AllDesktopPageViews.PageViews,
		MobileViews:   views.AllMobilePageViews.PageViews,
		OverviewViews: views.OverviewPageViews.PageViews,
		AboutViews:    views.AboutPageViews.PageViews,
	}

	log.Printf("[LINKEDIN ORG] ✅ Stats: total=%d, desktop=%d, mobile=%d",
		stats.TotalViews, stats.DesktopViews, stats.MobileViews)
	return stats
}

// truncateStr — Log için string kısalt
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FetchLinkedInOrgSocialData — Tüm sosyal verileri birleştir
func FetchLinkedInOrgSocialData(accessToken string, providedOrgURN string) (*LinkedInOrgSocialData, error) {
	orgURN := providedOrgURN
	orgName := ""

	// Eğer spesifik bir orgURN verilmemişse (örneğin eski akışta), varsayılan olarak ilkini al
	if orgURN == "" {
		fetchedURN, fetchedName := FetchLinkedInOrganizations(accessToken)
		orgURN = fetchedURN
		orgName = fetchedName
	}

	if orgURN == "" {
		return nil, fmt.Errorf("LinkedIn organizasyon bulunamadı")
	}

	// 2. Takipçi sayısını çek
	followers := FetchLinkedInOrgFollowers(accessToken, orgURN)

	// 3. Sayfa görüntülenme istatistikleri
	pageStats := FetchLinkedInOrgStats(accessToken, orgURN)

	// Engagement = totalViews / followers * 100 (sayfa görüntülenme bazlı)
	engagementRate := float64(0)
	if followers > 0 {
		engagementRate = (float64(pageStats.TotalViews) / float64(followers)) * 100
	}

	return &LinkedInOrgSocialData{
		Followers:     followers,
		TotalViews:    pageStats.TotalViews,
		DesktopViews:  pageStats.DesktopViews,
		MobileViews:   pageStats.MobileViews,
		OverviewViews: pageStats.OverviewViews,
		AboutViews:    pageStats.AboutViews,
		Engagement:    engagementRate,
		Posts:         0, // placeholder
		OrgName:       orgName,
		OrgURN:        orgURN,
	}, nil
}
