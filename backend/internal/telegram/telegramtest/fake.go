// Package telegramtest is a fake Telegram Bot API server for tests: it
// serves queued updates to getUpdates (long-poll style) and records every
// message the bot sends.
package telegramtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"foodmanager/internal/telegram"
)

const Token = "123456:TEST-TOKEN"

type Server struct {
	URL string

	mu       sync.Mutex
	updates  []telegram.Update
	nextID   int64
	nextMsg  int64
	sent     []telegram.SendMessageParams
	edited   []telegram.EditMessageTextParams
	answered []string
	blocked  map[int64]bool
	notify   chan struct{}
}

func New(t testing.TB) *Server {
	s := &Server{nextID: 1, nextMsg: 100, blocked: map[int64]bool{}, notify: make(chan struct{}, 1)}
	srv := httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

// Block makes sendMessage to chatID fail like a user who blocked the bot.
func (s *Server) Block(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocked[chatID] = true
}

// SendText queues a private text message from user to the bot.
func (s *Server) SendText(from telegram.User, text string) {
	s.push(telegram.Update{Message: &telegram.Message{
		MessageID: s.msgID(), From: &from, Chat: telegram.Chat{ID: from.ID, Type: "private"}, Text: text,
	}})
}

// Press queues an inline-button press by user on the given bot message.
func (s *Server) Press(from telegram.User, msg telegram.SendMessageParams, data string) {
	s.push(telegram.Update{CallbackQuery: &telegram.CallbackQuery{
		ID: "cb" + data, From: from, Data: data,
		Message: &telegram.Message{MessageID: 1, Chat: telegram.Chat{ID: msg.ChatID, Type: "private"}},
	}})
}

// WaitSent waits until a message to chatID containing substr was sent.
func (s *Server) WaitSent(t testing.TB, chatID int64, substr string) telegram.SendMessageParams {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, m := range s.sent {
			if m.ChatID == chatID && strings.Contains(m.Text, substr) {
				s.mu.Unlock()
				return m
			}
		}
		s.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no message to %d containing %q; sent: %s", chatID, substr, s.dump())
	return telegram.SendMessageParams{}
}

// WaitEdited waits until a message edit containing substr was made.
func (s *Server) WaitEdited(t testing.TB, substr string) telegram.EditMessageTextParams {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		for _, m := range s.edited {
			if strings.Contains(m.Text, substr) {
				s.mu.Unlock()
				return m
			}
		}
		s.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no edit containing %q", substr)
	return telegram.EditMessageTextParams{}
}

// Sent returns all messages sent so far to chatID.
func (s *Server) Sent(chatID int64) []telegram.SendMessageParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []telegram.SendMessageParams
	for _, m := range s.sent {
		if m.ChatID == chatID {
			out = append(out, m)
		}
	}
	return out
}

func (s *Server) dump() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.MarshalIndent(s.sent, "", "  ")
	return string(b)
}

func (s *Server) msgID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextMsg++
	return s.nextMsg
}

func (s *Server) push(u telegram.Update) {
	s.mu.Lock()
	u.UpdateID = s.nextID
	s.nextID++
	s.updates = append(s.updates, u)
	s.mu.Unlock()
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "bot"+Token {
		reply(w, false, nil, 401, "Unauthorized")
		return
	}
	var p map[string]json.RawMessage
	_ = json.NewDecoder(r.Body).Decode(&p)

	switch parts[1] {
	case "getMe":
		reply(w, true, telegram.User{ID: 1, IsBot: true, FirstName: "Покупки", Username: "test_food_bot"}, 0, "")
	case "getUpdates":
		var offset int64
		_ = json.Unmarshal(p["offset"], &offset)
		reply(w, true, s.pending(r, offset), 0, "")
	case "sendMessage":
		var m telegram.SendMessageParams
		raw, _ := json.Marshal(p)
		_ = json.Unmarshal(raw, &m)
		s.mu.Lock()
		blocked := s.blocked[m.ChatID]
		if !blocked {
			s.sent = append(s.sent, m)
		}
		s.mu.Unlock()
		if blocked {
			reply(w, false, nil, 403, "Forbidden: bot was blocked by the user")
			return
		}
		reply(w, true, telegram.Message{MessageID: s.msgID(), Chat: telegram.Chat{ID: m.ChatID}, Text: m.Text}, 0, "")
	case "editMessageText":
		var m telegram.EditMessageTextParams
		raw, _ := json.Marshal(p)
		_ = json.Unmarshal(raw, &m)
		s.mu.Lock()
		s.edited = append(s.edited, m)
		s.mu.Unlock()
		reply(w, true, true, 0, "")
	case "answerCallbackQuery":
		var id string
		_ = json.Unmarshal(p["callback_query_id"], &id)
		s.mu.Lock()
		s.answered = append(s.answered, id)
		s.mu.Unlock()
		reply(w, true, true, 0, "")
	default: // setMyCommands, setChatMenuButton, ...
		reply(w, true, true, 0, "")
	}
}

// pending returns updates with id >= offset, waiting a little for new ones
// like real long polling does.
func (s *Server) pending(r *http.Request, offset int64) []telegram.Update {
	deadline := time.After(500 * time.Millisecond)
	for {
		s.mu.Lock()
		var out []telegram.Update
		for _, u := range s.updates {
			if u.UpdateID >= offset {
				out = append(out, u)
			}
		}
		s.mu.Unlock()
		if len(out) > 0 {
			return out
		}
		select {
		case <-s.notify:
		case <-deadline:
			return []telegram.Update{}
		case <-r.Context().Done():
			return []telegram.Update{}
		}
	}
}

func reply(w http.ResponseWriter, ok bool, result any, code int, desc string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": ok, "result": result, "error_code": code, "description": desc})
}
