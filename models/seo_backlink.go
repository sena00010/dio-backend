package models

import "time"

// SeoBacklinkSummary — Projenin genel backlink özet metrikleri (DataForSEO'dan)
type SeoBacklinkSummary struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ProjectID        uint      `gorm:"uniqueIndex;not null" json:"project_id"`
	Domain           string    `gorm:"size:255" json:"domain"` // MySQL constraint sorununu gidermek için eklendi
	TotalBacklinks   int       `json:"total_backlinks"`
	ReferringDomains int       `json:"referring_domains"`
	RankScore        float64   `json:"rank_score"`          // DataForSEO rank (0-1000)
	DomainRating     int       `json:"domain_rating"`       // Hesaplanan DR (0-100)
	DoFollowCount    int       `json:"dofollow_count"`
	NoFollowCount    int       `json:"nofollow_count"`
	SpamLinks        int       `json:"spam_links"`          // spam_score > 30 olanlar
	BrokenLinks      int       `json:"broken_links"`
	NewBacklinks7d   int       `json:"new_backlinks_7d"`    // Son 7 günde kazanılan
	LostBacklinks7d  int       `json:"lost_backlinks_7d"`   // Son 7 günde kaybedilen
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

	// Relations
	Project SeoProject `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}

func (SeoBacklinkSummary) TableName() string {
	return "seo_backlink_summaries"
}

// SeoBacklinkItem — Tek bir backlink kaydı (en güçlü 100 link)
type SeoBacklinkItem struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	ProjectID    uint       `gorm:"index;not null" json:"project_id"`
	TargetDomain string    `gorm:"size:500" json:"target_domain"`
	SourceDomain string    `gorm:"size:500" json:"source_domain"`
	SourceURL    string    `gorm:"size:1000" json:"source_url"`
	TargetURL    string    `gorm:"size:1000" json:"target_url"`
	AnchorText   string    `gorm:"size:500" json:"anchor_text"`
	Rank         float64   `json:"rank"`
	SpamScore    float64   `json:"spam_score"`
	IsDofollow   bool      `json:"is_dofollow"`
	LinkType     string    `gorm:"size:50" json:"link_type"` // anchor, image, redirect, etc.
	FirstSeenAt  *time.Time `json:"first_seen_at,omitempty"`
	CreatedAt    time.Time `json:"created_at"`

	// Relations
	Project SeoProject `gorm:"foreignKey:ProjectID" json:"project,omitempty"`
}

func (SeoBacklinkItem) TableName() string {
	return "seo_backlink_items"
}
