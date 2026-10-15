package handlers

import (
	"testing"
	"time"
)

func TestBuildPlanSlots(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, time.April, 20, 12, 0, 0, 0, loc)
	slots := buildPlanSlots(2026, time.May, 3, now)

	sales, regular := 0, 0
	var special []string
	for _, s := range slots {
		if s.Date.Month() != time.May {
			t.Fatalf("slot outside month: %v", s.Date)
		}
		if s.SpecialDay != nil {
			special = append(special, s.SpecialDay.Name)
			continue
		}
		regular++
		if s.Pillar == "urun" {
			sales++
		}
	}
	if sales*5 > regular {
		t.Errorf("satış içeriği %%20'yi aşıyor: %d/%d", sales, regular)
	}
	if len(special) < 4 { // 1 Mayıs, Anneler Günü, 19 Mayıs, Kurban Bayramı
		t.Errorf("özel günler eksik: %v", special)
	}

	// Ay ortasında başlanırsa geçmiş günler planlanmaz
	mid := buildPlanSlots(2026, time.May, 3, time.Date(2026, time.May, 20, 9, 0, 0, 0, loc))
	for _, s := range mid {
		if s.Date.Day() <= 20 {
			t.Errorf("geçmiş/bugün planlandı: %v", s.Date)
		}
	}

	if got := buildPlanSlots(2026, time.May, 3, time.Date(2026, time.June, 2, 0, 0, 0, 0, loc)); len(got) != 0 {
		t.Errorf("geçmiş ay için slot üretilmemeli, %d geldi", len(got))
	}
}
