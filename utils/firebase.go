package utils

import (
	"context"
	"log"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"google.golang.org/api/option"
)

var firebaseAuth *auth.Client

// InitFirebase — Firebase Admin SDK'yı başlatır
func InitFirebase() {
	credFile := config.Get("FIREBASE_CREDENTIALS")
	if credFile == "" {
		credFile = "firebase-admin.json"
	}

	opt := option.WithCredentialsFile(credFile)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("❌ Firebase uygulaması başlatılamadı: %v", err)
	}

	firebaseAuth, err = app.Auth(context.Background())
	if err != nil {
		log.Fatalf("❌ Firebase Auth client oluşturulamadı: %v", err)
	}

	log.Println("✅ Firebase Admin SDK başlatıldı")
}

// FirebaseTokenInfo — doğrulanmış token'dan çıkarılan bilgiler
type FirebaseTokenInfo struct {
	UID   string
	Email string
	Name  string
}

// VerifyFirebaseToken — Firebase ID token'ı doğrular ve kullanıcı bilgilerini döner
func VerifyFirebaseToken(idToken string) (*FirebaseTokenInfo, error) {
	token, err := firebaseAuth.VerifyIDToken(context.Background(), idToken)
	if err != nil {
		log.Printf("❌ Firebase token doğrulama hatası: %v", err)
		return nil, err
	}

	// Token claims'den bilgileri çıkar
	email, _ := token.Claims["email"].(string)
	name, _ := token.Claims["name"].(string)

	return &FirebaseTokenInfo{
		UID:   token.UID,
		Email: email,
		Name:  name,
	}, nil
}
