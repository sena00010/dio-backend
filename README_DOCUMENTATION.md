# Dio Panel Backend — Teknik Dökümantasyon (A-Z)

Bu döküman, Dio Panel Backend projesinin mimarisini, veritabanı yapısını, API uç noktalarını (endpoints) ve iş mantığını (logic) detaylandırmak amacıyla hazırlanmıştır.

---

## 1. Mimari Genel Bakış

Dio Panel, işletmelerin dijital pazarlama, içerik üretimi ve SEO süreçlerini AI destekli araçlarla yönetmesini sağlayan bir platformdur.

- **Dil & Framework**: Go (Golang) - Gin Web Framework
- **Veritabanı**: PostgreSQL (GORM ORM kullanılarak yönetilir)
- **Kimlik Doğrulama**: Firebase Admin SDK (Google Login için) + Özel JWT (JSON Web Token)
- **AI Entegrasyonları**: Gemini (Google), OpenAI/Claude (Prompt yönetimi üzerinden), Pollinations/HuggingFace (Görsel üretimi)
- **Zamanlanmış Görevler**: `robfig/cron` (SEO veri senkronizasyonu ve Action Engine için)

---

## 2. Veritabanı Tablo Yapısı (Models)

Sistemdeki temel tablolar ve görevleri aşağıdadır:

### 2.1. Kullanıcı ve Şirket Yönetimi
- **`users`**: Platform kullanıcılarını tutar. `firebase_uid` ile Google login desteği sunar.
- **`companies`**: Kullanıcının sahip olduğu/yönettiği markayı temsil eder. Sektör, hedef kitle, marka renkleri, logo ve ses tonu (tone of voice) gibi kritik bilgileri tutar.
- **`team_members`**: Bir şirkete bağlı olan ekip üyelerini ve rollerini yönetir.
- **`company_competitors`**: Şirketin rakip analizleri için eklenen rakip marka bilgilerini tutar.

### 2.2. İçerik Üretimi (AI & Social Media)
- **`ai_ideas`**: AI tarafından üretilen ham içerik fikirleri. Başlık, açıklama ve görsel önerilerini içerir.
- **`posts`**: Onaylanmış içerik fikirlerinin veya manuel oluşturulan postların taslak/yayında hallerini tutar. Görsel URL'leri ve tasarım katmanlarını (layout) saklar.
- **`ai_schedules`**: İçeriklerin ne zaman paylaşılacağını tutan takvim verileri.
- **`product_image_studios`**: Ürün fotoğraflarının AI ile profesyonel hale getirilmesi sürecindeki projeleri tutar.

### 2.3. SEO ve Analiz
- **`seo_projects`**: Google Search Console (GSC) ile bağlanan her bir web sitesini temsil eder.
- **`seo_metrics_cache`**: GSC'den çekilen tıklama, gösterim, CTR gibi metriklerin performans için önbelleğe alınmış halleri.
- **`seo_action_tasks`**: "Action Engine" tarafından üretilen SEO iyileştirme önerileri.
- **`seo_tracked_keywords`**: İzlenen anahtar kelimelerin sıralama verileri.
- **`seo_technical_health`**: Web sitesinin teknik sağlık skorları ve hataları.

### 2.4. Reklam Hesapları
- **`meta_accounts`**, **`google_ads_accounts`**, **`tiktok_accounts`**, **`linkedin_accounts`**: Platformlarla yapılan OAuth entegrasyonları sonrası çekilen reklam hesabı ve dashboard verileri.

---

## 3. API Endpoints ve Handlers

Rotalar `main.go` içerisinde tanımlanmıştır. `handlers/` dizinindeki fonksiyonlar bu rotaları işler.

### 3.1. Auth (Kimlik Doğrulama)
- `POST /api/auth/register`: Yeni kullanıcı ve şirket kaydı.
- `POST /api/auth/login`: E-posta/Şifre ile giriş.
- `POST /api/auth/google`: Firebase ID Token ile Google üzerinden giriş/kayıt.
- `GET /api/me`: Mevcut kullanıcının ve bağlı şirketinin bilgilerini döner.

