package shopping

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"

	"foodmanager/internal/apperr"
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

const (
	maxProductName  = 80
	maxDescription  = 200
	maxNote         = 200
	maxStoreName    = 40
	maxCategoryName = 40
	maxEmoji        = 16
	maxQuantity     = 100_000
)

var validUnits = map[string]bool{"pcs": true, "kg": true, "g": true, "l": true, "ml": true, "pack": true}

// requiredText collapses whitespace and enforces 1..max characters.
func requiredText(raw string, max int, field, emptyMsg string) (string, error) {
	s := strings.Join(strings.Fields(raw), " ")
	if s == "" {
		return "", apperr.Invalid(field, emptyMsg)
	}
	if utf8.RuneCountInString(s) > max {
		return "", apperr.Invalid(field, "не длиннее "+strconv.Itoa(max)+" символов")
	}
	return s, nil
}

// optionalText trims; empty becomes nil (the field is cleared).
func optionalText(raw *string, max int, field string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(s) > max {
		return nil, apperr.Invalid(field, "не длиннее "+strconv.Itoa(max)+" символов")
	}
	return &s, nil
}

// normalizeQuantity validates the amount. A unit without a quantity is
// meaningless and dropped; a quantity without a unit means pieces.
func normalizeQuantity(qty *float64, unit *string) (*float64, *string, error) {
	if unit != nil && !validUnits[*unit] {
		return nil, nil, apperr.Invalid("unit", "неизвестная единица измерения")
	}
	if qty == nil {
		return nil, nil, nil
	}
	if *qty <= 0 || *qty > maxQuantity {
		return nil, nil, apperr.Invalid("quantity", "количество должно быть больше нуля")
	}
	if unit == nil {
		pcs := "pcs"
		unit = &pcs
	}
	return qty, unit, nil
}
