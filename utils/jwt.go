package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kullaniciadiniz/dio-panel-backend/config"
)

type JWTClaims struct {
	UserID uint `json:"user_id"`
	jwt.RegisteredClaims
}

// GenerateToken — kullanıcı ID'si ile 72 saatlik JWT üretir
func GenerateToken(userID uint) (string, error) {
	secret := config.Get("JWT_SECRET")
	if secret == "" {
		return "", errors.New("JWT_SECRET tanımlı değil")
	}

	claims := JWTClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(72 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "dio-panel",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken — JWT'yi doğrulayıp userID döner
func ParseToken(tokenStr string) (uint, error) {
	secret := config.Get("JWT_SECRET")
	if secret == "" {
		return 0, errors.New("JWT_SECRET tanımlı değil")
	}

	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("beklenmeyen imzalama yöntemi")
		}
		return []byte(secret), nil
	})

	if err != nil {
		return 0, err
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return 0, errors.New("geçersiz token")
	}

	return claims.UserID, nil
}
