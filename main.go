package main

import (
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/config"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/handlers"
	"github.com/kullaniciadiniz/dio-panel-backend/middleware"
	"github.com/kullaniciadiniz/dio-panel-backend/services"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
	"github.com/robfig/cron/v3"
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
		// DOMAIN VE IP ADRESLERI BURADA!
		AllowOrigins: []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"http://187.77.109.127:8000",
			"http://app.dijitalisortagi.com",
			"https://app.dijitalisortagi.com",
			//  // <-- YENİ DOMAINIMIZ EKLENDI
		},
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		// DİKKAT: Google Login vs. için gizli başlıklar eklendi (Accept, X-Requested-With, Cache-Control)
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Requested-With", "Cache-Control"},
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

	// Meta OAuth2 callback — public (Facebook tarayıcıyı yönlendirir, JWT yok)
	r.GET("/api/meta/auth/callback", handlers.MetaOAuthCallback)
	r.GET("/auth/facebook/callback", handlers.MetaOAuthCallback)

	// Google Ads OAuth2 callback — public (Google tarayıcıyı yönlendirir, JWT yok)
	r.GET("/api/google/auth/callback", handlers.GoogleAdsCallback)

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
		api.GET("/protected/companies/stats", handlers.GetCompanyStats)
		api.GET("/protected/companies/:id/detail", handlers.GetCompanyDetail)

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
		api.POST("/protected/product-image-studio/:id/generate-new", handlers.GenerateNewProductImage)
		api.POST("/protected/product-image-studio/create-image", handlers.GenerateCreationModeImage)
		api.GET("/protected/product-image-studio/jobs/:jobID", handlers.GetProductImageStudioJob)
		api.POST("/protected/product-image-studio/variants/:variantID/select", handlers.SelectProductImageVariant)
		api.POST("/protected/product-image-studio/ad-variants/:variantID/select", handlers.SelectProductImageAdVariant)

		// Studio Brief Kütüphanesi
		api.GET("/protected/studio-briefs", handlers.ListStudioBriefs)
		api.POST("/protected/studio-briefs", handlers.CreateStudioBrief)
		api.PATCH("/protected/studio-briefs/:id", handlers.UpdateStudioBrief)
		api.DELETE("/protected/studio-briefs/:id", handlers.DeleteStudioBrief)
		api.POST("/protected/studio-briefs/:id/use", handlers.UseStudioBrief)

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

			// ── Yeni SEO Dashboard rotaları ──
			seo.GET("/projects", handlers.SeoListProjects)
			seo.POST("/projects/:id/sync", handlers.SeoSyncProject)
			seo.GET("/projects/:id/overview", handlers.SeoProjectOverview)

			// ── Action Engine rotaları ──
			seo.GET("/actions/run-engine", handlers.ActionRunEngine)
			seo.GET("/actions/list", handlers.ActionList)
			seo.PATCH("/actions/:id/status", handlers.ActionUpdateStatus)

			// ── Rankings Engine rotaları ──
			seo.GET("/rankings/sync", handlers.RankingsSync)
			seo.GET("/rankings/list", handlers.RankingsList)
			seo.GET("/rankings/summary", handlers.RankingsSummary)

			// ── Keyword Intelligence ──
			seo.GET("/keywords/dashboard", handlers.KeywordDashboard)

			// ── Content Brief ──
			seo.GET("/content/opportunities", handlers.ContentOpportunities)
			seo.POST("/content/generate-brief", handlers.ContentGenerateBrief)
			seo.GET("/content/briefs", handlers.ContentBriefsList)

			// ── Technical SEO ──
			seo.GET("/technical/dashboard", handlers.TechnicalDashboard)
			seo.GET("/technical/sync", handlers.TechnicalSync)
			seo.GET("/technical/issues/details", handlers.TechnicalIssueDetails)

			// ── Backlink ──
			seo.GET("/backlinks/dashboard", handlers.BacklinkDashboard)
			seo.GET("/backlinks/sync", handlers.BacklinkSync)

			// ── AI Visibility ──
			seo.GET("/ai-visibility/dashboard", handlers.AIVisibilityDashboard)
			seo.GET("/ai-visibility/sync", handlers.AIVisibilitySync)

			// ── Dashboard SEO Summary (seo_metrics_cache) ──
			seo.GET("/dashboard/summary", handlers.SeoDashboardSummary)
			seo.POST("/dashboard/sync-cache", handlers.SeoDashboardSyncCache)
		}

		// ──────────────────────────────────────────
		// Meta Ads rotaları (JWT gerekli)
		// ──────────────────────────────────────────
		meta := api.Group("/meta")
		{
			meta.GET("/auth/login", handlers.MetaOAuthLogin)
			meta.GET("/status", handlers.MetaIntegrationStatus)
			meta.GET("/insights", handlers.MetaAccountInsightsHandler)
			meta.GET("/campaigns", handlers.MetaCampaignsHandler)
			meta.GET("/dashboard", handlers.MetaDashboardHandler)
			meta.DELETE("/disconnect", handlers.MetaDisconnect)
		}

		// ──────────────────────────────────────────
		// Google Ads rotaları (JWT gerekli)
		// ──────────────────────────────────────────
		gads := api.Group("/google-ads")
		{
			gads.GET("/auth/login", handlers.GoogleAdsLogin)
			gads.GET("/status", handlers.GoogleAdsIntegrationStatus)
			gads.GET("/accounts", handlers.GoogleAdsListAccounts)
			gads.POST("/select-account", handlers.GoogleAdsSelectAccount)
			gads.GET("/dashboard", handlers.GoogleAdsDashboardHandler)
			gads.DELETE("/disconnect", handlers.GoogleAdsDisconnect)
		}
	}

	// ═══════════════════════════════════════
	// CRON JOBS
	// ═══════════════════════════════════════
	c := cron.New()
	// Her gece 03:00'te Action Engine çalıştır
	_, _ = c.AddFunc("0 3 * * *", func() {
		log.Println("🕐 Cron: Action Engine başlıyor (03:00)")
		services.RunActionEngineForAllProjects()
	})
	// Her gece 03:30'da GSC Metrics Cache güncelle
	_, _ = c.AddFunc("30 3 * * *", func() {
		log.Println("🕐 Cron: GSC Metrics Cache sync başlıyor (03:30)")
		services.SyncAllCompaniesGSCMetrics()
	})
	c.Start()
	log.Println("⏰ Cron job'lar ayarlandı: Action Engine 03:00, GSC Metrics Cache 03:30")

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