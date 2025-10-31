package util

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"net/url"
	"strings"
)

const base62Chars = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func ValidateURL(raw string) bool {
	u, err := url.ParseRequestURI(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return true
}

// Base62 encode of an integer
func base62Encode(num uint64) string {
	if num == 0 {
		return "0"
	}
	b := make([]byte, 0)
	for num > 0 {
		rem := num % 62
		b = append([]byte{base62Chars[rem]}, b...)
		num /= 62
	}
	return string(b)
}

// Deterministic ID generator: produce N-length string by slicing SHA256 digest
// We attempt different offsets if collision occurs.
func DeterministicShortCode(original string, length int, attempt int) string {
	// hash original
	h := sha256.Sum256([]byte(original))
	// pick 8 bytes starting at (attempt mod (32-8+1)) -> convert to uint64
	offset := attempt % (len(h) - 8 + 1)
	slice := h[offset : offset+8]
	num := binary.BigEndian.Uint64(slice)
	// compress if too big by modulo
	// To ensure variety across lengths, we fold the number slightly
	if length >= 10 {
		// use full numeric
	} else {
		// reduce
		limit := uint64(math.Pow(62, float64(length)))
		num = num % limit
	}
	code := base62Encode(num)
	// pad/truncate to requested length
	if len(code) > length {
		code = code[:length]
	} else if len(code) < length {
		// left pad with zeros (not necessary but keeps deterministic)
		code = strings.Repeat("0", length-len(code)) + code
	}
	return code
}
