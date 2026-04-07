package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"golang.org/x/crypto/bcrypt"
)

// ──────────────────────────────────────────────
// GET /api/protected/team?company_id=X
// ──────────────────────────────────────────────

func ListTeamMembers(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	companyIDStr := c.Query("company_id")
	if companyIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "company_id parametresi gerekli"})
		return
	}

	// Sahiplik kontrolü — sadece kendi markalarının ekibini görebilir
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", companyIDStr, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu markaya erişim yetkiniz yok"})
		return
	}

	// team_members JOIN users
	var members []models.TeamMember
	if err := database.DB.
		Preload("User").
		Where("company_id = ?", company.ID).
		Order("created_at DESC").
		Find(&members).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ekip üyeleri alınamadı"})
		return
	}

	// İstemciye temiz veri döndür
	type MemberResponse struct {
		ID        uint   `json:"id"`
		UserID    uint   `json:"user_id"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Email     string `json:"email"`
		Role      string `json:"role"`
		CreatedAt string `json:"created_at"`
	}

	var result []MemberResponse
	for _, m := range members {
		result = append(result, MemberResponse{
			ID:        m.ID,
			UserID:    m.UserID,
			FirstName: m.User.FirstName,
			LastName:  m.User.LastName,
			Email:     m.User.Email,
			Role:      m.Role,
			CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	// Marka sahibini de (owner) listeye ekle
	var owner models.User
	if err := database.DB.First(&owner, userID).Error; err == nil {
		// Owner zaten team_members'da değilse ekle
		ownerExists := false
		for _, m := range result {
			if m.UserID == owner.ID {
				ownerExists = true
				break
			}
		}
		if !ownerExists {
			result = append([]MemberResponse{{
				ID:        0,
				UserID:    owner.ID,
				FirstName: owner.FirstName,
				LastName:  owner.LastName,
				Email:     owner.Email,
				Role:      "owner",
				CreatedAt: owner.CreatedAt.Format("2006-01-02T15:04:05Z"),
			}}, result...)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"members": result,
		"count":   len(result),
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/team/add
// ──────────────────────────────────────────────

type AddTeamMemberRequest struct {
	CompanyID uint   `json:"company_id" binding:"required"`
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name" binding:"required"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=6"`
	Role      string `json:"role" binding:"required"`
}

func AddTeamMember(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req AddTeamMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Geçerli roller
	validRoles := map[string]bool{
		"admin": true, "member": true, "client": true, "viewer": true,
	}
	if !validRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz rol. Geçerli roller: admin, member, client, viewer"})
		return
	}

	// Sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu markaya erişim yetkiniz yok"})
		return
	}

	// Kullanıcıyı bul veya oluştur
	var targetUser models.User
	err := database.DB.Where("email = ?", req.Email).First(&targetUser).Error

	if err != nil {
		// Kullanıcı yok → oluştur
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Şifre hashlenemedi"})
			return
		}

		targetUser = models.User{
			FirstName:    req.FirstName,
			LastName:     req.LastName,
			Email:        req.Email,
			PasswordHash: string(hash),
			AuthProvider: "email",
		}

		if createErr := database.DB.Create(&targetUser).Error; createErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Kullanıcı oluşturulamadı: " + createErr.Error()})
			return
		}

		log.Printf("✅ Yeni kullanıcı oluşturuldu: %s %s (%s)", targetUser.FirstName, targetUser.LastName, targetUser.Email)
	}

	// Kendini eklemeye çalışıyorsa engelle
	if targetUser.ID == userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Kendinizi ekip üyesi olarak ekleyemezsiniz, siz zaten marka sahibisiniz"})
		return
	}

	// Aynı markada zaten var mı?
	var existingMember models.TeamMember
	if err := database.DB.Where("company_id = ? AND user_id = ?", req.CompanyID, targetUser.ID).First(&existingMember).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Bu kullanıcı zaten bu markada ekip üyesi"})
		return
	}

	// TeamMember kaydını oluştur
	member := models.TeamMember{
		CompanyID: req.CompanyID,
		UserID:    targetUser.ID,
		Role:      req.Role,
	}

	if err := database.DB.Create(&member).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Ekip üyesi eklenemedi: " + err.Error()})
		return
	}

	log.Printf("✅ Ekip üyesi eklendi: %s %s → %s (role: %s)", targetUser.FirstName, targetUser.LastName, company.Name, req.Role)

	c.JSON(http.StatusCreated, gin.H{
		"message": "Ekip üyesi başarıyla eklendi",
		"member": gin.H{
			"id":         member.ID,
			"user_id":    targetUser.ID,
			"first_name": targetUser.FirstName,
			"last_name":  targetUser.LastName,
			"email":      targetUser.Email,
			"role":       member.Role,
		},
	})
}
