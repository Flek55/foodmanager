package auth

import (
	"net/mail"
	"strings"
	"unicode/utf8"

	"foodmanager/internal/apperr"
)

const (
	minPasswordLen    = 8
	maxPasswordLen    = 128
	maxDisplayNameLen = 50
	maxEmailLen       = 254
)

// normalizeEmail trims and lowercases the address and checks it's a bare
// address (no "Name <addr>" form).
func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" {
		return "", apperr.Invalid("email", "укажите email")
	}
	if len(email) > maxEmailLen {
		return "", apperr.Invalid("email", "слишком длинный email")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || !strings.Contains(email[strings.LastIndex(email, "@"):], ".") {
		return "", apperr.Invalid("email", "похоже, в email опечатка")
	}
	return email, nil
}

func validatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < minPasswordLen {
		return apperr.Invalid("password", "пароль должен быть не короче 8 символов")
	}
	if n > maxPasswordLen {
		return apperr.Invalid("password", "пароль слишком длинный")
	}
	return nil
}

func normalizeDisplayName(raw string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" {
		return "", apperr.Invalid("displayName", "укажите имя")
	}
	if utf8.RuneCountInString(name) > maxDisplayNameLen {
		return "", apperr.Invalid("displayName", "имя должно быть не длиннее 50 символов")
	}
	return name, nil
}
