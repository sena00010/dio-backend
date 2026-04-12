package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// ActionData — JSON sütunundaki esnek veri
type ActionData map[string]interface{}

func (a ActionData) Value() (driver.Value, error) {
	if a == nil {
		return "{}", nil
	}
	b, err := json.Marshal(a)
	return string(b), err
}

func (a *ActionData) Scan(src interface{}) error {
	if src == nil {
		*a = ActionData{}
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
	return json.Unmarshal(source, a)
}

// SeoActionTask — Aksiyon motoru tarafından üretilen görevler
type SeoActionTask struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	ProjectID     uint       `gorm:"not null;index" json:"project_id"`
	Category      string     `gorm:"size:50;not null" json:"category"`       // critical, ctr_opportunity, growth, technical
	TaskType      string     `gorm:"size:100;not null" json:"task_type"`     // low_ctr, cannibalization, competitor_weakness, ...
	Title         string     `gorm:"size:500;not null" json:"title"`
	Description   string     `gorm:"type:text" json:"description"`
	Severity      string     `gorm:"size:20;default:'medium'" json:"severity"` // critical, high, medium, low
	Status        string     `gorm:"size:20;default:'open'" json:"status"`     // open, in_progress, done, dismissed
	PotentialGain float64    `json:"potential_gain"`                            // tahmini tık/ay kazancı
	ActionData    ActionData `gorm:"type:json" json:"action_data"`             // detay verisi (URL, CTR, pozisyon vs.)
	ActionLabel   string     `gorm:"size:100" json:"action_label"`             // buton metni: "AI Yaz", "Düzelt", ...
	LookupKey     string     `gorm:"size:500;index" json:"lookup_key"`         // upsert için: project_id + task_type + benzersiz anahtar
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
