package handlers

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kullaniciadiniz/dio-panel-backend/database"
	"github.com/kullaniciadiniz/dio-panel-backend/models"
)

// POST /api/admin/prompts/seed — Mevcut Go kodundaki promptları veritabanına yükle
// Bu endpoint sadece ilk kurulumda 1 kez çağrılır
func AdminSeedPrompts(c *gin.Context) {
	// Temizleme parametresi var mı (yeniden kurulum için)
	force := c.Query("force") == "true"

	if force {
		database.DB.Exec("DELETE FROM system_prompts")
		log.Println("🗑️ Eski promptlar temizlendi (force=true)")
	} else {
		// Zaten prompt var mı kontrol
		var count int64
		database.DB.Model(&models.SystemPrompt{}).Count(&count)
		if count > 0 {
			c.JSON(http.StatusOK, gin.H{
				"message": "Promptlar zaten yüklenmiş. Zorla baştan kurmak için /seed?force=true kullanın.",
				"count":   count,
			})
			return
		}
	}

	seeds := getDefaultPromptSeeds()

	created := 0
	for _, seed := range seeds {
		if err := database.DB.Create(&seed).Error; err != nil {
			log.Printf("⚠️ Seed prompt oluşturulamadı (%s): %v", seed.PromptKey, err)
			continue
		}
		created++
	}

	log.Printf("✅ %d/%d prompt seed olarak yüklendi (Template Engine formatında)", created, len(seeds))
	c.JSON(http.StatusCreated, gin.H{
		"message": "Promptlar başarıyla yüklendi",
		"created": created,
		"total":   len(seeds),
	})
}

