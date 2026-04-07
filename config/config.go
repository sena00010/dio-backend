package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

func LoadEnv() {
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env dosyası bulunamadı, sistem ortam değişkenleri kullanılacak")
	}
}

func Get(key string) string {
	return os.Getenv(key)
}
