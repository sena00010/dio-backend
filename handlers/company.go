package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

// ──────────────────────────────────────────────
// POST /api/companies/onboarding
// multipart/form-data — Partial update + step increment
// ──────────────────────────────────────────────

func CompanyOnboarding(c *gin.Context) {
	// JWT middleware'den gelen userID
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	// ─── Mevcut şirket var mı kontrol et ───
	var company models.Company
	isNew := false

	if err := database.DB.Where("user_id = ?", userID).First(&company).Error; err != nil {
		// Yeni Company oluştur
		company = models.Company{
			UserID:      userID,
			CurrentStep: 0,
		}
		isNew = true
	}

	// ─── Form alanlarını güncelle (partial update) ───
	updateCompanyFromForm(c, &company)

	// ─── current_step işle ───
	if stepStr := c.PostForm("current_step"); stepStr != "" {
		if stepVal, err := strconv.Atoi(stepStr); err == nil {
			company.CurrentStep = stepVal
		}
	} else {
		// current_step gönderilmediyse otomatik artır
		company.CurrentStep++
	}

	// ─── Logo dosyası varsa işle ───
	logoURL, err := handleLogoUpload(c)
	if err != nil && err.Error() != "http: no such file" {
		log.Printf("❌ Logo yükleme hatası: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Logo yüklenemedi: " + err.Error()})
		return
	}
	if logoURL != "" {
		company.LogoURL = logoURL
	}

	// Kullanıcı dosya yüklemediyse ama AI'dan gelen harici logo URL varsa → indir ve kaydet
	if logoURL == "" {
		if extURL := c.PostForm("external_logo_url"); extURL != "" {
			downloaded, err := utils.DownloadExternalLogo(extURL)
			if err != nil {
				log.Printf("⚠️ Harici logo indirilemedi: %v", err)
			} else {
				company.LogoURL = downloaded
				log.Printf("✅ Harici logo indirildi: %s", downloaded)
			}
		}
	}

	// ─── Kaydet ───
	if isNew {
		if err := database.DB.Create(&company).Error; err != nil {
			log.Printf("❌ Şirket oluşturma hatası: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Şirket oluşturulamadı: " + err.Error()})
			return
		}
		log.Printf("✅ Yeni şirket oluşturuldu: %s (user_id: %d, step: %d)", company.Name, userID, company.CurrentStep)
		c.JSON(http.StatusCreated, gin.H{
			"message":      "Şirket başarıyla oluşturuldu",
			"company":      company,
			"current_step": company.CurrentStep,
		})
	} else {
		if err := database.DB.Save(&company).Error; err != nil {
			log.Printf("❌ Şirket güncelleme hatası: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Şirket güncellenemedi"})
			return
		}
		log.Printf("✅ Şirket güncellendi: %s (user_id: %d, step: %d)", company.Name, userID, company.CurrentStep)
		c.JSON(http.StatusOK, gin.H{
			"message":      "Şirket bilgileri başarıyla güncellendi",
			"company":      company,
			"current_step": company.CurrentStep,
		})
	}
}

// ──────────────────────────────────────────────
// GET /api/companies/me
// Kullanıcının TÜM markalarını dizi olarak döner
// ──────────────────────────────────────────────

