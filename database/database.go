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
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
		// ProductImageStudio <-> Variant karşılıklı FK yüzünden migrate sırası kırılmasın
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		log.Fatalf("❌ Veritabanına bağlanılamadı: %v", err)
	}

	log.Println("✅ MySQL bağlantısı başarılı")

	// AutoMigrate — tabloları oluştur / güncelle
	if err := DB.AutoMigrate(
		&models.User{},
		&models.Company{},
		&models.CompanyCompetitor{},
		&models.AiSchedule{},
		&models.Post{},
		&models.PostSlide{},
		&models.AiIdea{},
		&models.TeamMember{},
		&models.SeoIntegration{},
		&models.ProductImageStudio{},
		&models.ProductImageVariant{},
		&models.ProductImageAdVariant{},
		&models.SeoProject{},
		&models.SeoProjectSnapshot{},
		&models.SeoPerformanceDaily{},
		&models.SeoDiscoveryMetrics{},
		&models.SeoActionTask{},
		&models.SeoTrackedKeyword{},
		&models.SeoKeywordDailyStat{},
		&models.SeoContentBrief{},
		&models.SeoTechnicalHealth{},
		&models.SeoBacklinkSummary{},
		&models.SeoBacklinkItem{},
		&models.SeoAIVisibility{},
		&models.SeoCompetitor{},
		&models.SeoMetricsCache{},
		&models.UserMetaAccount{},
		&models.UserGoogleAdsAccount{},
		&models.UserTikTokAccount{},
		&models.UserLinkedInAccount{},
		&models.SystemPrompt{},
		&models.AdminUser{},
		&models.AiModelSetting{},
	); err != nil {
		log.Fatalf("❌ AutoMigrate hatası: %v", err)
	}

	log.Println("✅ Tablo migration'ları tamamlandı")

	// Migrator instance (index yönetimi için)
	migrator := DB.Migrator()

	// seo_metrics_caches tablosuna unique composite index ekle (site_url dahil)
	if migrator.HasIndex(&models.SeoMetricsCache{}, "idx_seo_metrics_cache_unique") {
		// Eski index'i kaldır (site_url içermiyordu)
		DB.Exec("DROP INDEX idx_seo_metrics_cache_unique ON seo_metrics_caches")
	}
	if !migrator.HasIndex(&models.SeoMetricsCache{}, "idx_seo_metrics_cache_unique_v2") {
		DB.Exec("CREATE UNIQUE INDEX idx_seo_metrics_cache_unique_v2 ON seo_metrics_caches(company_id, site_url(255), page_url(255), metric_type, record_date)")
		log.Println("✅ seo_metrics_caches unique index v2 oluşturuldu (site_url dahil)")
	}

	// Çoklu marka desteği: user_id unique index'ini kaldır (Ajans modeli)
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
