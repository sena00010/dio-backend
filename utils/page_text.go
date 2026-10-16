package utils

import (
	"context"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// FetchPageText — herkese açık bir web sayfasının başlık, açıklama ve görünür metnini döndürür.
// SSRF koruması: yalnızca http(s), özel/yerel/bağlantı-yerel IP'lere bağlanılmaz (yönlendirmelerde de).
func FetchPageText(raw string, maxChars int) (title, text string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", "", fmt.Errorf("geçerli bir http(s) adresi girin")
	}

	dialer := &net.Dialer{Timeout: 8 * time.Second}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !isPublicIP(ip.IP) {
					return nil, fmt.Errorf("bu adrese erişime izin verilmiyor")
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("çok fazla yönlendirme")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("geçersiz yönlendirme")
			}
			return nil
		}}
	req, _ := http.NewRequest("GET", u.String(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; DIO-ContentBot/1.0)")
	req.Header.Set("Accept-Language", "tr-TR,tr;q=0.9,en;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("sayfa açılamadı: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("sayfa açılamadı (HTTP %d)", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	doc := string(body)

	pick := func(re string) string {
		if m := regexp.MustCompile(re).FindStringSubmatch(doc); len(m) > 1 {
			return strings.TrimSpace(html.UnescapeString(m[1]))
		}
		return ""
	}
	title = pick(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["']([^"']+)`)
	if title == "" {
		title = pick(`(?is)<title[^>]*>(.*?)</title>`)
	}
	desc := pick(`(?is)<meta[^>]+name=["']description["'][^>]+content=["']([^"']+)`)
	if desc == "" {
		desc = pick(`(?is)<meta[^>]+property=["']og:description["'][^>]+content=["']([^"']+)`)
	}

	clean := regexp.MustCompile(`(?is)<(script|style|noscript|svg|nav|footer|header|form)[^>]*>.*?</(script|style|noscript|svg|nav|footer|header|form)>`).ReplaceAllString(doc, " ")
	clean = regexp.MustCompile(`(?is)<br\s*/?>|</(p|div|li|h[1-6]|section|article)>`).ReplaceAllString(clean, "\n")
	clean = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(clean, " ")
	clean = html.UnescapeString(clean)
	var lines []string
	for _, l := range strings.Split(clean, "\n") {
		l = strings.Join(strings.Fields(l), " ")
		if len([]rune(l)) >= 25 { // menü ve kısa parçaları ele
			lines = append(lines, l)
		}
	}
	text = strings.Join(lines, "\n")
	if desc != "" {
		text = desc + "\n" + text
	}
	if r := []rune(text); len(r) > maxChars {
		text = string(r[:maxChars])
	}
	if strings.TrimSpace(text) == "" {
		return title, "", fmt.Errorf("sayfada okunabilir metin bulunamadı")
	}
	return title, text, nil
}

func isPublicIP(ip net.IP) bool {
	return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() ||
		ip.Equal(net.ParseIP("169.254.169.254")) || (ip.To4() != nil && ip.To4()[0] == 100 && ip.To4()[1]&0xC0 == 64))
}
