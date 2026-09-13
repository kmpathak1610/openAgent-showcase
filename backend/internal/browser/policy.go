package browser

import (
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	MaxConcurrentBrowsers = 3
	MaxSessionsPerUser    = 5
	SessionTTL            = 30 * time.Minute
	PageTimeout           = 30 * time.Second
	MaxPagesVisited       = 10
	MaxDownloads          = 5
	MaxDownloadSize       = 10 * 1024 * 1024 // 10MB
	MaxContentChars       = 20000            // LLM content limit
	TotalTaskTimeout      = 5 * time.Minute
)

// Allowed schemes
var allowedSchemes = map[string]bool{"http": true, "https": true}

// IsAllowedURL validates URL for SSRF, private IP, scheme, etc.
func IsAllowedURL(raw string) (bool, string) {
	if raw == "" {
		return false, "empty url"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false, "invalid url"
	}
	if !allowedSchemes[strings.ToLower(u.Scheme)] {
		return false, "scheme not allowed, use http/https"
	}
	if u.Host == "" {
		return false, "host required"
	}
	// block file, data, javascript etc already by scheme check
	// SSRF: block private IPs if host is IP literal
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		if isPrivateIP(ip) {
			return false, "private/internal IP not allowed"
		}
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return false, "loopback/link-local not allowed"
		}
	}
	// block internal domains
	lower := strings.ToLower(u.Host)
	if strings.HasSuffix(lower, ".internal") || strings.HasSuffix(lower, ".local") {
		return false, "internal domain not allowed"
	}
	// basic length limit
	if len(raw) > 2048 {
		return false, "url too long"
	}
	return true, ""
}

func isPrivateIP(ip net.IP) bool {
	privateBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"fc00::/7",
	}
	for _, cidr := range privateBlocks {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// Domain extraction for audit
func DomainOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil { return "" }
	return u.Host
}

// Risk classification for browser tools (maps to existing approval risk)
func RiskForTool(name string) string {
	switch name {
	case "browser.search", "browser.navigate", "browser.extract", "browser.screenshot", "browser.back", "browser.wait", "browser.new_tab":
		return "low"
	case "browser.click", "browser.type", "browser.select", "browser.download":
		return "medium"
	default:
		return "low"
	}
}

// RequiresApproval for high-risk browser side effects (publishing etc. handled via provider, not browser directly)
func RequiresApproval(name string, input map[string]any) bool {
	// Medium+ with external side effect could require approval — but browser click/type alone is medium, not auto approval.
	// Only if tool is publishing/social handled via provider layer, not browser.
	return false
}
