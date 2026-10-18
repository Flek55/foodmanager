package httpapi

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"foodmanager/internal/api"
)

// family registers a user with a household and returns the client and id.
func family(t *testing.T, base, email string) (*client, string) {
	c := newClient(t, base)
	c.do("POST", "/auth/register", api.RegisterRequest{Email: email, Password: "password123", DisplayName: "Маша"}, nil)
	var h api.HouseholdDetail
	if code := c.do("POST", "/households", api.HouseholdInput{Name: "Ивановы"}, &h); code != http.StatusCreated {
		t.Fatalf("create household: %d", code)
	}
	return c, h.Id.String()
}

func TestShoppingListFlow(t *testing.T) {
	base := newServer(t)
	c, hid := family(t, base, "masha@example.com")

	var snap api.ListSnapshot
	if code := c.do("GET", "/households/"+hid+"/list", nil, &snap); code != http.StatusOK {
		t.Fatalf("get list: %d", code)
	}
	if len(snap.Categories) != 14 || len(snap.Items) != 0 {
		t.Fatalf("new household: %d categories, %d items", len(snap.Categories), len(snap.Items))
	}

	qty, unit := 2.0, api.Unit("l")
	var item api.ListItem
	if code := c.do("POST", "/households/"+hid+"/items", api.AddItemRequest{Name: "Молоко", Quantity: &qty, Unit: &unit}, &item); code != http.StatusCreated {
		t.Fatalf("add: %d", code)
	}
	if item.CategoryId == nil || item.AddedBy == nil || *item.AddedBy != "Маша" || *item.Quantity != 2 {
		t.Fatalf("unexpected item %+v", item)
	}

	// Same product again (different case) merges into the existing item.
	var again api.ListItem
	if code := c.do("POST", "/households/"+hid+"/items", api.AddItemRequest{Name: "молоко"}, &again); code != http.StatusOK || again.Id != item.Id {
		t.Fatalf("re-add: %d %+v", code, again)
	}

	var checked api.ListItem
	if code := c.do("PUT", "/households/"+hid+"/items/"+item.Id.String()+"/checked", api.SetCheckedRequest{Checked: true}, &checked); code != http.StatusOK || checked.CheckedAt == nil {
		t.Fatalf("check: %d %+v", code, checked)
	}

	var cleared api.ClearCheckedResult
	if code := c.do("POST", "/households/"+hid+"/items/clear-checked", nil, &cleared); code != http.StatusOK || cleared.Removed != 1 {
		t.Fatalf("clear: %d %+v", code, cleared)
	}

	c.do("GET", "/households/"+hid+"/list", nil, &snap)
	if len(snap.Items) != 0 || len(snap.Products) != 1 || snap.Products[0].UseCount != 1 {
		t.Fatalf("after clear: %+v", snap)
	}
}

func TestListIsPrivateToHousehold(t *testing.T) {
	base := newServer(t)
	_, hid := family(t, base, "masha@example.com")
	stranger, _ := family(t, base, "petya@example.com")

	if code := stranger.do("GET", "/households/"+hid+"/list", nil, nil); code != http.StatusNotFound {
		t.Fatalf("stranger reads list: %d", code)
	}
	if code := stranger.do("POST", "/households/"+hid+"/items", api.AddItemRequest{Name: "Хлеб"}, nil); code != http.StatusNotFound {
		t.Fatalf("stranger adds item: %d", code)
	}
}

func TestEventsStream(t *testing.T) {
	base := newServer(t)
	c, hid := family(t, base, "masha@example.com")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/households/"+hid+"/events", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("events: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	waitFor := func(prefix string) string {
		t.Helper()
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatalf("stream closed while waiting for %q", prefix)
				}
				if strings.HasPrefix(l, prefix) {
					return l
				}
			case <-ctx.Done():
				t.Fatalf("timeout waiting for %q", prefix)
			}
		}
	}

	waitFor("event: ready")
	c.do("POST", "/households/"+hid+"/items", api.AddItemRequest{Name: "Хлеб"}, nil)
	if got := waitFor(`data: {"kind"`); !strings.Contains(got, `"kind":"list"`) {
		t.Fatalf("unexpected event %q", got)
	}

	// Strangers can't listen in.
	stranger, _ := family(t, base, "petya@example.com")
	if code := stranger.do("GET", "/households/"+hid+"/events", nil, nil); code != http.StatusNotFound {
		t.Fatalf("stranger events: %d", code)
	}
}
