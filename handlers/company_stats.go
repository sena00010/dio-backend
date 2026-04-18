package handlers

import (
	"net/http"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/database"

	"github.com/gin-gonic/gin"
)

// BrandStat — tek bir marka/şirket için toplanan istatistikler
type BrandStat struct {
	CompanyID          uint      `json:"company_id"`
	TotalIdeas         int64     `json:"total_ideas"`          // ai_ideas tablosundaki toplam fikir
	SelectedIdeas      int64     `json:"selected_ideas"`       // is_selected = 1 olanlar
	TotalPosts         int64     `json:"total_posts"`          // posts tablosu
	PublishedPosts     int64     `json:"published_posts"`      // status = 'published'
	ScheduledPosts     int64     `json:"scheduled_posts"`      // ai_schedules tablosu
	PostsWithImage     int64     `json:"posts_with_image"`     // image_url dolu post sayısı
	PlatformBreakdown  map[string]int64 `json:"platform_breakdown"` // platform → adet
	LastActivityAt     *time.Time `json:"last_activity_at"`     // en son içerik oluşturma tarihi
}

// GetCompanyStats godoc
// GET /api/protected/companies/stats
// Giriş yapan kullanıcının tüm markalarına ait istatistikleri döner
func GetCompanyStats(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	db := database.DB

	// Kullanıcıya ait company ID'leri
	var companyIDs []uint
	db.Table("companies").Where("user_id = ?", userID).Pluck("id", &companyIDs)

	if len(companyIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{"stats": []BrandStat{}})
		return
	}

	result := make([]BrandStat, 0, len(companyIDs))

	for _, cid := range companyIDs {
		stat := BrandStat{
			CompanyID:         cid,
			PlatformBreakdown: make(map[string]int64),
		}

		// ── ai_ideas istatistikleri ──
		db.Table("ai_ideas").Where("company_id = ?", cid).Count(&stat.TotalIdeas)
		db.Table("ai_ideas").Where("company_id = ? AND is_selected = 1", cid).Count(&stat.SelectedIdeas)

		// ── posts istatistikleri ──
		db.Table("posts").Where("company_id = ?", cid).Count(&stat.TotalPosts)
		db.Table("posts").Where("company_id = ? AND status = 'published'", cid).Count(&stat.PublishedPosts)
		db.Table("posts").Where("company_id = ? AND image_url IS NOT NULL AND image_url != ''", cid).Count(&stat.PostsWithImage)

		// ── ai_schedules istatistikleri ──
		db.Table("ai_schedules").Where("company_id = ?", cid).Count(&stat.ScheduledPosts)

		// ── Platform bazlı dağılım (ai_ideas'den) ──
		type PlatformRow struct {
			Platform string
			Cnt      int64
		}
		var platformRows []PlatformRow
		db.Table("ai_ideas").
			Select("LOWER(platform) as platform, COUNT(*) as cnt").
			Where("company_id = ? AND platform IS NOT NULL AND platform != ''", cid).
			Group("LOWER(platform)").
			Find(&platformRows)
		for _, pr := range platformRows {
			if pr.Platform != "" {
				stat.PlatformBreakdown[pr.Platform] = pr.Cnt
			}
		}

		// ── Son aktivite tarihi (en son oluşturulan ai_idea veya post) ──
		var lastIdeaDate, lastPostDate *time.Time

		db.Table("ai_ideas").
			Where("company_id = ?", cid).
			Select("MAX(created_at)").
			Row().
			Scan(&lastIdeaDate)

		db.Table("posts").
			Where("company_id = ?", cid).
			Select("MAX(created_at)").
			Row().
			Scan(&lastPostDate)

		if lastIdeaDate != nil && lastPostDate != nil {
			if lastIdeaDate.After(*lastPostDate) {
				stat.LastActivityAt = lastIdeaDate
			} else {
				stat.LastActivityAt = lastPostDate
			}
		} else if lastIdeaDate != nil {
			stat.LastActivityAt = lastIdeaDate
		} else if lastPostDate != nil {
			stat.LastActivityAt = lastPostDate
		}

		result = append(result, stat)
	}

	c.JSON(http.StatusOK, gin.H{"stats": result})
}
