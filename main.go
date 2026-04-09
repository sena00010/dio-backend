package main

import (
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/handlers"
	"github.com/kullaniciadiniz/dio-panel-backend/middleware"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)

func main() {
	// .env dosyasını yükle
	config.LoadEnv()

	// Veritabanı bağlantısı
	database.Connect()

	// Firebase Admin SDK başlat
	utils.InitFirebase()

	// Gin router
	r := gin.Default()

	// CORS ayarları — Frontend'in backend'e erişimi için
	r.Use(cors.New(cors.Config{
		// YENİ ADRESİMİZİ LİSTEYE EKLEDİK!
		AllowOrigins:     []string{"http://localhost:3000", "http://localhost:3001", "http://187.77.109.127:8000"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))

	// Static dosya sunumu — CORS header'lı (frontend'den erişim için)
	r.GET("/uploads/*filepath", func(c *gin.Context) {
		// BURAYI DA HERKESE AÇIK (YILDIZ) YAPIYORUZ Kİ RESİMLER GÖRÜNSÜN
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.File("./assets/uploads" + c.Param("filepath"))
	})

	// ──────────────────────────────────────────
	// Auth rotaları (public)
	// ──────────────────────────────────────────
	auth := r.Group("/api/auth")
	{
		auth.POST("/register", handlers.Register)
		auth.POST("/login", handlers.Login)
		auth.POST("/google", handlers.GoogleLogin)
	}

	// Google OAuth2 callback — public (Google tarayıcıyı yönlendirir, JWT yok)
	r.GET("/api/seo/auth/google/callback", handlers.SeoGoogleCallback)

	// AI Image proxy — public (img tag Authorization header gönderemez)
	r.GET("/api/ai-image", handlers.ProxyAIImage)

	// ──────────────────────────────────────────
	// Korumalı rotalar (JWT gerekli)
	// ──────────────────────────────────────────
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware())
	{
		// Kullanıcı bilgileri
		api.GET("/me", handlers.GetMe)
		api.GET("/auth/me", handlers.GetMe)

		// Company rotaları
		api.GET("/companies/me", handlers.GetMyCompany)
		api.POST("/companies/onboarding", handlers.CompanyOnboarding)
		api.POST("/onboarding/research", handlers.BrandResearch)
		api.POST("/protected/companies", handlers.CreateCompany)
		api.PUT("/protected/companies/:id", handlers.UpdateCompany)
		api.POST("/protected/companies/generate-description", handlers.GenerateBrandDesc)

		// AI İçerik Fikirleri
		api.POST("/protected/generate-ideas", handlers.GenerateIdeas)
		api.PATCH("/protected/update-idea", handlers.UpdateIdea)
		api.POST("/protected/regenerate-idea", handlers.RegenerateIdea)
		api.POST("/protected/approve-idea", handlers.ApproveIdea)

		// Post & Görsel Stüdyosu
		api.GET("/protected/posts", handlers.ListPosts)
		api.GET("/protected/posts/:id", handlers.GetPost)
		api.PATCH("/protected/posts/:id", handlers.UpdatePost)
		api.POST("/protected/generate-image", handlers.GeneratePostImage)
		api.POST("/protected/generate-multiple-images", handlers.GenerateMultipleImages)
		api.POST("/protected/select-image", handlers.SelectImage)
		api.POST("/protected/upload-canvas", handlers.UploadCanvas)

		// Ekip Yönetimi
		api.GET("/protected/team", handlers.ListTeamMembers)
		api.POST("/protected/team/add", handlers.AddTeamMember)

		// İçerik Takvimi
		api.GET("/protected/pending-contents", handlers.GetPendingContents)
		api.GET("/protected/schedules", handlers.ListSchedules)
		api.POST("/protected/schedules", handlers.CreateSchedule)
		api.POST("/protected/schedules/update", handlers.UpdateSchedule)
		api.DELETE("/protected/schedules/:id", handlers.DeleteSchedule)

		// Kreatif Stüdyo — AI Konu Önerileri
		api.POST("/protected/topic-suggestions", handlers.GenerateTopicSuggestions)
		api.POST("/protected/trend-radar", handlers.GenerateTrendRadar)
		api.POST("/protected/creative-dashboard", handlers.CreativeDashboard) // Combined paralel endpoint
		api.POST("/protected/generate-creatives", handlers.GenerateCreatives)
		api.POST("/protected/generate-description", handlers.GenerateDescription)    // AI marka açıklaması
		api.POST("/protected/generate-tone-examples", handlers.GenerateToneExamples) // AI ton örnekleri
		api.POST("/protected/save-creative", handlers.SaveCreative)
		api.POST("/protected/save-creative-idea", handlers.SaveCreativeIdea)
		api.POST("/protected/save-creative-ideas-batch", handlers.SaveCreativeIdeasBatch)
		api.POST("/protected/generate-creative-images", handlers.GenerateCreativeImages)
		api.GET("/protected/creative-images-status", handlers.GetCreativeImagesStatus)
		api.POST("/protected/migrate-ideas-to-posts", handlers.MigrateIdeasToPosts)
		api.GET("/protected/product-image-studio/sidebar", handlers.GetProductStudioSidebar)
		api.GET("/protected/product-image-studio", handlers.ListProductImageStudios)
		api.POST("/protected/product-image-studio/upload", handlers.UploadProductImageStudioSource)
		api.GET("/protected/product-image-studio/:id", handlers.GetProductImageStudio)
		api.PATCH("/protected/product-image-studio/:id", handlers.UpdateProductImageStudio)
		api.POST("/protected/product-image-studio/:id/analyze", handlers.AnalyzeProductImageStudio)
		api.POST("/protected/product-image-studio/:id/generate", handlers.GenerateProductImageStudio)
		api.POST("/protected/product-image-studio/:id/generate-ads", handlers.GenerateProductImageAdStudio)
		api.GET("/protected/product-image-studio/jobs/:jobID", handlers.GetProductImageStudioJob)
		api.POST("/protected/product-image-studio/variants/:variantID/select", handlers.SelectProductImageVariant)
		api.POST("/protected/product-image-studio/ad-variants/:variantID/select", handlers.SelectProductImageAdVariant)

		// ──────────────────────────────────────────
		// SEO rotaları (JWT + SEO rol kontrolü)
		// ──────────────────────────────────────────

		seo := api.Group("/seo")
		seo.Use(middleware.SEORoleMiddleware())
		{
			seo.GET("/auth/google/login", handlers.SeoGoogleLogin)
			seo.GET("/integration/status", handlers.SeoIntegrationStatus)
			seo.GET("/dashboard/metrics", handlers.SeoDashboardMetrics)
			seo.GET("/sites", handlers.SeoListSites)
			seo.DELETE("/integration/disconnect", handlers.SeoDisconnect)
		}
	}

	// Sunucuyu başlat
	port := config.Get("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Dio Panel Backend %s portunda çalışıyor...\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("❌ Sunucu başlatılamadı: %v", err)
	}
}
