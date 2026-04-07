package main

import (
	"fmt"
	"log"

	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	config.LoadEnv()

	dsn := config.Get("DB_DSN")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// Foreign key kontrolünü kapat
	db.Exec("SET FOREIGN_KEY_CHECKS = 0")

	tables := []string{"ai_schedules", "posts", "companies", "users"}
	for _, t := range tables {
		if err := db.Exec("DROP TABLE IF EXISTS " + t).Error; err != nil {
			log.Printf("Tablo %s silinemedi: %v", t, err)
		} else {
			fmt.Printf("✅ %s tablosu silindi\n", t)
		}
	}

	db.Exec("SET FOREIGN_KEY_CHECKS = 1")
	fmt.Println("✅ Tüm tablolar temizlendi. Şimdi 'go run main.go' ile yeniden başlatın.")
}
