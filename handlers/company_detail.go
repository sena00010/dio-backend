package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/kullaniciadiniz/dio-panel-backend/database"

	"github.com/gin-gonic/gin"
)

// ── Response structs ──

type IdeaSummary struct {
	ID          uint      `json:"id"`
	Title       string    `json:"title"`
	Platform    string    `json:"platform"`
	Campaign    string    `json:"campaign_type"`
	IsSelected  bool      `json:"is_selected"`
	AIScore     int       `json:"ai_score"`
	ImageURL    string    `json:"image_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type StudioSummary struct {
	ID              uint      `json:"id"`
	Title           string    `json:"title"`
	SourceImageURL  string    `json:"source_image_url"`
	Category        string    `json:"category"`
	Status          string    `json:"status"`
	VariantCount    int64     `json:"variant_count"`
	AdVariantCount  int64     `json:"ad_variant_count"`
	CreatedAt       time.Time `json:"created_at"`
}

type BriefSummary struct {
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	BriefType string    `json:"brief_type"`
	Tags      string    `json:"tags,omitempty"`
	IsFavorite bool     `json:"is_favorite"`
	UsedCount  int      `json:"used_count"`
	CreatedAt  time.Time `json:"created_at"`
}

type ScheduleSummary struct {
	ID          uint      `json:"id"`
	PostID      *uint     `json:"post_id,omitempty"`
	Platform    string    `json:"platform"`
	ScheduledAt time.Time `json:"scheduled_at"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type AdVariantSummary struct {
	ID          uint   `json:"id"`
	StudioID    uint   `json:"studio_id"`
	Name        string `json:"name"`
	Platform    string `json:"platform"`
	Format      string `json:"format_label"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Score       int    `json:"score"`
	Status      string `json:"status"`
	PreviewURL  string `json:"preview_url,omitempty"`
	Headline    string `json:"headline,omitempty"`
	SubHeadline string `json:"sub_headline,omitempty"`
	CTA         string `json:"cta,omitempty"`
}

type CompanyDetailResponse struct {
	Ideas       []IdeaSummary      `json:"ideas"`
	Studios     []StudioSummary    `json:"studios"`
	Briefs      []BriefSummary     `json:"briefs"`
	Schedules   []ScheduleSummary  `json:"schedules"`
	AdVariants  []AdVariantSummary `json:"ad_variants"`

	// Aggregate counts
	TotalIdeas      int64 `json:"total_ideas"`
	SelectedIdeas   int64 `json:"selected_ideas"`
	TotalPosts      int64 `json:"total_posts"`
	PostsWithImage  int64 `json:"posts_with_image"`
	TotalStudios    int64 `json:"total_studios"`
	TotalBriefs     int64 `json:"total_briefs"`
	TotalScheduled  int64 `json:"total_scheduled"`
	TotalAdVariants int64 `json:"total_ad_variants"`
	TotalImgVariants int64 `json:"total_img_variants"`
}

// GetCompanyDetail godoc
// GET /api/protected/companies/:id/detail
// Returns comprehensive activity data for a specific company/brand
func GetCompanyDetail(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	companyIDStr := c.Param("id")
	companyID, err := strconv.ParseUint(companyIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid company ID"})
		return
	}

	db := database.DB

	// Verify ownership
	var ownerCheck int64
	db.Table("companies").Where("id = ? AND user_id = ?", companyID, userID).Count(&ownerCheck)
	if ownerCheck == 0 {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	result := CompanyDetailResponse{}
	cid := uint(companyID)

	// ── AI Ideas (son 50) ──
	var ideas []IdeaSummary
	db.Table("ai_ideas").
		Select("id, generated_title as title, platform, campaign_type, is_selected, ai_score, image_url, created_at").
		Where("company_id = ?", cid).
		Order("created_at DESC").
		Limit(50).
		Find(&ideas)
	result.Ideas = ideas
	if result.Ideas == nil {
		result.Ideas = []IdeaSummary{}
	}

	db.Table("ai_ideas").Where("company_id = ?", cid).Count(&result.TotalIdeas)
	db.Table("ai_ideas").Where("company_id = ? AND is_selected = 1", cid).Count(&result.SelectedIdeas)

	// ── Posts ──
	db.Table("posts").Where("company_id = ?", cid).Count(&result.TotalPosts)
	db.Table("posts").Where("company_id = ? AND image_url IS NOT NULL AND image_url != ''", cid).Count(&result.PostsWithImage)

	// ── Product Image Studios ──
	var studios []StudioSummary
	db.Table("product_image_studios").
		Select("id, title, source_image_url, category, status, created_at").
		Where("company_id = ?", cid).
		Order("created_at DESC").
		Find(&studios)
	// Fill variant counts
	for i, s := range studios {
		db.Table("product_image_variants").Where("studio_id = ?", s.ID).Count(&studios[i].VariantCount)
		db.Table("product_image_ad_variants").Where("studio_id = ?", s.ID).Count(&studios[i].AdVariantCount)
	}
	result.Studios = studios
	if result.Studios == nil {
		result.Studios = []StudioSummary{}
	}
	result.TotalStudios = int64(len(studios))

	// ── Studio Briefs ──
	var briefs []BriefSummary
	db.Table("studio_briefs").
		Select("id, title, content, brief_type, tags, is_favorite, used_count, created_at").
		Where("company_id = ?", cid).
		Order("created_at DESC").
		Find(&briefs)
	result.Briefs = briefs
	if result.Briefs == nil {
		result.Briefs = []BriefSummary{}
	}
	result.TotalBriefs = int64(len(briefs))

	// ── Schedules ──
	var schedules []ScheduleSummary
	db.Table("ai_schedules").
		Select("id, post_id, platform, scheduled_at, status, created_at").
		Where("company_id = ?", cid).
		Order("scheduled_at DESC").
		Find(&schedules)
	result.Schedules = schedules
	if result.Schedules == nil {
		result.Schedules = []ScheduleSummary{}
	}
	result.TotalScheduled = int64(len(schedules))

	// ── Ad Variants (tüm studio'lar üzerinden) ──
	var studioIDs []uint
	db.Table("product_image_studios").Where("company_id = ?", cid).Pluck("id", &studioIDs)
	if len(studioIDs) > 0 {
		var adVariants []AdVariantSummary
		db.Table("product_image_ad_variants").
			Select("id, studio_id, name, platform, format_label, width, height, score, status, preview_url, headline, sub_headline, cta").
			Where("studio_id IN ?", studioIDs).
			Order("score DESC").
			Find(&adVariants)
		result.AdVariants = adVariants
		result.TotalAdVariants = int64(len(adVariants))
	}
	if result.AdVariants == nil {
		result.AdVariants = []AdVariantSummary{}
	}

	// ── Total image variants ──
	if len(studioIDs) > 0 {
		db.Table("product_image_variants").Where("studio_id IN ?", studioIDs).Count(&result.TotalImgVariants)
	}

	c.JSON(http.StatusOK, result)
}
