package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"foodmanager/internal/apperr"
	"foodmanager/internal/events"
)

const sseHeartbeat = 25 * time.Second

// serveEvents streams household change notifications as Server-Sent Events.
// It's a plain handler rather than an OpenAPI operation because the
// response is an endless text/event-stream, not JSON.
//
// Protocol:
//
//	event: ready            sent once: {"version": N}
//	data: {"kind": ...}     on every change
//	: ping                  every 25s, so proxies don't drop an idle stream
func (h *Handler) serveEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, err := currentUser(ctx)
	if err != nil {
		writeAppError(w, err)
		return
	}
	householdID, err := uuid.Parse(chi.URLParam(r, "householdId"))
	if err != nil {
		writeAppError(w, apperr.NotFoundCode(apperr.CodeHouseholdNotFound, "семья не найдена"))
		return
	}

	// Subscribe before reading the version: an event published in between
	// is then delivered rather than lost.
	sub := h.hub.Subscribe(householdID)
	defer sub.Close()

	version, err := h.shopping.CurrentVersion(ctx, user.ID, householdID)
	if err != nil {
		writeAppError(w, err)
		return
	}

	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx & co)
	w.WriteHeader(http.StatusOK)

	send := func(format string, args ...any) bool {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return false
		}
		return rc.Flush() == nil
	}

	ready, _ := json.Marshal(map[string]int64{"version": version})
	if !send("event: ready\ndata: %s\n\n", ready) {
		return
	}

	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if !send(": ping\n\n") {
				return
			}
		case ev := <-sub.C:
			data, _ := json.Marshal(ev)
			if !send("data: %s\n\n", data) {
				return
			}
		}
	}
}

var _ events.Publisher = (*events.Hub)(nil)
