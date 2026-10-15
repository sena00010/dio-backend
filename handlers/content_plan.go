package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// Aylık içerik planı: tek tıkla ayın gönderilerini içerik türü dengesi
// (en fazla %20 satış) ve Türk özel günleriyle üretir, takvime "planned" olarak ekler.
// ──────────────────────────────────────────────

var trMonths = []string{"", "Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}
var trWeekdays = []string{"Pazar", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi"}

// İçerik türleri — sıra, 5 gönderide en fazla 1 satış içeriği olacak şekilde
var pillarCycle = []string{"egitim", "etkilesim", "sosyal_kanit", "sahne_arkasi", "urun"}

var pillarBriefs = map[string]string{
	"egitim":       "Eğitici / bilgi veren içerik: hedef kitlenin bir sorununa pratik çözüm. Caption'da en az 3 somut, uygulanabilir madde (adım, ipucu veya bilgi) AÇIKÇA yazılmalı; 'işte bir tarif' deyip içeriği boş bırakma. Satış yok.",
	"etkilesim":    "Etkileşim içeriği: takipçiye soru soran, yorum yazdıran, anket/seçim yaptıran içerik. Satış yok.",
	"sosyal_kanit": "Sosyal kanıt: müşterilerin en çok tercih ettiği ürün/hizmet, tekrar gelme sebepleri veya markanın somut çalışma şekli. ASLA müşteri adı, alıntı, yorum, yıldız puanı veya rakam UYDURMA; gerçek yorum gerekiyorsa caption'a \"[Buraya gerçek bir müşteri yorumu ekleyin]\" yer tutucusunu koy.",
	"sahne_arkasi": "Sahne arkası: ekip, üretim/hizmet süreci, markanın değerleri ve insan yüzü.",
	"urun":         "Ürün / kampanya: tek bir ürün veya hizmetin net faydası ve güçlü bir CTA.",
	"ozel_gun":     "Özel gün içeriği.",
}

var pillarLabels = map[string]string{
	"egitim": "Eğitici", "etkilesim": "Etkileşim", "sosyal_kanit": "Sosyal Kanıt",
	"sahne_arkasi": "Sahne Arkası", "urun": "Ürün / Kampanya", "ozel_gun": "Özel Gün",
}

// Haftalık gönderi sayısına göre etkileşimin yüksek olduğu günler
var weekdaySets = map[int][]time.Weekday{
	1: {time.Thursday},
	2: {time.Tuesday, time.Thursday},
	3: {time.Tuesday, time.Thursday, time.Saturday},
	4: {time.Monday, time.Wednesday, time.Friday, time.Sunday},
	5: {time.Monday, time.Tuesday, time.Thursday, time.Friday, time.Saturday},
	6: {time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday},
	7: {time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday},
}

type planSlot struct {
	Date       time.Time
	Pillar     string
	SpecialDay *utils.SpecialDay
}

type planItem struct {
	ScheduleID  uint   `json:"schedule_id"`
	IdeaID      uint   `json:"idea_id"`
	Date        string `json:"date"`
	Pillar      string `json:"pillar"`
	PillarLabel string `json:"pillar_label"`
	Title       string `json:"title"`
	SpecialDay  string `json:"special_day,omitempty"`
}

type planJob struct {
	ID        string `json:"job_id"`
	userID    uint
	Status    string     `json:"status"` // running | completed
	Total     int        `json:"total"`
	Done      int        `json:"done"`
	Failed    int        `json:"failed"`
	Items     []planItem `json:"items"`
	createdAt time.Time
}

var (
	planJobs   = map[string]*planJob{}
	planJobsMu sync.Mutex
)

func istanbulLoc() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}

// buildPlanSlots — ayın gönderi günlerini ve içerik türlerini belirler (yalnızca yarından itibaren)
func buildPlanSlots(year int, month time.Month, perWeek int, now time.Time) []planSlot {
	loc := now.Location()
	first := time.Date(year, month, 1, 0, 0, 0, 0, loc)
	tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	start := first
	if tomorrow.After(start) {
		start = tomorrow
	}

	special := map[string]utils.SpecialDay{}
	for _, d := range utils.SpecialDaysInMonth(year, month, loc) {
		if !d.Date.Before(start) {
			special[d.Date.Format("2006-01-02")] = d
		}
	}

	wanted := map[time.Weekday]bool{}
	for _, wd := range weekdaySets[perWeek] {
		wanted[wd] = true
	}

	var slots []planSlot
	regular := 0
	for d := start; d.Month() == month; d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		if sd, ok := special[key]; ok {
			sdCopy := sd
			slots = append(slots, planSlot{Date: d.Add(10 * time.Hour), Pillar: "ozel_gun", SpecialDay: &sdCopy})
			continue
		}
		if wanted[d.Weekday()] {
			slots = append(slots, planSlot{Date: d.Add(19 * time.Hour), Pillar: pillarCycle[regular%len(pillarCycle)]})
			regular++
		}
	}
	return slots
}

