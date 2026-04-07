package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ──────────────────────────────────────────────
// Request / Response yapıları
// ──────────────────────────────────────────────

type RegisterRequest struct {
	FirstName   string `json:"first_name" binding:"required"`
	LastName    string `json:"last_name" binding:"required"`
	Email       string `json:"email" binding:"required,email"`
	PhoneNumber string `json:"phone_number"`
	Password    string `json:"password" binding:"required,min=6"`
	CompanyName string `json:"company_name" binding:"required"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type GoogleLoginRequest struct {
	IDToken string `json:"id_token" binding:"required"`
}

type AuthResponse struct {
	Token       string  `json:"token"`
	HasCompany  bool    `json:"has_company"`
	CurrentStep int     `json:"current_step"`
	User        UserDTO `json:"user"`
}

type UserDTO struct {
	ID          uint   `json:"id"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phone_number,omitempty"`
	AuthProvider string `json:"auth_provider"`
}

// ──────────────────────────────────────────────
// POST /api/auth/register
// ──────────────────────────────────────────────

func Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Email benzersizlik kontrolü
	var existing models.User
	if err := database.DB.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Bu e-posta adresi zaten kayıtlı"})
		return
	}

	// Şifreyi hashle
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Şifre işlenemedi"})
		return
	}

	// User oluştur
	user := models.User{
		FirstName:    req.FirstName,
		LastName:     req.LastName,
		Email:        req.Email,
		PhoneNumber:  req.PhoneNumber,
		PasswordHash: string(hash),
		AuthProvider: "email",
	}

	if err := database.DB.Create(&user).Error; err != nil {
		log.Printf("❌ Register - Kullanıcı oluşturma hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Kullanıcı oluşturulamadı: " + err.Error()})
		return
	}

	// Company oluştur
	company := models.Company{
		UserID: user.ID,
		Name:   req.CompanyName,
	}

	if err := database.DB.Create(&company).Error; err != nil {
		log.Printf("❌ Register - Şirket oluşturma hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Şirket oluşturulamadı: " + err.Error()})
		return
	}

	// JWT üret
	token, err := utils.GenerateToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token üretilemedi"})
		return
	}

	hasCompany, currentStep := checkCompanyStatus(user.ID)

	c.JSON(http.StatusCreated, AuthResponse{
		Token:       token,
		HasCompany:  hasCompany,
		CurrentStep: currentStep,
		User:        toUserDTO(user),
	})
}

// ──────────────────────────────────────────────
// POST /api/auth/login
// ──────────────────────────────────────────────

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Kullanıcıyı bul
	var user models.User
	if err := database.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "E-posta veya şifre hatalı"})
		return
	}

	// Şifre doğrula (Google ile kayıtlı kullanıcılar şifreyle giriş yapamaz)
	if user.AuthProvider == "google" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bu hesap Google ile kayıtlı. Lütfen Google ile giriş yapın."})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "E-posta veya şifre hatalı"})
		return
	}

	// has_company + current_step kontrolü
	hasCompany, currentStep := checkCompanyStatus(user.ID)

	// JWT üret
	token, err := utils.GenerateToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token üretilemedi"})
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		Token:       token,
		HasCompany:  hasCompany,
		CurrentStep: currentStep,
		User:        toUserDTO(user),
	})
}

// ──────────────────────────────────────────────
// POST /api/auth/google
// ──────────────────────────────────────────────

func GoogleLogin(c *gin.Context) {
	var req GoogleLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 🔐 Gerçek Firebase ID Token doğrulama
	fbInfo, err := utils.VerifyFirebaseToken(req.IDToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Geçersiz Firebase token: " + err.Error()})
		return
	}

	// Veritabanında FirebaseUID ile ara
	var user models.User
	dbErr := database.DB.Where("firebase_uid = ?", fbInfo.UID).First(&user).Error

	if dbErr == gorm.ErrRecordNotFound {
		// Yeni kullanıcı oluştur — Firebase'den gelen gerçek bilgilerle
		firstName, lastName := splitName(fbInfo.Name)
		uid := fbInfo.UID
		user = models.User{
			FirebaseUID:  &uid,
			FirstName:    firstName,
			LastName:     lastName,
			Email:        fbInfo.Email,
			AuthProvider: "google",
		}
		if err := database.DB.Create(&user).Error; err != nil {
			log.Printf("❌ GoogleLogin - Kullanıcı oluşturma hatası: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Kullanıcı oluşturulamadı: " + err.Error()})
			return
		}
		log.Printf("✅ Google ile yeni kullanıcı oluşturuldu: %s (%s)", fbInfo.Email, fbInfo.UID)
	} else if dbErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Veritabanı hatası"})
		return
	}

	// has_company + current_step kontrolü
	hasCompany, currentStep := checkCompanyStatus(user.ID)

	// JWT üret
	token, err := utils.GenerateToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token üretilemedi"})
		return
	}

	c.JSON(http.StatusOK, AuthResponse{
		Token:       token,
		HasCompany:  hasCompany,
		CurrentStep: currentStep,
		User:        toUserDTO(user),
	})
}

// ──────────────────────────────────────────────
// Yardımcı fonksiyonlar
// ──────────────────────────────────────────────

func checkCompanyStatus(userID uint) (bool, int) {
	var company models.Company
	if err := database.DB.Where("user_id = ?", userID).First(&company).Error; err != nil {
		return false, 0
	}
	return true, company.CurrentStep
}

func toUserDTO(u models.User) UserDTO {
	return UserDTO{
		ID:           u.ID,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Email:        u.Email,
		PhoneNumber:  u.PhoneNumber,
		AuthProvider: u.AuthProvider,
	}
}

// splitName — "Ad Soyad" formatını ayırır
func splitName(fullName string) (string, string) {
	if fullName == "" {
		return "Google", "User"
	}
	for i, ch := range fullName {
		if ch == ' ' {
			return fullName[:i], fullName[i+1:]
		}
	}
	return fullName, ""
}

// ──────────────────────────────────────────────
// GET /api/auth/me
// Token'dan kullanıcı + şirket bilgilerini döner
// ──────────────────────────────────────────────

func GetMe(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	userID := userIDVal.(uint)

	// Kullanıcıyı bul
	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Kullanıcı bulunamadı"})
		return
	}

	// Tüm şirketleri çek
	var companies []models.Company
	database.DB.Where("user_id = ?", userID).Order("id ASC").Find(&companies)

	var companyResp interface{}
	if len(companies) > 0 {
		company := companies[0]
		companyResp = gin.H{
			"id":               company.ID,
			"name":             company.Name,
			"sector":           company.Sector,
			"goal":             company.Goal,
			"content_language": company.ContentLanguage,
			"platforms":        company.Platforms,
			"website":          company.Website,
			"logo_url":         company.LogoURL,
			"brand_colors":     company.BrandColors,
			"slogan":           company.Slogan,
			"description":      company.Description,
			"forbidden_words":  company.ForbiddenWords,
			"target_audience":  company.TargetAudience,
			"product_categories": company.ProductCategories,
			"current_step":     company.CurrentStep,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":            user.ID,
			"first_name":    user.FirstName,
			"last_name":     user.LastName,
			"email":         user.Email,
			"phone":         user.PhoneNumber,
			"auth_provider": user.AuthProvider,
		},
		"company":   companyResp,
		"companies": companies,
	})
}
