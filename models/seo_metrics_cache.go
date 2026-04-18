package models

import "time"

// SeoMetricsCache — GSC'den çekilen page ve query metriklerinin günlük cache tablosu
// Unique composite key: company_id + site_url + page_url + metric_type + record_date
type SeoMetricsCache struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	CompanyID   uint      `gorm:"index;not null" json:"company_id"`
	SiteURL     string    `gorm:"size:500;not null;index" json:"site_url"`  // Hangi GSC sitesine ait (sc-domain:x veya https://x)
	PageURL     string    `gorm:"size:2048;not null" json:"page_url"`       // URL (page) veya sorgu kelimesi (query)
	MetricType  string    `gorm:"size:10;not null" json:"metric_type"`      // 'page' veya 'query'
	Clicks      float64   `gorm:"default:0" json:"clicks"`
	Impressions float64   `gorm:"default:0" json:"impressions"`
	CTR         float64   `gorm:"default:0" json:"ctr"`
	Position    float64   `gorm:"default:0" json:"position"`
	RecordDate  string    `gorm:"size:10;not null;index" json:"record_date"` // YYYY-MM-DD
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
