package household

import (
	"crypto/rand"
	"strings"
)

// Alphabet without look-alikes (no 0/O, 1/I/L): 31 symbols, so 8 characters
// give ~39.6 bits, plenty against guessing when combined with rate limiting.
const (
	codeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	codeLen      = 8
	// Largest multiple of len(codeAlphabet) that fits in a byte; bytes at or
	// above it are rejected so every symbol is equally likely.
	codeByteLimit = 256 / len(codeAlphabet) * len(codeAlphabet)
)

// newInviteCode returns a random code in canonical form (no dash).
func newInviteCode() (string, error) {
	out := make([]byte, 0, codeLen)
	buf := make([]byte, codeLen*2)
	for len(out) < codeLen {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if int(b) >= codeByteLimit {
				continue
			}
			out = append(out, codeAlphabet[int(b)%len(codeAlphabet)])
			if len(out) == codeLen {
				break
			}
		}
	}
	return string(out), nil
}

// NormalizeInviteCode turns user input like " k7qm-2xpa " into "K7QM2XPA".
func NormalizeInviteCode(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		if r == '-' || r == ' ' || r == '\t' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// FormatInviteCode renders a canonical code for humans: "K7QM-2XPA".
func FormatInviteCode(code string) string {
	if len(code) != codeLen {
		return code
	}
	return code[:4] + "-" + code[4:]
}
