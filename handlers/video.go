package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// Aynı anda en fazla 2 video işlenir (CPU yoğun ffmpeg)
var videoSlots = make(chan struct{}, 2)

type createVideoRequest struct {
	CompanyID   uint   `json:"company_id" binding:"required"`
	PostID      *uint  `json:"post_id"`
	Title       string `json:"title"`
	Brief       string `json:"brief"`
	DurationSec int    `json:"duration_sec"`
	Format      string `json:"format"`
	Voice       string `json:"voice"`
	// İsteğe bağlı: "İNKA=İnka, İK=insan kaynakları"
	Pronunciation string `json:"pronunciation"`
	Language      string `json:"language"`
	Subtitles     *bool  `json:"subtitles"` // varsayılan açık
	Intro         *bool  `json:"intro"`     // varsayılan açık
	SourceURL     string `json:"source_url"`
	Platform      string `json:"platform"`
}

// POST /api/protected/videos — videoyu kuyruğa alır, üretim arka planda sürer
func CreateVideo(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	var req createVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek"})
		return
	}
	req.SourceURL = strings.TrimSpace(req.SourceURL)
	if strings.TrimSpace(req.Brief) == "" && req.SourceURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Videonun anlatacağı metni yazın ya da bir link girin"})
		return
	}
	if _, ok := utils.VideoLanguages[req.Language]; !ok {
		req.Language = "tr"
	}
	subtitles, intro := true, true
	if req.Subtitles != nil {
		subtitles = *req.Subtitles
	}
	if req.Intro != nil {
		intro = *req.Intro
	}
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}
	if err := utils.VideoToolsAvailable(); err != nil {
		log.Printf("❌ Video araçları eksik: %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Video oluşturma şu an kullanılamıyor"})
		return
	}
	if req.DurationSec < 10 {
		req.DurationSec = 10
	}
	if req.DurationSec > 180 {
		req.DurationSec = 180
	}
	// Format ve azami süre mecraya göre
	platCode, plat := videoPlatformOf(req.Platform)
	req.Platform = platCode
	req.Format = plat.Format
	if req.DurationSec > plat.MaxSec {
		req.DurationSec = plat.MaxSec
	}
	switch req.Voice {
	case "kadin", "erkek", "yok":
	default:
		req.Voice = "kadin"
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSpace(req.Brief)
		if title == "" {
			title = req.SourceURL
		}
		if r := []rune(title); len(r) > 60 {
			title = string(r[:60]) + "…"
		}
	}

	v := models.VideoProject{
		CompanyID: company.ID, PostID: req.PostID, Title: title, Brief: req.Brief,
		DurationSec: req.DurationSec, Format: req.Format, Voice: req.Voice, Pronunciation: truncate(req.Pronunciation, 480),
		Language: req.Language, Subtitles: subtitles, Intro: intro, SourceURL: truncate(req.SourceURL, 490), Platform: req.Platform,
		Status: "queued", Stage: "Sırada bekliyor", Progress: 2,
	}
	if err := database.DB.Create(&v).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Video kaydı oluşturulamadı"})
		return
	}
	go runVideo(v.ID, company)
	c.JSON(http.StatusAccepted, gin.H{"video": v})
}

func runVideo(id uint, company models.Company) {
	videoSlots <- struct{}{}
	defer func() { <-videoSlots }()

	var v models.VideoProject
	if database.DB.First(&v, id).Error != nil {
		return
	}
	update := func(fields map[string]interface{}) {
		database.DB.Model(&models.VideoProject{}).Where("id = ?", id).Updates(fields)
	}
	progress := func(stage string, p int) {
		update(map[string]interface{}{"status": "running", "stage": stage, "progress": p})
	}
	fail := func(msg string) {
		log.Printf("❌ Video %d başarısız: %s", id, msg)
		update(map[string]interface{}{"status": "failed", "stage": "Hata", "error": truncate(msg, 480)})
	}

	start := time.Now()
	brief := v.Brief
	if v.SourceURL != "" {
		progress("Link içeriği okunuyor", 3)
		title, text, err := utils.FetchPageText(v.SourceURL, 6000)
		if err != nil {
			fail("Link okunamadı: " + err.Error())
			return
		}
		brief = fmt.Sprintf("KAYNAK SAYFA (%s): %s\n%s", v.SourceURL, title, text)
		if strings.TrimSpace(v.Brief) != "" {
			brief += "\n\nKULLANICI NOTU (öncelikli): " + v.Brief
		}
		if title != "" && (v.Title == "" || v.Title == v.SourceURL) {
			update(map[string]interface{}{"title": truncate(title, 250)})
		}
	}
	progress("Senaryo yazılıyor", 5)
	req := utils.VideoRequest{
		Brief: brief, CompanyContext: buildCompanyContext(company), BrandName: company.Name,
		Language: v.Language, Subtitles: v.Subtitles, Intro: v.Intro, LogoURL: company.LogoURL, Platform: v.Platform,
		Sector: company.Sector, BrandColors: company.BrandColors,
		DurationSec: v.DurationSec, Format: v.Format, Voice: v.Voice, Pronunciations: v.Pronunciation,
	}
	scenes, err := utils.GenerateVideoScript(req)
	if err != nil {
		fail(err.Error())
		return
	}
	script, _ := json.Marshal(scenes)
	update(map[string]interface{}{"script": string(script)})

	url, thumb, err := utils.RenderVideo(req, scenes, progress)
	if err != nil {
		fail(err.Error())
		return
	}
	// Zaman akışlı senaryo ve mecraya hazır paylaşım metni
	timed, _ := json.Marshal(scenes)
	update(map[string]interface{}{"script": string(timed)})
	progress("Paylaşım metni yazılıyor", 98)
	if cp, e := utils.GenerateVideoCopy(req, scenes); e == nil {
		update(map[string]interface{}{"post_title": truncate(cp.Title, 250), "post_text": cp.Caption, "hashtags": truncate(cp.Hashtags, 480)})
	} else {
		log.Printf("⚠️ Video %d paylaşım metni üretilemedi: %v", id, e)
	}
	update(map[string]interface{}{"status": "done", "stage": "Hazır", "progress": 100, "video_url": url, "thumb_url": thumb, "error": ""})
	log.Printf("🎬 Video %d hazır (%d sahne, %v): %s", id, len(scenes), time.Since(start).Round(time.Second), url)
}

func videoPlatformOf(code string) (string, utils.VideoPlatform) {
	if p, ok := utils.VideoPlatforms[code]; ok {
		return code, p
	}
	return "instagram", utils.VideoPlatforms["instagram"]
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// markStaleVideos — sunucu yeniden başladıysa yarım kalan işleri başarısız say
func markStaleVideos(companyID uint) {
	database.DB.Model(&models.VideoProject{}).
		Where("company_id = ? AND status IN ? AND updated_at < ?", companyID, []string{"queued", "running"}, time.Now().Add(-20*time.Minute)).
		Updates(map[string]interface{}{"status": "failed", "stage": "Hata", "error": "İşlem yarıda kaldı, lütfen tekrar deneyin"})
}

// GET /api/protected/videos?company_id=X
func ListVideos(c *gin.Context) {
	userID := c.MustGet("userID").(uint)
	cid, _ := strconv.Atoi(c.Query("company_id"))
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", cid, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}
	markStaleVideos(company.ID)
	var list []models.VideoProject
	database.DB.Where("company_id = ?", company.ID).Order("created_at DESC").Limit(30).Find(&list)
	c.JSON(http.StatusOK, gin.H{"videos": list})
}