func trDate(t time.Time) string {
	return fmt.Sprintf("%d %s %s", t.Day(), trMonths[t.Month()], trWeekdays[t.Weekday()])
}

// buildPlanTopic — bir grup slot için AI'a verilecek görev metni
func buildPlanTopic(slots []planSlot, monthLabel string, recentTitles []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "AYLIK İÇERİK PLANI (%s). Aşağıdaki listeyle BİREBİR aynı sırada TAM OLARAK %d fikir üret; her fikir listedeki maddeye karşılık gelsin:\n", monthLabel, len(slots))
	for i, s := range slots {
		if s.SpecialDay != nil {
			fmt.Fprintf(&b, "%d) %s — Özel gün: %s. %s\n", i+1, trDate(s.Date), s.SpecialDay.Name, utils.SpecialDayToneRule(s.SpecialDay.Kind))
		} else {
			fmt.Fprintf(&b, "%d) %s — İçerik türü: %s\n", i+1, trDate(s.Date), pillarBriefs[s.Pillar])
		}
	}
	if len(recentTitles) > 0 {
		fmt.Fprintf(&b, "Daha önce kullanılan bu başlıkları ve açılarını TEKRARLAMA: %s\n", strings.Join(recentTitles, " | "))
	}
	return b.String()
}

// GET /api/protected/special-days?year=2026&month=5
func GetSpecialDays(c *gin.Context) {
	year, _ := strconv.Atoi(c.Query("year"))
	month, _ := strconv.Atoi(c.Query("month"))
	if year < 2000 || month < 1 || month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz yıl veya ay"})
		return
	}
	days := utils.SpecialDaysInMonth(year, time.Month(month), istanbulLoc())
	out := make([]gin.H, 0, len(days))
	for _, d := range days {
		out = append(out, gin.H{"date": d.Date.Format("2006-01-02"), "name": d.Name, "kind": d.Kind})
	}
	c.JSON(http.StatusOK, gin.H{"days": out})
}

type contentPlanRequest struct {
	CompanyID uint   `json:"company_id" binding:"required"`
	Year      int    `json:"year" binding:"required"`
	Month     int    `json:"month" binding:"required"`
	PerWeek   int    `json:"posts_per_week"`
	Platform  string `json:"platform"`
}

// POST /api/protected/content-plan — işi başlatır, job_id döner (üretim arka planda sürer)
func CreateContentPlan(c *gin.Context) {
	userID := c.MustGet("userID").(uint)

	var req contentPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Month < 1 || req.Month > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz ay"})
		return
	}
	if req.PerWeek < 1 || req.PerWeek > 7 {
		req.PerWeek = 3
	}
	if req.Platform == "" {
		req.Platform = "Instagram"
	}

	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	loc := istanbulLoc()
	slots := buildPlanSlots(req.Year, time.Month(req.Month), req.PerWeek, time.Now().In(loc))
	if len(slots) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bu ayda planlanacak gün kalmadı. Sonraki ayı seçin."})
		return
	}

	var recent []models.AiIdea
	database.DB.Select("generated_title").Where("company_id = ?", company.ID).Order("created_at DESC").Limit(30).Find(&recent)
	recentTitles := make([]string, 0, len(recent))
	for _, r := range recent {
		if r.GeneratedTitle != "" {
			recentTitles = append(recentTitles, r.GeneratedTitle)
		}
	}

	idBytes := make([]byte, 8)
	rand.Read(idBytes)
	job := &planJob{ID: hex.EncodeToString(idBytes), userID: userID, Status: "running", Total: len(slots), createdAt: time.Now()}
	planJobsMu.Lock()
	for id, j := range planJobs { // eski işleri temizle
		if time.Since(j.createdAt) > time.Hour {
			delete(planJobs, id)
		}
	}
	planJobs[job.ID] = job
	planJobsMu.Unlock()

	monthLabel := fmt.Sprintf("%s %d", trMonths[req.Month], req.Year)
	go runContentPlan(job, company, req.Platform, slots, monthLabel, recentTitles)

	log.Printf("🗓️ Aylık plan başladı: company=%d %s, %d gönderi (job %s)", company.ID, monthLabel, len(slots), job.ID)
	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID, "total": len(slots)})
}