### 3.2. Company & Onboarding (Marka Yapılandırma)
- `POST /api/companies/onboarding`: Marka kurulum adımlarını yönetir.
- `POST /api/onboarding/research`: Verilen web sitesini analiz ederek marka bilgilerini otomatik doldurur.
- `POST /api/protected/suggest-competitors`: AI ile sektöre göre rakip önerileri sunar.
- `GET /api/protected/companies/:id/detail`: Şirketin tüm detaylarını ve istatistiklerini getirir.

### 3.3. AI Content Engine (İçerik Üretimi)
- `POST /api/protected/generate-ideas`: Marka kimliğine uygun içerik fikirleri üretir (`ai_ideas` tablosuna yazar).
- `POST /api/protected/approve-idea`: Bir AI fikrini onaylar ve bunu bir `post` kaydına dönüştürür.
- `POST /api/protected/generate-image`: Bir post için AI ile görsel (image) üretir.
- `POST /api/protected/upload-canvas`: Tasarım stüdyosundan gelen görseli kaydeder.

### 3.4. SEO Dashboard & Tools
- `GET /api/seo/dashboard/metrics`: Genel SEO performans özetini döner.
- `GET /api/seo/actions/run-engine`: Manuel olarak SEO öneri motorunu çalıştırır.
- `GET /api/seo/rankings/list`: Anahtar kelime sıralama geçmişini listeler.
- `GET /api/seo/technical/dashboard`: Site sağlığı ve teknik hataları gösterir.

### 3.5. Ads Integration (Reklam Yönetimi)
Her platform (Meta, Google, TikTok, LinkedIn) için benzer yapıdadır:
- `/auth/login`: Platformun OAuth sayfasına yönlendirir.
- `/status`: Entegrasyonun aktif olup olmadığını kontrol eder.
- `/dashboard`: Harcama, tıklama gibi reklam performans verilerini getirir.

---

## 4. Servisler (Services)

İş mantığının ağır kısımları `services/` dizininde toplanmıştır:

1.  **`action_engine.go`**: Web sitesi verilerini analiz ederek "Başlık çok uzun", "Tıklama düşüyor" gibi otomatik aksiyonlar/öneriler üretir.
2.  **`gsc_service.go`**: Google Search Console API ile haberleşerek ham verileri çeker.
3.  **`technical_engine.go`**: Web sitelerini tarayarak (crawling simülasyonu veya API ile) teknik SEO açıklarını bulur.
4.  **`ai_visibility_engine.go`**: Web sitesinin yapay zeka cevaplarında (SGE, Perplexity vb.) ne kadar görünür olduğunu analiz eder.
5.  **`google_ads_service.go`**: Google Ads verilerini çekmek için gerekli API çağrılarını yönetir.

---

## 5. Utils (Yardımcı Araçlar)

- **`jwt.go`**: Token üretimi ve doğrulama.
- **`firebase.go`**: Google Login için Firebase token doğrulaması.
- **`product_image_studio.go`**: Görsel işleme ve AI image generation prompt yönetimi.
- **`prompt_manager.go`**: AI modellerine gönderilen prompt'ların merkezi yönetim sistemi.

---

## 6. Cron Jobs (Zamanlanmış Görevler)

Sistemde her gece çalışan otomatik görevler bulunmaktadır:
- **03:00**: Action Engine çalıştırılır (Tüm projeler için yeni öneriler üretilir).
- **03:30**: GSC Metrics Cache senkronizasyonu yapılır (Dünün verileri veritabanına işlenir).

---

## 7. Admin Panel Kontrolleri

Admin yetkisine sahip kullanıcılar `/api/admin` altındaki uç noktalar ile:
- **Prompt'ları** güncelleyebilir (AI'nın nasıl cevap vereceğini belirleyen şablonlar).
- **AI Modellerini** yönetebilir (Hangi özelliğin hangi modeli -Gemini 1.5, GPT-4 vb.- kullanacağını ayarlar).
- Sistemdeki tüm kayıtları izleyebilir.

---

Bu döküman, Dio Panel Backend sisteminin genel işleyişini özetlemektedir. Her bir handler dosyası kendi içinde hata yönetimini ve veri validasyonunu (binding) yapmaktadır.
