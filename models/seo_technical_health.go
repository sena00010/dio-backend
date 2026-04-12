package models

import "time"

// SeoTechnicalHealth — PSI + GSC Coverage teknik sağlık verileri
type SeoTechnicalHealth struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProjectID uint      `gorm:"not null;index" json:"project_id"`

	// ── PageSpeed Insights (Mobil) ──
	MobileScore int     `json:"mobile_score"`          // 0-100
	LcpMs       float64 `json:"lcp_ms"`                // Largest Contentful Paint (ms)
	FidMs       float64 `json:"fid_ms"`                // First Input Delay / TBT (ms)
	ClsScore    float64 `json:"cls_score"`              // Cumulative Layout Shift
	TtfbMs      float64 `json:"ttfb_ms"`               // Time to First Byte (ms)
	FcpMs       float64 `json:"fcp_ms"`                // First Contentful Paint (ms)
	SiMs        float64 `json:"si_ms"`                  // Speed Index (ms)

	// ── PageSpeed Insights (Masaüstü) ──
	DesktopScore int     `json:"desktop_score"`
	DesktopLcp   float64 `json:"desktop_lcp"`
	DesktopFid   float64 `json:"desktop_fid"`
	DesktopCls   float64 `json:"desktop_cls"`
	DesktopTtfb  float64 `json:"desktop_ttfb"`

	// ── GSC Coverage ──
	IndexedPages      int `json:"indexed_pages"`
	NotIndexedPages   int `json:"not_indexed_pages"`
	TotalSubmitted    int `json:"total_submitted"`      // Sitemap'ten

	// ── Genel Sağlık Skoru ──
	OverallHealthScore int `json:"overall_health_score"` // 0-100

	// ── Schema Durumu ──
	SchemaMissing int `json:"schema_missing"`

	// ── Mobil Uyumluluk ──
	MobileFriendly bool `json:"mobile_friendly"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SeoTechnicalHealth) TableName() string {
	return "seo_technical_health"
}