func getDefaultPromptSeeds() []models.SystemPrompt {
	return []models.SystemPrompt{
		// ────────────────────────────────────
		// Kategori: Ürün Görsel Stüdyosu
		// ────────────────────────────────────
		{
			PromptKey:   "product_studio_main",
			Category:    "Ürün Görsel Stüdyosu",
			Title:       "Ana Ürün Görseli Prompt",
			Description: "Ürün fotoğrafı yüklendikten sonra e-ticaret platformları için profesyonel görsel üretiminde kullanılan ana prompt.",
			Content: `Sen {{.companyName}} icin calisan kidemli bir e-ticaret art direktoru ve sosyal ticaret gorsel stratejistisin.

AMAC:
- Yuklenen gorseldeki urunun kendisini kullan.
- Urunun siluetini, oranlarini, ambalaj kimligini, on yuzunu ve taniyabilir urun tipini koru.
- Ayni urunu "{{.presetPlatform}}" icin daha premium, daha profesyonel ve paylasilabilir bir platform gorseline donustur.
- Daginik arka plani temizle, ama yeni bir urun tasarlama.

TEMEL TALIMAT:
- Kaynak gercek sadece yuklenen gorseldir.
- Bu urunu kullanarak harika bir tasarimla revize et.
- Sonuc Hepsiburada, Trendyol ve sosyal medya hesaplarinda paylasilabilecek kadar guclu olmali.
- Asla farkli bir urun, farkli bir paket, farkli bir hayvan mamasi ya da farkli bir kategori uretme.
- Asla DIO ekleme, ekstra logo ekleme, yeni watermark ekleme.

KAYNAK URUN OZETI:
{{.referenceSummary}}

MARKA / SATICI BAGLAMI:
- Marka: {{.companyName}}
- Sektor: {{.companySector}}
- Kategori: {{.categoryHint}}
- Hedef kitle: {{.companyTargetAudience}}
- Iletisim tonu: {{.companyToneOfVoice}}
- Marka renkleri: {{.brandColors}}

CIKTI OZELLIKLERI:
- Varyant adi: {{.presetName}}
- Platform: {{.presetPlatform}}
- Tuval: {{.presetWidth}}x{{.presetHeight}} px
- Arka plan yonu: {{.presetBackground}}
- Tasarim notu: {{.presetDesignNotes}}
- Kaynak odakli yaratici yon: {{.inferSourceCreativeDirection}}
- Platform kurali: {{.inferPlatformCreativeRules}}
- Referans estetik: {{.inferVisualReferenceStyle}}

ZORUNLU KURALLAR:
- Bu bir yeni urun uretimi degil, yuklenen urunun gorsel revizesidir.
- Urun her zaman ana kahraman olsun.
- Urunun gorunen marka alani, paket yapisi, urun tipi ve genel kimligi korunmali.
- DIO, baska logo, kampanya damgasi, rozet, watermark, UI, el, sticker veya alakasiz nesne ekleme.
- Gorseli daha iyi isik, daha temiz kompozisyon, daha iyi golge, daha premium doku ve daha profesyonel sahneleme ile guclendir.

Sadece gorsel uretime yonelik bir cevap dusun.`,
			Variables: `["companyName","presetPlatform","referenceSummary","companySector","categoryHint","companyTargetAudience","companyToneOfVoice","brandColors","presetName","presetWidth","presetHeight","presetBackground","presetDesignNotes","inferSourceCreativeDirection","inferPlatformCreativeRules","inferVisualReferenceStyle"]`,
			AiProvider: "openai",
			AiModel:    "gpt-4o-mini",
			IsActive:  true,
		},
		{
			PromptKey:   "product_studio_edit",
			Category:    "Ürün Görsel Stüdyosu",
			Title:       "Ürün Görseli Edit Prompt",
			Description: "OpenAI image edit API için kullanılan prompt.",
			Content: `Bu gorseldeki urunu kullanarak {{.presetPlatform}} platformu icin profesyonel bir e-ticaret / pazarlama gorseli olustur.

URUN: {{.productHint}}
Urunu oldugu gibi koru — ayni paket, ayni renkler, ayni marka yazilari, ayni form. Urunu yeniden cizme veya degistirme.

GOREV:
Bu urun gorselini, tum buyuk e-ticaret platformlarinda paylasima hazir, profesyonel bir tanitim gorselina donustur.

{{.sceneDirective}}

TASARIM YONERGELERI:
- Urunu sahnenin ana kahramani olarak konumla, one cikar
- {{.categoryCreativeHint}}
- Urunle uyumlu, kategoriye ozgu dekoratif ve destekleyici ogeler ekle
- Profesyonel studyo isigi, yumusak golgelendirme ve derinlik hissi kullan

CIKTI: {{.presetWidth}}x{{.presetHeight}} piksel, profesyonel e-ticaret kalitesinde pazarlama gorseli`,
			Variables: `["presetPlatform","productHint","sceneDirective","categoryCreativeHint","presetWidth","presetHeight"]`,
			AiProvider: "openai",
			AiModel:    "gpt-4o-mini",
			IsActive:  true,
		},
		{
			PromptKey:   "product_image_reference_analyzer",
			Category:    "Ürün Görsel Stüdyosu",
			Title:       "Referans Görsel Analizi Prompt",
			Description: "Yüklenen ürün fotoğrafını analiz edip detaylı bir özet çıkaran prompt.",
			Content: `Yuklenen urun fotografini bir gorsel uretim sistemi icin analiz et.

Kategori ipucu: {{.categoryHint}}

Amac:
- Gorseldeki urunu sadik sekilde tarif et ki ayni urun yeni sahnelerde korunabilsin.
- Kategori ipucu ile gorsel celisirse gorseli esas al.
- Paketli bir urunse ambalaj kimligini acikca belirt.

Asagidaki noktalari kisaca anlat:
- urun tipi, siluet ve form, gorunen materyal ve doku, baskin renkler, vb.

Yanit sadece Turkce, tek paragraf, 70-140 kelime olsun.`,
			Variables: `["categoryHint"]`,
			AiProvider: "openai",
			AiModel:    "gpt-4o-mini",
			IsActive:  true,
		},

		// ────────────────────────────────────
		// Kategori: Reklam Stüdyosu (Ad Studio)
		// ────────────────────────────────────
		{
			PromptKey:   "ad_studio_main",
			Category:    "Reklam Stüdyosu",
			Title:       "Ana Reklam Kreatifi Prompt",
			Description: "Story, Banner gibi platformlara özel reklam görseli üretiminde kullanılan ana prompt.",
			Content: `Sen yuklenen urun gorselinden platforma hazir sosyal ticaret kreatifi uretecek kidemli bir performans pazarlama art direktorusun.

KAYNAK URUN OZETI:
{{.referenceSummary}}

MARKA BAGLAMI:
- Marka: {{.companyName}}
- Sektor: {{.companySector}}
- Kategori: {{.categoryHint}}
- Hedef kitle: {{.companyTargetAudience}}
- Iletisim tonu: {{.companyToneOfVoice}}
- Renkler: {{.brandColors}}

KREATIF OZELLIKLERI:
- Format: {{.presetFormatLabel}}
- Platform: {{.presetPlatform}}
- Tuval: {{.presetWidth}}x{{.presetHeight}}
- Temiz kalmasi gereken metin alani: {{.presetTextPosition}}
- Metin hizalama beklentisi: {{.presetTextAlign}}
- Desteklenecek baslik: "{{.copyHeadline}}"
- Desteklenecek alt baslik: "{{.copySubHeadline}}"
- CTA tonu: "{{.copyCTA}}"
- Tasarim notu: {{.presetDesignNotes}}
- Yaratici yon: {{.inferAdCreativeDirection}}

ZORUNLU KURALLAR:
- Yuklenen urun kimligi aynen korunmali.
- Bu is yeni urun uretimi degil, yuklenen urunun kampanya revizesidir.
- Daha sonra metin eklenebilmesi icin temiz bir guvenli alan birak.
- Gorselin icine yeni text, logo, label, watermark, UI basma.

Sadece gorsel uretime yonelik dusun.`,
			Variables: `["referenceSummary","companyName","companySector","categoryHint","companyTargetAudience","companyToneOfVoice","brandColors","presetFormatLabel","presetPlatform","presetWidth","presetHeight","presetTextPosition","presetTextAlign","copyHeadline","copySubHeadline","copyCTA","presetDesignNotes","inferAdCreativeDirection"]`,
			AiProvider: "openai",
			AiModel:    "gpt-4o-mini",
			IsActive:  true,
		},
		{
			PromptKey:   "brand_description_generator",
			Category:    "Marka Açıklaması",
			Title:       "Marka Açıklama Üretim Prompt",
			Description: "Marka bilgileri ve website içeriğinden otomatik marka açıklaması üreten prompt.",
			Content: `Sen bir dijital pazarlama ve marka stratejisti olarak çalışıyorsun. Aşağıdaki marka adı ({{.companyName}}) ve websitesini ({{.website}}) analiz ederek şirket hakkında kısa ve net bir şirket özeti oluştur.

Sektör: {{.sector}}

WEBSITE İÇERİĞİ:
{{.siteContent}}

Aşağıdaki bilgileri çıkar ve TEK BİR PARAGRAF halinde yaz:
1. Şirket ne iş yapıyor
2. Hangi hizmetleri sunuyor
3. Hedef müşteri kitlesi
4. Reklamların ulaşması gereken kişiler
5. Şirketin pazarlama diliyle yazılmış kısa bir şirket tanımı

KURALLAR:
- Çıktıyı profesyonel bir pazarlama analizi gibi yaz
- Türkçe olarak yaz
- Kısa ve öz ol — maksimum 4-5 cümle.`,
			Variables: `["companyName","sector","website","siteContent"]`,
			AiProvider: "openai",
			AiModel:    "gpt-4o-mini",
			IsActive:  true,
		},
		{
			PromptKey:   "creative_variant_generator",
			Category:    "Kreatif İçerik Üretimi",
			Title:       "Kreatif Varyant Üretici Prompt",
			Description: "Sosyal medya için farklı açılardan kreatif içerik varyantları üreten ana prompt.",
			Content: `Sen dünyanın en ödüllü kreatif direktörü, dijital pazarlama stratejisti ve sektör analistisin. 30 yıldır Apple, Nike gibi global devlerin kampanyalarını yönetiyorsun.

Marka Bağlamı:
{{.brandContext}}

Lütfen aşağıdaki konuları: {{.topicsStr}}
ve özellikleri: {{.emphasisDesc}}
Odağa alarak, şu platformlar için ({{.platformsStr}}) kreatif varyantlar üret.
Formatlar: {{.formatsStr}}
İstenen varyant sayısı: {{.variantCount}}
Marka Tonu: {{.companyToneOfVoice}}

Sadece JSON çıktısı üret.`,
			Variables: `["brandContext","topicsStr","emphasisDesc","platformsStr","formatsStr","variantCount","companyToneOfVoice"]`,
			AiProvider: "openai",
			AiModel:    "gpt-4o-mini",
			IsActive:  true,
		},
		{
			PromptKey:   "topic_suggestion",
			Category:    "Konu Önerileri",
			Title:       "Konu Önerisi Prompt",
			Description: "Markaya özel sosyal medya konu önerileri üreten prompt.",
			Content: `Sen dünyanın en iyi e-ticaret ve dijital pazarlama uzmanısın. Yüzlerce markayı başarıya taşıdın.

Mevcut Ay: {{.currentMonth}}
Önümüzdeki Özel Günler: {{.upcomingEvents}}

Marka Bağlamı:
{{.brandContext}}

Sadece JSON çıktısı vererek, bu aya özel sosyal medya içerik/konu fikirleri çıkar.`,
			Variables: `["currentMonth","upcomingEvents","brandContext"]`,
			AiProvider: "openai",
			AiModel:    "gpt-4o-mini",
			IsActive:  true,
		},
		{
			PromptKey:   "content_idea_generator",
			Category:    "Konu Önerileri",
			Title:       "AI İçerik Fikri Üretici Prompt",
			Description: "Konu, platform ve kampanya tipine göre sosyal medya içerik fikirleri üreten ana prompt. Title, description, hashtag, görsel önerisi ve tasarım katmanlarını kapsar.",
			Content: `You are a legendary art director with 20 years of experience designing for Vogue, Apple, and Netflix. You create social media layouts where every design choice — font, color, position — is driven by the IMAGE and CONTENT mood.

Generate {{.count}} unique content ideas with:
- title: Short, impactful post title
- description: Full post caption
- hashtags: Relevant hashtags
- image_suggestion_prompt: Detailed English prompt for AI image generation
- suggested_layout: Text position on canvas
- suggested_font: Perfect Google Font for content mood
- logo_position: "auto"
- content_items: Bullet points array

User Prompt: {{.topic}}
Platform: {{.platform}}
Context: {{.companyContext}}
Campaign Type: {{.campaignType}}

Return ONLY valid JSON.`,
			Variables: `["count","topic","platform","campaignType","companyContext"]`,
			AiProvider: "openai",
			AiModel:    "gpt-4o-mini",
			IsActive:  true,
		},
	}
}
