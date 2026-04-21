package handlers

import (
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"golang.org/x/crypto/bcrypt"
)

// ──────────────────────────────────────────────
// Admin Auth — Register & Login
// ──────────────────────────────────────────────

type AdminRegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	FullName string `json:"full_name" binding:"required"`
}

type AdminLoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// POST /api/admin/register
func AdminRegister(c *gin.Context) {
	var req AdminRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Şifre karmaşıklık kontrolü: 1 büyük, 1 küçük, 1 sayı, en az 8 karakter
	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Şifre en az 8 karakter olmalıdır"})
		return
	}
	hasUpper := regexp.MustCompile(`[A-Z]`).MatchString(req.Password)
	hasLower := regexp.MustCompile(`[a-z]`).MatchString(req.Password)
	hasNumber := regexp.MustCompile(`[0-9]`).MatchString(req.Password)

	if !hasUpper || !hasLower || !hasNumber {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Şifre en az 1 büyük harf, 1 küçük harf ve 1 rakam içermelidir"})
		return
	}

	// Email zaten var mı kontrol
	var existing models.AdminUser
	if err := database.DB.Where("email = ?", req.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Bu email ile zaten bir admin kayıtlı"})
		return
	}

	// Şifre hashle
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Şifre hashlenemedi"})
		return
	}

	admin := models.AdminUser{
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		FullName:     req.FullName,
		Role:         "admin",
		IsActive:     true,
	}

	if err := database.DB.Create(&admin).Error; err != nil {
		log.Printf("❌ Admin kayıt hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Admin oluşturulamadı"})
		return
	}

	// JWT token üret
	token, err := generateAdminJWT(admin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token oluşturulamadı"})
		return
	}

	log.Printf("✅ Yeni admin kayıt oldu: %s (%s)", admin.FullName, admin.Email)
	c.JSON(http.StatusCreated, gin.H{
		"message": "Admin başarıyla oluşturuldu",
		"token":   token,
		"admin": gin.H{
			"id":        admin.ID,
			"email":     admin.Email,
			"full_name": admin.FullName,
			"role":      admin.Role,
		},
	})
}

// POST /api/admin/login
func AdminLogin(c *gin.Context) {
	var req AdminLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var admin models.AdminUser
	if err := database.DB.Where("email = ?", req.Email).First(&admin).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email veya şifre hatalı"})
		return
	}

	if !admin.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu hesap devre dışı bırakılmış"})
		return
	}

	// Şifre doğrula
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Email veya şifre hatalı"})
		return
	}

	// Son giriş zamanını güncelle
	now := time.Now()
	admin.LastLoginAt = &now
	database.DB.Save(&admin)

	// JWT token üret
	token, err := generateAdminJWT(admin)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token oluşturulamadı"})
		return
	}

	log.Printf("✅ Admin giriş yaptı: %s", admin.Email)
	c.JSON(http.StatusOK, gin.H{
		"message": "Giriş başarılı",
		"token":   token,
		"admin": gin.H{
			"id":        admin.ID,
			"email":     admin.Email,
			"full_name": admin.FullName,
			"role":      admin.Role,
		},
	})
}

// GET /api/admin/me
func AdminMe(c *gin.Context) {
	adminID, _ := c.Get("adminID")

	var admin models.AdminUser
	if err := database.DB.First(&admin, adminID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Admin bulunamadı"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"admin": gin.H{
			"id":            admin.ID,
			"email":         admin.Email,
			"full_name":     admin.FullName,
			"role":          admin.Role,
			"last_login_at": admin.LastLoginAt,
			"created_at":    admin.CreatedAt,
		},
	})
}

// ──────────────────────────────────────────────
// JWT Token üretimi
// ──────────────────────────────────────────────

func generateAdminJWT(admin models.AdminUser) (string, error) {
	secret := os.Getenv("ADMIN_JWT_SECRET")
	if secret == "" {
		secret = os.Getenv("JWT_SECRET")
	}
	if secret == "" {
		secret = "dio-admin-secret-key-2024"
	}

	claims := jwt.MapClaims{
		"admin_id": admin.ID,
		"email":    admin.Email,
		"role":     admin.Role,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}
