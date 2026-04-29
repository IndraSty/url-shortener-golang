package qrcode

import (
	"bytes"
	"fmt"

	goqrcode "github.com/skip2/go-qrcode"
)

const (
	// DefaultSize is the QR code PNG dimension in pixels.
	DefaultSize = 256

	// MaxSize prevents abuse via enormous image generation.
	MaxSize = 1024

	// MinSize keeps QR codes scannable.
	MinSize = 64
)

// Generator creates QR code PNGs server-side using go-qrcode.
// No external service needed — pure Go, zero network call.
type Generator struct{}

// NewGenerator creates a QR code generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// Generate creates a PNG QR code for the given URL.
// size is the image dimension in pixels (clamped to MinSize–MaxSize).
// Returns the raw PNG bytes ready to serve as image/png.
func (g *Generator) Generate(url string, size int) ([]byte, error) {
	if url == "" {
		return nil, fmt.Errorf("qrcode: url cannot be empty")
	}

	// Clamp size to safe bounds
	if size < MinSize {
		size = MinSize
	}
	if size > MaxSize {
		size = MaxSize
	}

	// go-qrcode: medium error correction (recovers up to 15% data loss)
	// Good balance between QR code density and scan reliability
	qr, err := goqrcode.New(url, goqrcode.Medium)
	if err != nil {
		return nil, fmt.Errorf("qrcode: failed to create qr: %w", err)
	}

	// Write PNG to an in-memory buffer — no temp file needed
	var buf bytes.Buffer
	if err := qr.Write(size, &buf); err != nil {
		return nil, fmt.Errorf("qrcode: failed to encode png: %w", err)
	}

	return buf.Bytes(), nil
}

// GenerateDefault generates a QR code at the default 256×256 size.
func (g *Generator) GenerateDefault(url string) ([]byte, error) {
	return g.Generate(url, DefaultSize)
}
