package shopping

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Parsing free text into list items, for messages to the bot like
// "молоко 2л, хлеб, яйца 10". Mirrors frontend/src/lib/units.ts
// (parseQuickAdd) so the bot and the app understand the same input.

var unitWords = map[string]string{
	"шт": "pcs", "штук": "pcs", "штука": "pcs", "штуки": "pcs",
	"кг": "kg", "кило": "kg", "килограмм": "kg", "килограмма": "kg",
	"г": "g", "гр": "g", "грамм": "g", "грамма": "g",
	"л": "l", "литр": "l", "литра": "l", "литров": "l",
	"мл": "ml",
	"уп": "pack", "упак": "pack", "упаковка": "pack", "упаковки": "pack", "упаковок": "pack",
	"пач": "pack", "пачка": "pack", "пачки": "pack", "пачек": "pack",
}

var trailingRe, leadingRe = func() (*regexp.Regexp, *regexp.Regexp) {
	words := make([]string, 0, len(unitWords))
	for w := range unitWords {
		words = append(words, w)
	}
	// Longest first, so "литра" wins over "л".
	sort.Slice(words, func(i, j int) bool { return len([]rune(words[i])) > len([]rune(words[j])) })
	units := strings.Join(words, "|")
	num := `(\d+(?:[.,]\d+)?)`
	return regexp.MustCompile(`(?i)^(.+?)[\s,]+(?:[xх×]\s*)?` + num + `\s*(` + units + `)?\.?$`),
		regexp.MustCompile(`(?i)^` + num + `\s*(` + units + `)?\.?\s+(.+)$`)
}()

// ParsedItem is one item from free text.
type ParsedItem struct {
	Name     string
	Quantity *float64
	Unit     *string
}

// ParseItem splits "Молоко 2л" into name and amount. A number without a
// unit means pieces; "Молоко 3,2%" stays a plain name.
func ParseItem(text string) ParsedItem {
	text = strings.Join(strings.Fields(text), " ")
	toNum := func(s string) *float64 {
		f, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64)
		if err != nil {
			return nil
		}
		return &f
	}
	toUnit := func(s string) *string {
		u := "pcs"
		if s != "" {
			u = unitWords[strings.ToLower(s)]
		}
		return &u
	}
	if m := trailingRe.FindStringSubmatch(text); m != nil {
		return ParsedItem{Name: strings.TrimSpace(m[1]), Quantity: toNum(m[2]), Unit: toUnit(m[3])}
	}
	if m := leadingRe.FindStringSubmatch(text); m != nil {
		return ParsedItem{Name: strings.TrimSpace(m[3]), Quantity: toNum(m[1]), Unit: toUnit(m[2])}
	}
	return ParsedItem{Name: text}
}

// SplitItems splits a message into separate items: by lines, ";" and
// commas — except a comma between digits, which is a decimal separator
// ("Молоко 2,5%").
func SplitItems(text string) []string {
	runes := []rune(text)
	var parts []string
	var cur strings.Builder
	flush := func() {
		if s := strings.TrimSpace(cur.String()); s != "" {
			parts = append(parts, s)
		}
		cur.Reset()
	}
	for i, r := range runes {
		isDecimalComma := r == ',' && i > 0 && i+1 < len(runes) && unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1])
		if r == '\n' || r == ';' || (r == ',' && !isDecimalComma) {
			flush()
			continue
		}
		cur.WriteRune(r)
	}
	flush()
	return parts
}
