package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// SEORoleMiddleware — Sadece admin, owner veya seo_expert rolüne sahip kullanıcılara izin verir
func SEORoleMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userIDVal, exists := c.Get("userID")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
			return
		}
		userID := userIDVal.(uint)

		// company_id query veya header'dan al
		companyIDStr := c.Query("company_id")
		if companyIDStr == "" {
			companyIDStr = c.GetHeader("X-Company-ID")
		}
		if companyIDStr == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "company_id parametresi gerekli"})
			return
		}

		companyID, err := strconv.ParseUint(companyIDStr, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Geçersiz company_id"})
			return
		}

		// 1) Marka sahibi mi? (owner)
		var company models.Company
		if err := database.DB.Where("id = ? AND user_id = ?", companyID, userID).First(&company).Error; err == nil {
			// Owner — tam yetki
			c.Set("companyID", uint(companyID))
			c.Set("seoRole", "owner")
			c.Next()
			return
		}

		// 2) Team member olarak rolü uygun mu?
		var member models.TeamMember
		if err := database.DB.Where("company_id = ? AND user_id = ?", companyID, userID).First(&member).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Bu markaya erişim yetkiniz yok"})
			return
		}

		allowedRoles := map[string]bool{
			"admin":      true,
			"seo_expert": true,
		}

		if !allowedRoles[member.Role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "SEO modülüne erişim yetkiniz yok. Gerekli roller: admin, seo_expert"})
			return
		}

		c.Set("companyID", uint(companyID))
		c.Set("seoRole", member.Role)
		c.Next()
	}
}
