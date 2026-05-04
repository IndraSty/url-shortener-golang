package useragent

import "strings"

// Info holds parsed user-agent data.
type Info struct {
	Device  string // "desktop" | "mobile" | "tablet" | "bot"
	OS      string // "Windows" | "macOS" | "Linux" | "Android" | "iOS" | "Unknown"
	Browser string // "Chrome" | "Firefox" | "Safari" | "Edge" | "Opera" | "Unknown"
}

// Parse extracts device, OS, and browser from a raw User-Agent string.
// This is a lightweight rule-based parser — no external library needed.
// Order of checks matters: more specific patterns must come before general ones.
func Parse(ua string) Info {
	if ua == "" {
		return Info{Device: "unknown", OS: "Unknown", Browser: "Unknown"}
	}

	lower := strings.ToLower(ua)

	return Info{
		Device:  detectDevice(lower),
		OS:      detectOS(lower),
		Browser: detectBrowser(lower),
	}
}

// detectDevice returns "bot", "tablet", "mobile", or "desktop".
// Bot check must come first — bots often include mobile strings.
func detectDevice(ua string) string {
	// Bot detection — common crawlers and monitoring agents
	botKeywords := []string{
		"bot", "crawler", "spider", "slurp", "googlebot",
		"bingbot", "yandexbot", "duckduckbot", "facebookexternalhit",
		"twitterbot", "linkedinbot", "whatsapp", "telegrambot",
		"curl", "wget", "python-requests", "go-http-client",
		"postman", "insomnia", "uptimerobot", "pingdom",
	}
	for _, kw := range botKeywords {
		if strings.Contains(ua, kw) {
			return "bot"
		}
	}

	// Tablet — check before mobile because some tablets include "mobile"
	if strings.Contains(ua, "ipad") ||
		(strings.Contains(ua, "android") && !strings.Contains(ua, "mobile")) ||
		strings.Contains(ua, "tablet") {
		return "tablet"
	}

	// Mobile
	mobileKeywords := []string{
		"iphone", "ipod", "android", "mobile", "blackberry",
		"windows phone", "opera mini", "opera mobi",
	}
	for _, kw := range mobileKeywords {
		if strings.Contains(ua, kw) {
			return "mobile"
		}
	}

	return "desktop"
}

// detectOS returns the operating system name.
// Checks are ordered from most-specific to least-specific.
func detectOS(ua string) string {
	switch {
	case strings.Contains(ua, "windows phone"):
		return "Windows Phone"
	case strings.Contains(ua, "windows"):
		return "Windows"
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipod"):
		return "iOS"
	case strings.Contains(ua, "ipad"):
		return "iPadOS"
	case strings.Contains(ua, "mac os x") || strings.Contains(ua, "macos"):
		return "macOS"
	case strings.Contains(ua, "android"):
		return "Android"
	case strings.Contains(ua, "linux"):
		return "Linux"
	case strings.Contains(ua, "cros"):
		return "ChromeOS"
	default:
		return "Unknown"
	}
}

// detectBrowser returns the browser name.
// Order matters: Edge and Opera embed "Chrome" in their UA,
// so they must be checked before Chrome.
func detectBrowser(ua string) string {
	switch {
	case strings.Contains(ua, "edg/") || strings.Contains(ua, "edge/"):
		return "Edge"
	case strings.Contains(ua, "opr/") || strings.Contains(ua, "opera"):
		return "Opera"
	case strings.Contains(ua, "brave"):
		return "Brave"
	case strings.Contains(ua, "samsungbrowser"):
		return "Samsung Browser"
	case strings.Contains(ua, "chrome") || strings.Contains(ua, "crios"):
		return "Chrome"
	case strings.Contains(ua, "firefox") || strings.Contains(ua, "fxios"):
		return "Firefox"
	case strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome"):
		return "Safari"
	case strings.Contains(ua, "msie") || strings.Contains(ua, "trident"):
		return "Internet Explorer"
	default:
		return "Unknown"
	}
}

// StripReferrer extracts the domain from a referrer URL,
// dropping the path, query parameters, and fragment.
// This protects visitor privacy and normalizes referrer data.
//
// Examples:
//
//	StripReferrer("https://google.com/search?q=test") → "google.com"
//	StripReferrer("https://t.co/abc123")              → "t.co"
//	StripReferrer("")                                  → ""
//	StripReferrer("android-app://com.google.android") → ""
func StripReferrer(referrer string) string {
	if referrer == "" {
		return ""
	}

	var rest string
	switch {
	case strings.HasPrefix(referrer, "https://"):
		rest = referrer[len("https://"):]
	case strings.HasPrefix(referrer, "http://"):
		rest = referrer[len("http://"):]
	default:
		// Reject non-HTTP schemes: android-app://, ftp://, etc.
		return ""
	}

	// Remove scheme
	s := referrer
	if _, after, ok := strings.Cut(s, "://"); ok {
		_ = after
	} else {
		// No scheme — not a standard HTTP referrer
		return ""
	}

	// Remove path and everything after
	if idx := strings.Index(rest, "/"); idx != -1 {
		rest = rest[:idx]
	}

	// Remove port
	if idx := strings.LastIndex(rest, ":"); idx != -1 {
		rest = rest[:idx]
	}

	// Remove www. prefix for normalization
	rest = strings.TrimPrefix(rest, "www.")

	return strings.ToLower(rest)
}
