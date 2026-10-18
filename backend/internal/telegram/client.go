// Package telegram is a minimal Telegram Bot API client: just the handful
// of methods the bot uses, plain JSON over HTTP. See
// https://core.telegram.org/bots/api
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const DefaultAPIURL = "https://api.telegram.org"

type Client struct {
	token   string
	baseURL string
	http    *http.Client
}

// New creates a client. apiURL is normally DefaultAPIURL; tests point it at
// a fake server, and it can also be a self-hosted Bot API server or proxy.
func New(token, apiURL string) *Client {
	if apiURL == "" {
		apiURL = DefaultAPIURL
	}
	return &Client{
		token:   token,
		baseURL: strings.TrimRight(apiURL, "/"),
		// Above the long-poll timeout used by GetUpdates.
		http: &http.Client{Timeout: 70 * time.Second},
	}
}

// Error is an error answer from Telegram, e.g. 403 "bot was blocked by the user".
type Error struct {
	Code        int
	Description string
}

func (e *Error) Error() string { return fmt.Sprintf("telegram: %d %s", e.Code, e.Description) }

// IsBlocked reports whether the user blocked the bot or deleted the chat:
// sending to them will never work again until they /start the bot.
func IsBlocked(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == http.StatusForbidden
}

// call performs one Bot API method and decodes "result" into out.
func (c *Client) call(ctx context.Context, method string, params, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	url := c.baseURL + "/bot" + c.token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return c.redact(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return c.redact(err)
	}
	defer resp.Body.Close()

	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("telegram %s: decode response (HTTP %d): %w", method, resp.StatusCode, err)
	}
	if !envelope.OK {
		return &Error{Code: envelope.ErrorCode, Description: envelope.Description}
	}
	if out != nil {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}

// redact strips the bot token from transport errors: *url.Error prints the
// full request URL, and the token is in it. Leaking it to logs would let
// anyone control the bot.
func (c *Client) redact(err error) error {
	return errors.New(strings.ReplaceAll(err.Error(), c.token, "<token>"))
}

// ---- Methods ----

func (c *Client) GetMe(ctx context.Context) (User, error) {
	var u User
	err := c.call(ctx, "getMe", struct{}{}, &u)
	return u, err
}

// GetUpdates long-polls for new updates (waits up to timeout for one).
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeout time.Duration) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         int(timeout.Seconds()),
		"allowed_updates": []string{"message", "callback_query"},
	}, &updates)
	return updates, err
}

type SendMessageParams struct {
	ChatID      int64                 `json:"chat_id"`
	Text        string                `json:"text"`
	ParseMode   string                `json:"parse_mode,omitempty"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	// Link previews would turn every message with a URL into a big card.
	LinkPreviewOptions *LinkPreviewOptions `json:"link_preview_options,omitempty"`
}

type LinkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled"`
}

func (c *Client) SendMessage(ctx context.Context, p SendMessageParams) (Message, error) {
	if p.LinkPreviewOptions == nil {
		p.LinkPreviewOptions = &LinkPreviewOptions{IsDisabled: true}
	}
	var m Message
	err := c.call(ctx, "sendMessage", p, &m)
	return m, err
}

type EditMessageTextParams struct {
	ChatID      int64                 `json:"chat_id"`
	MessageID   int64                 `json:"message_id"`
	Text        string                `json:"text"`
	ParseMode   string                `json:"parse_mode,omitempty"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

func (c *Client) EditMessageText(ctx context.Context, p EditMessageTextParams) error {
	err := c.call(ctx, "editMessageText", p, nil)
	// Re-rendering an unchanged message is harmless.
	var e *Error
	if errors.As(err, &e) && strings.Contains(e.Description, "message is not modified") {
		return nil
	}
	return err
}

// AnswerCallbackQuery stops the loading spinner on a pressed inline button,
// optionally showing a short toast.
func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	return c.call(ctx, "answerCallbackQuery", map[string]any{"callback_query_id": id, "text": text}, nil)
}

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

func (c *Client) SetMyCommands(ctx context.Context, commands []BotCommand) error {
	return c.call(ctx, "setMyCommands", map[string]any{"commands": commands}, nil)
}

// SetMenuButtonWebApp makes the button next to the message field open the
// Mini App (our PWA inside Telegram) for every chat with the bot.
func (c *Client) SetMenuButtonWebApp(ctx context.Context, text, url string) error {
	return c.call(ctx, "setChatMenuButton", map[string]any{
		"menu_button": map[string]any{"type": "web_app", "text": text, "web_app": map[string]string{"url": url}},
	}, nil)
}
