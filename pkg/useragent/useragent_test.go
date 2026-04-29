package useragent_test

import (
	"testing"

	"github.com/IndraSty/url-shortener-golang/pkg/useragent"
	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name    string
		ua      string
		device  string
		os      string
		browser string
	}{
		{
			name:    "chrome on windows",
			ua:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			device:  "desktop",
			os:      "Windows",
			browser: "Chrome",
		},
		{
			name:    "safari on iphone",
			ua:      "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			device:  "mobile",
			os:      "iOS",
			browser: "Safari",
		},
		{
			name:    "firefox on linux",
			ua:      "Mozilla/5.0 (X11; Linux x86_64; rv:124.0) Gecko/20100101 Firefox/124.0",
			device:  "desktop",
			os:      "Linux",
			browser: "Firefox",
		},
		{
			name:    "edge on windows",
			ua:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
			device:  "desktop",
			os:      "Windows",
			browser: "Edge",
		},
		{
			name:    "android tablet",
			ua:      "Mozilla/5.0 (Linux; Android 13; SM-X700) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
			device:  "tablet",
			os:      "Android",
			browser: "Chrome",
		},
		{
			name:    "googlebot",
			ua:      "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
			device:  "bot",
			os:      "Unknown",
			browser: "Unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := useragent.Parse(tc.ua)
			assert.Equal(t, tc.device, info.Device)
			assert.Equal(t, tc.os, info.OS)
			assert.Equal(t, tc.browser, info.Browser)
		})
	}
}

func TestStripReferrer(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"https://www.google.com/search?q=test", "google.com"},
		{"https://t.co/abc123", "t.co"},
		{"http://facebook.com/some/path?ref=123", "facebook.com"},
		{"https://news.ycombinator.com/", "news.ycombinator.com"},
		{"", ""},
		{"android-app://com.google.android.gm", ""},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			result := useragent.StripReferrer(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func BenchmarkParse(b *testing.B) {
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
	for i := 0; i < b.N; i++ {
		useragent.Parse(ua)
	}
}