func GetMyCompany(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var companies []models.Company
	if err := database.DB.Preload("Competitors").Where("user_id = ?", userID).Order("id ASC").Find(&companies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Markalar yüklenemedi"})
		return
	}

	if len(companies) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"companies":   []interface{}{},
			"has_company": false,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"companies":    companies,
		"has_company":  true,
		"current_step": companies[0].CurrentStep,
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/companies
// Yeni marka oluştur (multipart/form-data)
// ──────────────────────────────────────────────

func CreateCompany(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	// Marka adı zorunlu
	name := c.PostForm("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Marka adı zorunludur"})
		return
	}

	// Yeni company struct — form alanlarını doldur
	company := models.Company{
		UserID:      userID,
		CurrentStep: 5, // Onboarding tamamlanmış kabul et
	}
	updateCompanyFromForm(c, &company)

	// Logo dosyası varsa işle
	logoURL, err := handleLogoUpload(c)
	if err != nil && err.Error() != "http: no such file" {
		log.Printf("⚠️ Logo yükleme hatası: %v", err)
	}
	if logoURL != "" {
		company.LogoURL = logoURL
	}

	if err := database.DB.Create(&company).Error; err != nil {
		log.Printf("❌ Yeni marka oluşturma hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Marka oluşturulamadı"})
		return
	}

	log.Printf("✅ Yeni marka oluşturuldu: %s (user_id: %d)", company.Name, userID)
	c.JSON(http.StatusCreated, gin.H{
		"message": "Marka başarıyla oluşturuldu",
		"company": company,
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/companies/generate-description
// ──────────────────────────────────────────────

func GenerateBrandDesc(c *gin.Context) {
	var req struct {
		Name    string `json:"name"`
		Website string `json:"website"`
		Sector  string `json:"sector"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek"})
		return
	}
	
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Marka adı boş olamaz"})
		return
	}

	description, err := utils.GenerateBrandDescription(req.Name, req.Website, req.Sector)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI açıklaması oluşturulamadı: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"description": description})
}

// ──────────────────────────────────────────────
// GET /api/protected/posts?company_id=X
// Markaya ait postları listele
// ──────────────────────────────────────────────

func ListPosts(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var posts []models.Post
	companyIDStr := c.Query("company_id")

	if companyIDStr != "" {
		// ── Belirli bir markanın postları ──
		companyID, err := strconv.ParseUint(companyIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz company_id"})
			return
		}
		// Sahiplik kontrolü
		var company models.Company
		if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Bu markaya erişim yetkiniz yok"})
			return
		}
		database.DB.Where("company_id = ?", companyID).Order("created_at DESC").Find(&posts)
	} else {
		// ── Kullanıcının TÜM markalarının postları ──
		var companyIDs []uint
		database.DB.Model(&models.Company{}).Where("user_id = ?", userID).Pluck("id", &companyIDs)
		if len(companyIDs) > 0 {
			database.DB.Where("company_id IN ?", companyIDs).Order("created_at DESC").Find(&posts)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"posts": posts,
		"total": len(posts),
	})
}

// ──────────────────────────────────────────────
// Form alanlarını Company struct'ına aktar (partial)
// ──────────────────────────────────────────────

func updateCompanyFromForm(c *gin.Context, company *models.Company) {
	if v := c.PostForm("name"); v != "" {
		company.Name = v
	}
	if v := c.PostForm("sector"); v != "" {
		company.Sector = v
	}
	if v := c.PostForm("goal"); v != "" {
		company.Goal = v
	}
	if v := c.PostForm("content_language"); v != "" {
		company.ContentLanguage = v
	}
	if v := c.PostForm("platforms"); v != "" {
		company.Platforms = v
	}
	if v := c.PostForm("website"); v != "" {
		company.Website = v
	}
	if v := c.PostForm("brand_colors"); v != "" {
		company.BrandColors = v
	}
	if v := c.PostForm("slogan"); v != "" {
		company.Slogan = v
	}
	if v := c.PostForm("description"); v != "" {
		company.Description = v
	}
	if v := c.PostForm("forbidden_words"); v != "" {
		company.ForbiddenWords = v
	}
	if v := c.PostForm("target_audience"); v != "" {
		company.TargetAudience = v
	}
	if v := c.PostForm("product_categories"); v != "" {
		company.ProductCategories = v
	}
	if v := c.PostForm("dynamic_fields"); v != "" {
		company.DynamicFields = &v
	}
	if v := c.PostForm("tone_of_voice"); v != "" {
		company.ToneOfVoice = v
	}
	if v := c.PostForm("brand_type"); v != "" {
		company.BrandType = v
	}
	if v := c.PostForm("brand_font"); v != "" {
		company.BrandFont = v
	}
	if v := c.PostForm("social_media"); v != "" {
		company.SocialMedia = v
	}
	if v := c.PostForm("competitive_edge"); v != "" {
		company.CompetitiveEdge = v
	}
}

// ──────────────────────────────────────────────
// Logo dosyasını diske kaydet
// ──────────────────────────────────────────────

func handleLogoUpload(c *gin.Context) (string, error) {
	file, err := c.FormFile("logo")
	if err != nil {
		return "", err
	}

	// Dosya boyutu kontrolü (max 5MB)
	if file.Size > 5*1024*1024 {
		return "", fmt.Errorf("dosya boyutu 5MB'dan büyük olamaz")
	}

	// Uzantı kontrolü
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowedExts := map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".svg": true}
	if !allowedExts[ext] {
		return "", fmt.Errorf("desteklenmeyen dosya formatı: %s", ext)
	}

	// Benzersiz dosya adı (timestamp)
	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)

	// Dosyayı oku
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("dosya açılamadı: %v", err)
	}
	defer src.Close()

	fileData, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("dosya okunamadı: %v", err)
	}

	// Bunny CDN yapılandırılmışsa → CDN'e yükle
	if utils.IsBunnyCDNConfigured() {
		cdnURL, err := utils.UploadToBunnyCDN(fileData, "logos", filename)
		if err != nil {
			log.Printf("⚠️ Bunny CDN logo upload başarısız, lokale kaydediliyor: %v", err)
		} else {
			log.Printf("✅ Logo Bunny CDN'e kaydedildi: %s", cdnURL)
			return cdnURL, nil
		}
	}

	// Fallback: lokale kaydet
	uploadDir := "./assets/uploads/logos"
	if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("dizin oluşturulamadı: %v", err)
	}

	savePath := filepath.Join(uploadDir, filename)
	if err := os.WriteFile(savePath, fileData, 0644); err != nil {
		return "", fmt.Errorf("dosya kaydedilemedi: %v", err)
	}

	// URL olarak döndür
	logoURL := "/uploads/logos/" + filename
	log.Printf("✅ Logo kaydedildi: %s", logoURL)
	return logoURL, nil
}

// ──────────────────────────────────────────────
// PUT /api/protected/companies/:id
// Mevcut markayı güncelle (multipart/form-data)
// Companies tablosundaki TÜM alanları destekler
// ──────────────────────────────────────────────

func UpdateCompany(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	// URL'den company ID al
	companyID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz marka ID"})
		return
	}

	// Sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu markaya erişim yetkiniz yok"})
		return
	}

	// ─── Tüm alanları güncelle (gönderilmeyenler değişmez) ───
	if v := c.PostForm("name"); v != "" {
		company.Name = v
	}
	if v := c.PostForm("sector"); v != "" {
		company.Sector = v
	}
	if v := c.PostForm("goal"); v != "" {
		company.Goal = v
	}
	if v := c.PostForm("content_language"); v != "" {
		company.ContentLanguage = v
	}
	if v := c.PostForm("platforms"); v != "" {
		company.Platforms = v
	}
	if v := c.PostForm("website"); v != "" {
		company.Website = v
	}
	if v := c.PostForm("brand_colors"); v != "" {
		company.BrandColors = v
	}
	if v := c.PostForm("slogan"); v != "" {
		company.Slogan = v
	}
	if v := c.PostForm("description"); v != "" {
		company.Description = v
	}
	if v := c.PostForm("forbidden_words"); v != "" {
		company.ForbiddenWords = v
	}
	if v := c.PostForm("target_audience"); v != "" {
		company.TargetAudience = v
	}
	if v := c.PostForm("product_categories"); v != "" {
		company.ProductCategories = v
	}
	if v := c.PostForm("dynamic_fields"); v != "" {
		company.DynamicFields = &v
	}
	if v := c.PostForm("tone_of_voice"); v != "" {
		company.ToneOfVoice = v
	}
	if v := c.PostForm("brand_type"); v != "" {
		company.BrandType = v
	}
	if v := c.PostForm("brand_font"); v != "" {
		company.BrandFont = v
	}
	if v := c.PostForm("social_media"); v != "" {
		company.SocialMedia = v
	}
	if v := c.PostForm("competitive_edge"); v != "" {
		company.CompetitiveEdge = v
	}
	if v := c.PostForm("current_step"); v != "" {
		if stepVal, err := strconv.Atoi(v); err == nil {
			company.CurrentStep = stepVal
		}
	}

	// ─── Logo dosyası varsa işle ───
	logoURL, err := handleLogoUpload(c)
	if err != nil && err.Error() != "http: no such file" {
		log.Printf("⚠️ Logo yükleme hatası: %v", err)
	}
	if logoURL != "" {
		company.LogoURL = logoURL
	}

	// ─── Kaydet ───
	if err := database.DB.Save(&company).Error; err != nil {
		log.Printf("❌ Marka güncelleme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Marka güncellenemedi: " + err.Error()})
		return
	}

	log.Printf("✅ Marka güncellendi: %s (id: %d, user_id: %d)", company.Name, company.ID, userID)
	c.JSON(http.StatusOK, gin.H{
		"message": "Marka başarıyla güncellendi",
		"company": company,
	})
}
