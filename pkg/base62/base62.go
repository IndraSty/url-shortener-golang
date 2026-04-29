package base62

import (
	"fmt"
	"strings"
)

const (
	// alphabet is the base62 character set.
	// Order matters — changing this breaks all existing slugs.
	// We use 0-9 first, then a-z, then A-Z (URL-safe, case-sensitive).
	alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	base     = int64(len(alphabet)) // 62
)

// Encode converts a positive integer ID to a base62 string.
// This is how we generate the default slug from links.id (bigserial).
//
// Examples:
//
//	Encode(1)       → "1"
//	Encode(61)      → "Z"
//	Encode(62)      → "10"
//	Encode(100000)  → "q0U"
func Encode(id int64) string {
	if id == 0 {
		return string(alphabet[0])
	}

	// Pre-allocate a small buffer — max int64 in base62 is ~11 chars
	buf := make([]byte, 0, 11)

	for id > 0 {
		buf = append(buf, alphabet[id%base])
		id /= base
	}

	// Reverse the buffer (we built it least-significant first)
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	return string(buf)
}

// Decode converts a base62 string back to its integer ID.
// Returns an error if the string contains characters outside the alphabet.
//
// Examples:
//
//	Decode("1")    → 1, nil
//	Decode("q0U")  → 100000, nil
//	Decode("!!")   → 0, error
func Decode(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("base62: cannot decode empty string")
	}

	var id int64
	for _, c := range s {
		pos := strings.IndexRune(alphabet, c)
		if pos == -1 {
			return 0, fmt.Errorf("base62: invalid character %q in slug", c)
		}
		id = id*base + int64(pos)
	}

	return id, nil
}

// IsValid returns true when every character in s is part of the base62 alphabet.
// Use this for fast validation of user-provided custom slugs
// before hitting the database.
func IsValid(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune(alphabet, c) {
			return false
		}
	}
	return true
}
