package database

import (
	"log"

	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	dsn := config.Get("DB_DSN")
	if dsn == "" {
		log.Fatal("❌ DB_DSN ortam değişkeni tanımlı değil")
	}

	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ Veritabanına bağlanılamadı: %v", err)
	}

	log.Println("✅ MySQL bağlantısı başarılı")

	// AutoMigrate — tabloları oluştur / güncelle
	if err := DB.AutoMigrate(
		&models.User{},
		&models.Company{},
		&models.AiSchedule{},
		&models.Post{},
		&models.AiIdea{},
		&models.TeamMember{},
		&models.SeoIntegration{},
		&models.ProductImageStudio{},
		&models.ProductImageVariant{},
		&models.ProductImageAdVariant{},
	); err != nil {
		log.Fatalf("❌ AutoMigrate hatası: %v", err)
	}

	log.Println("✅ Tablo migration'ları tamamlandı")

	// Çoklu marka desteği: user_id unique index'ini kaldır (Ajans modeli)
	migrator := DB.Migrator()
	if migrator.HasIndex(&models.Company{}, "uni_companies_user_id") {
		if err := migrator.DropIndex(&models.Company{}, "uni_companies_user_id"); err != nil {
			log.Printf("⚠️ uni_companies_user_id drop hatası (muhtemelen zaten yok): %v", err)
		}
	}
	if migrator.HasIndex(&models.Company{}, "idx_companies_user_id") {
		// Zaten var, dokunma
	} else {
		DB.Exec("CREATE INDEX idx_companies_user_id ON companies(user_id)")
	}
}
