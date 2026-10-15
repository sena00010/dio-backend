package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
	"github.com/kullaniciadiniz/dio-panel-backend/utils"
)



// ──────────────────────────────────────────────
// Request / Response yapıları
// ──────────────────────────────────────────────

type GenerateIdeasRequest struct {
	CompanyID    uint   `json:"company_id" binding:"required"`
	Topic        string `json:"topic" binding:"required"`
	Platform     string `json:"platform" binding:"required"`
	Count        int    `json:"count" binding:"required,min=1,max=10"`
	CampaignType string `json:"campaign_type" binding:"required"`
}

type UpdateIdeaRequest struct {
	ID          uint    `json:"id" binding:"required"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Hashtags    *string `json:"hashtags"`
}

type RegenerateIdeaRequest struct {
	ID uint `json:"id" binding:"required"`
}

type ApproveIdeaRequest struct {
	ID uint `json:"id" binding:"required"`
}

// ──────────────────────────────────────────────
// POST /api/protected/generate-ideas
// ──────────────────────────────────────────────

func GenerateIdeas(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req GenerateIdeasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Kullanıcının bu şirkete erişimi var mı kontrol et
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", req.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu şirkete erişim yetkiniz yok"})
		return
	}

	// Marka bağlamını oluştur
	companyContext := buildCompanyContext(company)

	// AI'dan fikir üret
	ideas, err := utils.GenerateIdeas(req.Topic, req.Platform, req.CampaignType, req.Count, companyContext)
	if err != nil {
		log.Printf("❌ AI fikir üretme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI fikir üretme başarısız: " + err.Error()})
		return
	}

	// Veritabanına kaydet
	var savedIdeas []models.AiIdea
	for _, idea := range ideas {
		// JSON serialize
		contentItemsJSON, _ := json.Marshal(idea.ContentItems)
		// Görseller yazısız üretilir; başlık/CTA stüdyoda bu katmanlarla marka fontuyla basılır
		designLayersJSON, _ := json.Marshal(idea.DesignLayers)
		if len(idea.DesignLayers) == 0 {
			designLayersJSON = []byte("[]")
		}

		aiIdea := models.AiIdea{
			CompanyID:             req.CompanyID,
			UserInputTopic:        req.Topic,
			Platform:              req.Platform,
			CampaignType:          req.CampaignType,
			GeneratedTitle:        idea.Title,
			GeneratedDescription:  idea.Description,
			GeneratedHashtags:     idea.Hashtags,
			ImageSuggestionPrompt: idea.ImageSuggestionPrompt,
			SuggestedLayout:       idea.SuggestedLayout + "|" + idea.LogoPosition,
			SuggestedFont:         idea.SuggestedFont,
			ContentItems:          string(contentItemsJSON),
			DesignLayers:          string(designLayersJSON),
			IsSelected:            false,
		}

		if err := database.DB.Create(&aiIdea).Error; err != nil {
			log.Printf("❌ AI fikir kaydetme hatası: %v", err)
			continue
		}
		savedIdeas = append(savedIdeas, aiIdea)
	}

	log.Printf("✅ %d AI fikir üretildi ve kaydedildi (company_id: %d, topic: %s)", len(savedIdeas), req.CompanyID, req.Topic)

	c.JSON(http.StatusCreated, gin.H{
		"message": fmt.Sprintf("%d fikir başarıyla üretildi", len(savedIdeas)),
		"ideas":   savedIdeas,
	})
}

// ──────────────────────────────────────────────
// PATCH /api/protected/update-idea
// ──────────────────────────────────────────────

func UpdateIdea(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req UpdateIdeaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fikri bul
	var idea models.AiIdea
	if err := database.DB.First(&idea, req.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fikir bulunamadı"})
		return
	}

	// Şirket sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", idea.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu fikre erişim yetkiniz yok"})
		return
	}

	// Partial update
	if req.Title != nil {
		idea.GeneratedTitle = *req.Title
	}
	if req.Description != nil {
		idea.GeneratedDescription = *req.Description
	}
	if req.Hashtags != nil {
		idea.GeneratedHashtags = *req.Hashtags
	}

	if err := database.DB.Save(&idea).Error; err != nil {
		log.Printf("❌ Fikir güncelleme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Fikir güncellenemedi"})
		return
	}

	log.Printf("✅ Fikir güncellendi: ID=%d", idea.ID)
	c.JSON(http.StatusOK, gin.H{
		"message": "Fikir başarıyla güncellendi",
		"idea":    idea,
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/regenerate-idea
// ──────────────────────────────────────────────

func RegenerateIdea(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req RegenerateIdeaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Mevcut fikri bul
	var idea models.AiIdea
	if err := database.DB.First(&idea, req.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fikir bulunamadı"})
		return
	}

	// Şirket sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", idea.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu fikre erişim yetkiniz yok"})
		return
	}

	// Marka bağlamını oluştur
	companyContext := buildCompanyContext(company)

	// AI'dan 1 yeni varyasyon üret
	newIdeas, err := utils.GenerateIdeas(idea.UserInputTopic, idea.Platform, idea.CampaignType, 1, companyContext)
	if err != nil {
		log.Printf("❌ AI yeniden üretme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Yeniden üretme başarısız: " + err.Error()})
		return
	}

	if len(newIdeas) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "AI boş yanıt döndü"})
		return
	}

	// JSON serialize
	contentItemsJSON, _ := json.Marshal(newIdeas[0].ContentItems)
	designLayersJSON, _ := json.Marshal(newIdeas[0].DesignLayers)

	// Mevcut kaydı güncelle
	idea.GeneratedTitle = newIdeas[0].Title
	idea.GeneratedDescription = newIdeas[0].Description
	idea.GeneratedHashtags = newIdeas[0].Hashtags
	idea.ImageSuggestionPrompt = newIdeas[0].ImageSuggestionPrompt
	idea.SuggestedLayout = newIdeas[0].SuggestedLayout
	idea.SuggestedFont = newIdeas[0].SuggestedFont
	idea.ContentItems = string(contentItemsJSON)
	idea.DesignLayers = string(designLayersJSON)

	if err := database.DB.Save(&idea).Error; err != nil {
		log.Printf("❌ Fikir güncelleme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Fikir güncellenemedi"})
		return
	}

	log.Printf("✅ Fikir yeniden üretildi: ID=%d", idea.ID)
	c.JSON(http.StatusOK, gin.H{
		"message": "Fikir başarıyla yeniden üretildi",
		"idea":    idea,
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/approve-idea
// ──────────────────────────────────────────────

func ApproveIdea(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req ApproveIdeaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fikri bul
	var idea models.AiIdea
	if err := database.DB.First(&idea, req.ID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Fikir bulunamadı"})
		return
	}

	// Şirket sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", idea.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu fikre erişim yetkiniz yok"})
		return
	}

	// Posts tablosuna yeni kayıt oluştur — layout ve content items dahil
	post := models.Post{
		CompanyID:       idea.CompanyID,
		Title:           idea.GeneratedTitle,
		Content:         idea.GeneratedDescription,
		Platform:        idea.Platform,
		Hashtags:        idea.GeneratedHashtags,
		ImagePrompt:     idea.ImageSuggestionPrompt,
		SuggestedLayout: idea.SuggestedLayout,
		SuggestedFont:   idea.SuggestedFont,
		ContentItems:    idea.ContentItems,
		DesignLayers:    idea.DesignLayers,
		Status:          "draft",
	}

	if err := database.DB.Create(&post).Error; err != nil {
		log.Printf("❌ Post oluşturma hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Post oluşturulamadı: " + err.Error()})
		return
	}

	// AI fikrinde is_selected işaretle
	idea.IsSelected = true
	database.DB.Save(&idea)

	log.Printf("✅ Fikir onaylandı ve post oluşturuldu: idea_id=%d → post_id=%d", idea.ID, post.ID)
	c.JSON(http.StatusCreated, gin.H{
		"message": "Fikir onaylandı ve post olarak kaydedildi",
		"post":    post,
	})
}

// ──────────────────────────────────────────────
// GET /api/protected/posts/:id
// ──────────────────────────────────────────────

func GetPost(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	postID := c.Param("id")

	var post models.Post
	if err := database.DB.First(&post, postID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post bulunamadı"})
		return
	}

	// Şirket sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", post.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu posta erişim yetkiniz yok"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"post":    post,
		"company": company,
	})
}

// ──────────────────────────────────────────────
// POST /api/protected/generate-image
// (Artık Dinamik Template Layout kullanır)
// ──────────────────────────────────────────────

type GenerateImageRequest struct {
	PostID       uint   `json:"post_id" binding:"required"`
	Style        string `json:"style"`
	CustomPrompt string `json:"custom_prompt"` // Kullanıcının özel prompt'u (opsiyonel)
	TextPosition string `json:"text_position"` // Kullanıcının seçtiği metin pozisyonu (opsiyonel)
}

func GeneratePostImage(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Kullanıcı kimliği bulunamadı"})
		return
	}
	userID := userIDVal.(uint)

	var req GenerateImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Varsayılan stil
	if req.Style == "" {
		req.Style = "realistic"
	}

	// Postu bul
	var post models.Post
	if err := database.DB.First(&post, req.PostID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Post bulunamadı"})
		return
	}

	// Şirket sahiplik kontrolü
	var company models.Company
	if err := database.DB.Where("id = ? AND user_id = ?", post.CompanyID, userID).First(&company).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "Bu posta erişim yetkiniz yok"})
		return
	}

	// Prompt: kullanıcının özel prompt'u varsa onu kullan, yoksa AI prompt'u
	imagePrompt := post.ImagePrompt
	if req.CustomPrompt != "" {
		imagePrompt = req.CustomPrompt
		log.Printf("📝 Kullanıcı özel prompt kullandı: %s", req.CustomPrompt)
	}

	if imagePrompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Görsel prompt'u bulunamadı"})
		return
	}

	// Content items'ı JSON'dan parse et
	var contentItems []utils.ContentItem
	if post.ContentItems != "" {
		if err := json.Unmarshal([]byte(post.ContentItems), &contentItems); err != nil {
			log.Printf("⚠️ ContentItems parse hatası, boş devam ediliyor: %v", err)
		}
	}

	// Layout: kullanıcı seçtiyse onu kullan, yoksa AI'ın önerisini al
	layout := req.TextPosition
	if layout == "" {
		layout = post.SuggestedLayout
	}
	if layout == "" {
		layout = "bottom_left"
	}

	// Tam marka bağlamı ile profesyonel görsel üret
	brandCtx := utils.BrandContext{
		// Company (marka kimliği)
		BrandName:      company.Name,
		Sector:         company.Sector,
		TargetAudience: company.TargetAudience,
		BrandColors:    company.BrandColors,
		ToneOfVoice:    company.ToneOfVoice,
		LogoURL:        company.LogoURL,
		Slogan:         company.Slogan,
		Description:    company.Description,
		BrandFont:      company.BrandFont,
		Website:        company.Website,
		// Post (içerik)
		Platform:     post.Platform,
		Headline:     post.Title,
		SubText:      post.Content,
		ImagePrompt:  imagePrompt,
		ContentItems: contentItems,
		// Kullanıcı seçimleri
		Style:        req.Style,
		TextPosition: layout,
	}

	localURL, err := utils.GenerateBrandImage(brandCtx)

	if err != nil {
		log.Printf("❌ Görsel üretme hatası (post_id: %d, stil: %s): %v", post.ID, req.Style, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Görsel üretme başarısız: " + err.Error()})
		return
	}

	// Görsel yazısız üretildiği için fikirden gelen tasarım katmanları korunur
	if post.DesignLayers == "" {
		post.DesignLayers = "[]"
	}

	// ── AI Vision ile optimal logo pozisyonunu analiz et ──
	logoPos, logoErr := utils.AnalyzeImageForLogoPosition(localURL, company.Name)
	if logoErr != nil {
		log.Printf("⚠️ Logo pozisyon analizi başarısız (varsayılan kullanılacak): %v", logoErr)
	} else {
		logoPosJSON, _ := json.Marshal(logoPos)
		post.LogoPosition = string(logoPosJSON)
		log.Printf("🎯 Logo pozisyonu belirlendi: x=%.1f%%, y=%.1f%% — %s", logoPos.X, logoPos.Y, logoPos.Reasoning)
	}

	// URL'i ve katmanları veritabanına kaydet
	post.ImageURL = localURL
	if err := database.DB.Save(&post).Error; err != nil {
		log.Printf("❌ Post kaydetme hatası: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Post kaydedilemedi"})
		return
	}

	log.Printf("✅ Template görsel üretildi: post_id=%d, stil=%s, layout=%s → %s", post.ID, req.Style, layout, localURL)
	c.JSON(http.StatusOK, gin.H{
		"message":       "Görsel başarıyla üretildi",
		"image_url":     localURL,
		"post":          post,
		"logo_position": logoPos,
	})
}



// ──────────────────────────────────────────────
// Yardımcı: Marka bağlamını oluştur
// ──────────────────────────────────────────────

func buildCompanyContext(company models.Company) string {
	ctx := fmt.Sprintf("Brand Name: %s", company.Name)

	if company.Sector != "" {
		ctx += fmt.Sprintf(", Sector/Industry: %s", company.Sector)
	}
	if company.ContentLanguage != "" {
		ctx += fmt.Sprintf(", Content Language: %s", company.ContentLanguage)
	}
	if company.TargetAudience != "" {
		ctx += fmt.Sprintf(", Target Audience: %s", company.TargetAudience)
	}
	if company.Description != "" {
		ctx += fmt.Sprintf(", Brand Description: %s", company.Description)
	}
	if company.Slogan != "" {
		ctx += fmt.Sprintf(", Slogan: %s", company.Slogan)
	}
	if company.Goal != "" {
		ctx += fmt.Sprintf(", Business Goal: %s", company.Goal)
	}
	if company.ProductCategories != "" {
		ctx += fmt.Sprintf(", Product/Service Categories: %s", company.ProductCategories)
	}
	if company.Platforms != "" {
		ctx += fmt.Sprintf(", Active Platforms: %s", company.Platforms)
	}
	if company.Website != "" {
		ctx += fmt.Sprintf(", Website: %s", company.Website)
	}
	if company.BrandType != "" {
		ctx += fmt.Sprintf(", Brand Type: %s", company.BrandType)
	}
	if company.BrandFont != "" {
		ctx += fmt.Sprintf(", Primary Font: %s", company.BrandFont)
	}

	// ── Sosyal Medya İletişimi ──
	if company.SocialMedia != "" {
		ctx += fmt.Sprintf("\n\n[SOCIAL MEDIA CHANNELS]: %s. Integrate a call-to-action or subtle references to these specific social media usernames organically within the description/caption.", company.SocialMedia)
	}

	// ── Marka renkleri — AI tasarım katmanları için kritik ──
	if company.BrandColors != "" {
		ctx += fmt.Sprintf("\n\n[BRAND DESIGN SYSTEM — COLORS]: The brand's official color palette is: %s. When creating design_layers, you MUST use these brand colors for text, badges, and CTA backgrounds. The headline and CTA elements should feature the brand's primary color. Use white (#ffffff) or very light colors for text on dark backgrounds. Use the brand colors for badge bg_color values. This creates visual brand consistency.\n\nIMPORTANT FOR IMAGE GENERATION: The 'image_suggestion_prompt' MUST EXPLICITLY state to include elements, lighting, themes, or background colors that match the brand palette.", company.BrandColors)
	}

	// ── Ton/ses — tüm içerik bu tonda yazılmalı ──
	if company.ToneOfVoice != "" {
		ctx += fmt.Sprintf("\n\n[SYSTEM INSTRUCTION — TONE OF VOICE]: You MUST write ALL text content (title, description, hashtags, design_layers text) using a '%s' tone. Every sentence must reflect this tone consistently. This is non-negotiable.", company.ToneOfVoice)
	}

	// ── Yasaklı kelimeler ──
	if company.ForbiddenWords != "" {
		ctx += fmt.Sprintf("\n\n[SYSTEM INSTRUCTION — FORBIDDEN WORDS]: The following words/phrases are STRICTLY BANNED. NEVER use any of these in any field: %s. If you accidentally include any of these, the output will be rejected.", company.ForbiddenWords)
	}

	ctx += captionQualityRules

	return ctx
}

// captionQualityRules — gerçek sosyal medya kalitesinde caption ve tasarım katmanı kuralları.
// DB'deki promptun uzunluk kurallarını geçersiz kılar.
const captionQualityRules = `

[SYSTEM INSTRUCTION — CAPTION QUALITY (overrides any earlier length rule)]:
- "description" is a ready-to-post caption written like a top social media manager:
  1) First line is a scroll-stopping HOOK (max 12 words): a question, bold claim, number or pain point.
  2) Then 2-4 short value paragraphs (1-2 sentences each), separated by a blank line ("\n\n"). Concrete benefits, no filler.
  3) End with ONE clear call-to-action (e.g. "Detaylar için DM'den yaz", "Linke tıkla", "Kaydet, sonra lazım olacak").
  4) Max 2-3 relevant emojis in total. No hashtags inside the description.
- "hashtags": 6-10 hashtags, mixing niche and mid-size ones relevant to the brand, sector and location; no generic ones like #love #instagood.
- Write in correct, natural Turkish with proper characters (ç, ğ, ı, İ, ö, ş, ü) unless another content language is specified. Never ASCII-fy Turkish words.
- Ideas in one batch must use different angles (education, social proof, behind the scenes, engagement question, offer); at most one direct sales idea.

[SYSTEM INSTRUCTION — DESIGN LAYERS]:
- The image is generated WITHOUT any text; design_layers are the ONLY text on the visual, so they must be strong.
- Always include a "headline" layer (max 6 words, punchy) and a "cta" layer (max 4 words, e.g. "Hemen Keşfet", "DM'den Yaz"). A short "description" layer is optional.
- Keep all layers inside the safe area: x between 6 and 94, y between 8 and 82 (bottom area is reserved for the logo).`
