package utils

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// ──────────────────────────────────────────────
// Bunny CDN Storage Upload Utility
// Tüm görseller artık Bunny Storage'a yüklenir
// ve Pull Zone CDN URL'i ile erişilir.
// ──────────────────────────────────────────────

// BunnyCDN yapılandırma bilgileri (.env'den okunur)
func getBunnyStorageName() string {
	return os.Getenv("BUNNY_STORAGE_NAME")
}

func getBunnyAPIKey() string {
	return os.Getenv("BUNNY_API_KEY")
}

func getBunnyPullZone() string {
	zone := os.Getenv("BUNNY_PULL_ZONE")
	return strings.TrimRight(zone, "/")
}

// IsBunnyCDNConfigured — Bunny CDN yapılandırması var mı kontrol eder
func IsBunnyCDNConfigured() bool {
	return getBunnyStorageName() != "" && getBunnyAPIKey() != "" && getBunnyPullZone() != ""
}

// UploadToBunnyCDN — Byte verisini Bunny CDN Storage'a yükler ve CDN URL döner.
// folder: "logos", "posts", "product-studio" gibi alt klasör.
// fileName: benzersiz dosya adı (örn: "ai_1234567890.png")
// Dönen URL: https://dio-cdn.b-cdn.net/logos/ai_1234567890.png
func UploadToBunnyCDN(data []byte, folder, fileName string) (string, error) {
	storageName := getBunnyStorageName()
	apiKey := getBunnyAPIKey()
	pullZone := getBunnyPullZone()

	if storageName == "" || apiKey == "" || pullZone == "" {
		return "", fmt.Errorf("Bunny CDN yapılandırması eksik (BUNNY_STORAGE_NAME, BUNNY_API_KEY, BUNNY_PULL_ZONE)")
	}

	// Upload URL: https://storage.bunnycdn.com/{storageZoneName}/{folder}/{fileName}
	uploadURL := fmt.Sprintf("https://storage.bunnycdn.com/%s/%s/%s", storageName, folder, fileName)

	req, err := http.NewRequest("PUT", uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("Bunny CDN request oluşturulamadı: %v", err)
	}

	req.Header.Set("AccessKey", apiKey)
	req.Header.Set("Content-Type", "application/octet-stream")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Bunny CDN upload hatası: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Bunny CDN upload hatası %d: %s", resp.StatusCode, string(body))
	}

	// CDN URL: https://dio-cdn.b-cdn.net/{folder}/{fileName}
	cdnURL := fmt.Sprintf("%s/%s/%s", pullZone, folder, fileName)
	log.Printf("✅ Bunny CDN'e yüklendi: %s", cdnURL)
	return cdnURL, nil
}

// UploadFileToBunnyCDN — Multipart dosyayı okuyup Bunny CDN'e yükler
// Gin handler'lardan gelen dosya byte'larını alır ve yükler.
func UploadFileToBunnyCDN(fileData []byte, folder, fileName string) (string, error) {
	return UploadToBunnyCDN(fileData, folder, fileName)
}

// DownloadAndUploadToBunnyCDN — Harici URL'den görseli indirip Bunny CDN'e yükler
func DownloadAndUploadToBunnyCDN(imageURL, folder, fileName string) (string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("görsel indirme hatası: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("görsel indirme hata kodu: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("görsel okuma hatası: %v", err)
	}

	return UploadToBunnyCDN(data, folder, fileName)
}
