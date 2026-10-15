package utils

import (
	"sort"
	"time"
)

// SpecialDay — içerik planına girebilecek Türkiye'ye özel gün
type SpecialDay struct {
	Date time.Time `json:"date"`
	Name string    `json:"name"`
	// Kind: "commercial" (satış/kampanya uygun), "celebration" (kutlama, hafif satış),
	// "national" (milli bayram, satış yok), "religious" (dini bayram, saygılı kutlama),
	// "memorial" (anma günü, kesinlikle satış ve neşeli ton yok)
	Kind string `json:"kind"`
}

// Dini bayramlar her yıl kayar; Diyanet takvimine göre elle girilir.
// Yeni yıl eklerken Diyanet'in açıkladığı 1. gün tarihini kullan.
var religiousHolidays = map[int][]struct {
	month time.Month
	day   int
	name  string
}{
	2026: {{time.March, 20, "Ramazan Bayramı"}, {time.May, 27, "Kurban Bayramı"}},
	2027: {{time.March, 9, "Ramazan Bayramı"}, {time.May, 16, "Kurban Bayramı"}},
}

// nthWeekday — ayın n. haftasındaki gün (örn. Mayıs'ın 2. Pazarı)
func nthWeekday(year int, month time.Month, wd time.Weekday, n int, loc *time.Location) time.Time {
	d := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	for d.Weekday() != wd {
		d = d.AddDate(0, 0, 1)
	}
	return d.AddDate(0, 0, 7*(n-1))
}

// SpecialDaysInMonth — verilen ayın özel günlerini tarih sırasıyla döndürür
func SpecialDaysInMonth(year int, month time.Month, loc *time.Location) []SpecialDay {
	day := func(m time.Month, d int) time.Time { return time.Date(year, m, d, 0, 0, 0, 0, loc) }
	all := []SpecialDay{
		{day(time.January, 1), "Yılbaşı", "celebration"},
		{day(time.February, 14), "Sevgililer Günü", "commercial"},
		{day(time.March, 8), "Dünya Kadınlar Günü", "celebration"},
		{day(time.March, 21), "Nevruz", "celebration"},
		{day(time.April, 23), "23 Nisan Ulusal Egemenlik ve Çocuk Bayramı", "national"},
		{day(time.May, 1), "1 Mayıs Emek ve Dayanışma Günü", "celebration"},
		{day(time.May, 19), "19 Mayıs Atatürk'ü Anma, Gençlik ve Spor Bayramı", "national"},
		{nthWeekday(year, time.May, time.Sunday, 2, loc), "Anneler Günü", "commercial"},
		{nthWeekday(year, time.June, time.Sunday, 3, loc), "Babalar Günü", "commercial"},
		{day(time.July, 15), "15 Temmuz Demokrasi ve Milli Birlik Günü", "memorial"},
		{day(time.August, 30), "30 Ağustos Zafer Bayramı", "national"},
		{day(time.October, 29), "29 Ekim Cumhuriyet Bayramı", "national"},
		{day(time.November, 10), "10 Kasım Atatürk'ü Anma Günü", "memorial"},
		{day(time.November, 11), "11.11 İndirim Günü", "commercial"},
		{day(time.November, 24), "Öğretmenler Günü", "celebration"},
		// Black Friday: Kasım'ın 4. Perşembesinden sonraki Cuma
		{nthWeekday(year, time.November, time.Thursday, 4, loc).AddDate(0, 0, 1), "Black Friday", "commercial"},
		{day(time.December, 31), "Yılbaşı Gecesi", "commercial"},
	}
	for _, h := range religiousHolidays[year] {
		all = append(all, SpecialDay{day(h.month, h.day), h.name, "religious"})
	}

	var out []SpecialDay
	for _, d := range all {
		if d.Date.Month() == month {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

// SpecialDayToneRule — özel gün türüne göre içerik kuralı (AI'a verilir)
func SpecialDayToneRule(kind string) string {
	switch kind {
	case "memorial":
		return "Saygılı anma içeriği. Satış, indirim, ürün tanıtımı, emoji ve neşeli ton KESİNLİKLE yok."
	case "national":
		return "Gururlu, birleştirici kutlama mesajı. Doğrudan satış yok; marka sadece imza olarak yer alır."
	case "religious":
		return "Sıcak ve saygılı bayram tebriği. Agresif satış yok; isterse nazik bir bayram dileği ile sınırlı tut."
	case "celebration":
		return "Kutlama içeriği; hafif marka bağlantısı olabilir, satış ikinci planda."
	default:
		return "Kampanya/fırsat içeriği uygundur: net teklif ve güçlü CTA."
	}
}
