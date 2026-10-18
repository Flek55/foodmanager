package shopping

import (
	"reflect"
	"testing"
)

func TestSplitItems(t *testing.T) {
	cases := map[string][]string{
		"молоко 2л, хлеб, яйца 10": {"молоко 2л", "хлеб", "яйца 10"},
		"Молоко 2,5%, сыр":         {"Молоко 2,5%", "сыр"},
		"сок 1,5 л\nхлеб;  масло":  {"сок 1,5 л", "хлеб", "масло"},
		" , ,хлеб,,":               {"хлеб"},
		"яйца 10, 2 кг картошки":   {"яйца 10", "2 кг картошки"},
	}
	for in, want := range cases {
		if got := SplitItems(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestParseItem(t *testing.T) {
	type res struct {
		name string
		qty  float64
		unit string
	}
	cases := map[string]res{
		"Молоко 2л":         {"Молоко", 2, "l"},
		"яйца 10":           {"яйца", 10, "pcs"},
		"500 г сыра":        {"сыра", 500, "g"},
		"сок 1,5 литра":     {"сок", 1.5, "l"},
		"Картошка 3 КГ":     {"Картошка", 3, "kg"},
		"батарейки x4":      {"батарейки", 4, "pcs"},
		"Молоко 3,2%":       {"Молоко 3,2%", 0, ""},
		"хлеб":              {"хлеб", 0, ""},
		"  творог   2  уп ": {"творог", 2, "pack"},
	}
	for in, want := range cases {
		got := ParseItem(in)
		var q float64
		var u string
		if got.Quantity != nil {
			q, u = *got.Quantity, *got.Unit
		}
		if got.Name != want.name || q != want.qty || u != want.unit {
			t.Errorf("%q: got %q %v %q, want %+v", in, got.Name, q, u, want)
		}
	}
}

func TestItemLabel(t *testing.T) {
	q, l := 1.5, "kg"
	if got := ItemLabel("Яблоки", &q, &l); got != "Яблоки 1,5 кг" {
		t.Errorf("got %q", got)
	}
	if got := ItemLabel("Хлеб", nil, nil); got != "Хлеб" {
		t.Errorf("got %q", got)
	}
}