func runContentPlan(job *planJob, company models.Company, platform string, slots []planSlot, monthLabel string, recentTitles []string) {
	companyContext := buildCompanyContext(company)
	const batchSize = 4
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for startIdx := 0; startIdx < len(slots); startIdx += batchSize {
		end := startIdx + batchSize
		if end > len(slots) {
			end = len(slots)
		}
		batch := slots[startIdx:end]
		wg.Add(1)
		sem <- struct{}{}
		go func(batch []planSlot) {
			defer wg.Done()
			defer func() { <-sem }()

			topic := buildPlanTopic(batch, monthLabel, recentTitles)
			ideas, err := utils.GenerateIdeas(topic, platform, "monthly_plan", len(batch), companyContext)
			if err != nil {
				log.Printf("⚠️ Aylık plan grubu üretilemedi: %v", err)
			}

			for i, slot := range batch {
				if err != nil || i >= len(ideas) {
					planJobsMu.Lock()
					job.Failed++
					planJobsMu.Unlock()
					continue
				}
				item, saveErr := savePlanIdea(company.ID, platform, slot, ideas[i])
				planJobsMu.Lock()
				if saveErr != nil {
					job.Failed++
				} else {
					job.Done++
					job.Items = append(job.Items, item)
				}
				planJobsMu.Unlock()
			}
		}(batch)
	}
	wg.Wait()

	planJobsMu.Lock()
	job.Status = "completed"
	planJobsMu.Unlock()
	log.Printf("✅ Aylık plan bitti (job %s): %d hazır, %d başarısız", job.ID, job.Done, job.Failed)
}

func savePlanIdea(companyID uint, platform string, slot planSlot, idea utils.IdeaResult) (planItem, error) {
	contentItemsJSON, _ := json.Marshal(idea.ContentItems)
	designLayersJSON, _ := json.Marshal(idea.DesignLayers)
	if len(idea.DesignLayers) == 0 {
		designLayersJSON = []byte("[]")
	}
	specialName := ""
	if slot.SpecialDay != nil {
		specialName = slot.SpecialDay.Name
	}

	aiIdea := models.AiIdea{
		CompanyID:             companyID,
		UserInputTopic:        "Aylık plan: " + pillarLabels[slot.Pillar],
		Platform:              platform,
		CampaignType:          "monthly_plan",
		GeneratedTitle:        idea.Title,
		GeneratedDescription:  idea.Description,
		GeneratedHashtags:     idea.Hashtags,
		ImageSuggestionPrompt: idea.ImageSuggestionPrompt,
		SuggestedLayout:       idea.SuggestedLayout + "|" + idea.LogoPosition,
		SuggestedFont:         idea.SuggestedFont,
		ContentItems:          string(contentItemsJSON),
		DesignLayers:          string(designLayersJSON),
		ContentPillar:         slot.Pillar,
		SpecialDay:            specialName,
	}
	if err := database.DB.Create(&aiIdea).Error; err != nil {
		return planItem{}, err
	}

	schedule := models.AiSchedule{
		CompanyID:   companyID,
		IdeaID:      &aiIdea.ID,
		Platform:    platform,
		ScheduledAt: slot.Date,
		Status:      "planned",
	}
	if err := database.DB.Create(&schedule).Error; err != nil {
		return planItem{}, err
	}

	return planItem{
		ScheduleID:  schedule.ID,
		IdeaID:      aiIdea.ID,
		Date:        slot.Date.Format(time.RFC3339),
		Pillar:      slot.Pillar,
		PillarLabel: pillarLabels[slot.Pillar],
		Title:       aiIdea.GeneratedTitle,
		SpecialDay:  specialName,
	}, nil
}

// GET /api/protected/content-plan/status?job_id=...
func GetContentPlanStatus(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	planJobsMu.Lock()
	defer planJobsMu.Unlock()
	job, ok := planJobs[c.Query("job_id")]
	if !ok || job.userID != userID {
		c.JSON(http.StatusNotFound, gin.H{"error": "Plan işi bulunamadı"})
		return
	}
	c.JSON(http.StatusOK, job)
}
