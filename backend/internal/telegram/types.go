package telegram

import (
	"html"
)

// Only the fields the bot uses; Telegram sends many more.

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // "private", "group", ...
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from,omitempty"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string      `json:"text"`
	CallbackData string      `json:"callback_data,omitempty"`
	URL          string      `json:"url,omitempty"`
	WebApp       *WebAppInfo `json:"web_app,omitempty"`
}

type WebAppInfo struct {
	URL string `json:"url"`
}

// Keyboard builds an inline keyboard, one button per row.
func Keyboard(buttons ...InlineKeyboardButton) *InlineKeyboardMarkup {
	rows := make([][]InlineKeyboardButton, len(buttons))
	for i, b := range buttons {
		rows[i] = []InlineKeyboardButton{b}
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

// Escape makes user-provided text safe inside parse_mode=HTML messages.
func Escape(s string) string { return html.EscapeString(s) }
