package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// JSONMap — MySQL JSON sütunlarını Go'da güvenli okuma/yazma
type JSONMap map[string]interface{}

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

func (j *JSONMap) Scan(src interface{}) error {
	if src == nil {
		*j = JSONMap{}
		return nil
	}
	var source []byte
	switch v := src.(type) {
	case string:
		source = []byte(v)
	case []byte:
		source = v
	default:
		return nil
	}
	return json.Unmarshal(source, j)
}

// JSONSlice — MySQL JSON array sütunları
type JSONSlice []interface{}

func (j JSONSlice) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	b, err := json.Marshal(j)
	return string(b), err
}

func (j *JSONSlice) Scan(src interface{}) error {
	if src == nil {
		*j = JSONSlice{}
		return nil
	}
	var source []byte
	switch v := src.(type) {
	case string:
		source = []byte(v)
	case []byte:
		source = v
	default:
		return nil
	}
	return json.Unmarshal(source, j)
}

// SeoDiscoveryMetrics — Kırılım verileri (Cihaz, Ülke, Arama Türü, En İyi Sorgular, Sayfalar)
type SeoDiscoveryMetrics struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	ProjectID         uint      `gorm:"index;not null" json:"project_id"`
	PeriodLabel       string    `gorm:"size:20;not null" json:"period_label"` // "28d"
	DeviceBreakdown   JSONSlice `gorm:"type:json" json:"device_breakdown"`   // [{name:"Mobil", clicks:2690, pct:70}, ..]
	CountryBreakdown  JSONSlice `gorm:"type:json" json:"country_breakdown"`  // [{code:"TR", name:"Türkiye", clicks:3410, pct:89}, ..]
	SearchTypeBreakdown JSONSlice `gorm:"type:json" json:"search_type_breakdown"` // [{name:"Web", clicks:3205, pct:83}, ..]
	TopQueries        JSONSlice `gorm:"type:json" json:"top_queries"`        // [{query:"...", type:"Non-Brand", clicks:487, ...}]
	TopPages          JSONSlice `gorm:"type:json" json:"top_pages"`          // [{page:"/urunler/...", clicks:892, ...}]
	CTROpportunities  JSONSlice `gorm:"type:json" json:"ctr_opportunities"`  // [{query:"...", impressions:11200, ctr:0.012, pos:19.4}]
	RisingQueries     JSONSlice `gorm:"type:json" json:"rising_queries"`     // [{query:"...", impressions:"4.7K", change:"+%41"}]
	FallingQueries    JSONSlice `gorm:"type:json" json:"falling_queries"`    // [{query:"...", pos:14.1, change:"+12"}]
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
