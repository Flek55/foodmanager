package httpapi

import (
	"context"
	"errors"
	"time"

	"foodmanager/internal/api"
	"foodmanager/internal/apperr"
	"foodmanager/internal/auth"
	"foodmanager/internal/store"
	"foodmanager/internal/telegram"
)

// TelegramConfig enables the Telegram endpoints. nil = no bot configured:
// they answer 404 telegram_disabled and the app hides Telegram features.
type TelegramConfig struct {
	BotToken    string // to verify Mini App init data
	BotUsername string // for t.me/<bot> deep links
}

// Mini App init data older than this is not accepted for login. It's when
// the app was opened inside Telegram, not when the request was made.
const initDataMaxAge = 24 * time.Hour

var errTelegramDisabled = apperr.NotFoundCode("telegram_disabled", "Telegram не настроен на этом сервере")

func (h *Handler) GetConfig(_ context.Context, _ api.GetConfigRequestObject) (api.GetConfigResponseObject, error) {
	var cfg api.AppConfig
	if h.telegram != nil {
		cfg.TelegramBot = &h.telegram.BotUsername
	}
	return api.GetConfig200JSONResponse(cfg), nil
}

func (h *Handler) deepLink(token string) string {
	return "https://t.me/" + h.telegram.BotUsername + "?start=" + token
}

// ---- linking ----

func (h *Handler) CreateTelegramLink(ctx context.Context, _ api.CreateTelegramLinkRequestObject) (api.CreateTelegramLinkResponseObject, error) {
	if h.telegram == nil {
		return nil, errTelegramDisabled
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	token, expires, err := h.auth.NewTelegramLinkToken(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return api.CreateTelegramLink200JSONResponse{Url: h.deepLink(token), ExpiresAt: expires}, nil
}

func (h *Handler) UnlinkTelegram(ctx context.Context, _ api.UnlinkTelegramRequestObject) (api.UnlinkTelegramResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.auth.UnlinkTelegram(ctx, user.ID); err != nil {
		return nil, err
	}
	return api.UnlinkTelegram204Response{}, nil
}

func (h *Handler) LinkTelegramWebApp(ctx context.Context, req api.LinkTelegramWebAppRequestObject) (api.LinkTelegramWebAppResponseObject, error) {
	if h.telegram == nil {
		return nil, errTelegramDisabled
	}
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	tg, err := h.verifyInitData(req.Body.InitData)
	if err != nil {
		return nil, err
	}
	acc, err := h.auth.LinkTelegram(ctx, user.ID, tg)
	if err != nil {
		return nil, err
	}
	return api.LinkTelegramWebApp200JSONResponse(toTelegramAccount(acc)), nil
}

func (h *Handler) UpdateTelegramSettings(ctx context.Context, req api.UpdateTelegramSettingsRequestObject) (api.UpdateTelegramSettingsResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	acc, err := h.auth.UpdateTelegramSettings(ctx, user.ID, req.Body.NotifyAdded, req.Body.NotifyStock)
	if err != nil {
		return nil, err
	}
	return api.UpdateTelegramSettings200JSONResponse(toTelegramAccount(acc)), nil
}

// ---- login through the bot ----

func (h *Handler) StartTelegramLogin(ctx context.Context, _ api.StartTelegramLoginRequestObject) (api.StartTelegramLoginResponseObject, error) {
	if h.telegram == nil {
		return nil, errTelegramDisabled
	}
	token, expires, err := h.auth.StartTelegramLogin(ctx, userAgent(ctx))
	if err != nil {
		return nil, err
	}
	return api.StartTelegramLogin200JSONResponse{Token: token, Url: h.deepLink(token), ExpiresAt: expires}, nil
}

func (h *Handler) CompleteTelegramLogin(ctx context.Context, req api.CompleteTelegramLoginRequestObject) (api.CompleteTelegramLoginResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	user, sess, status, err := h.auth.CompleteTelegramLogin(ctx, req.Body.Token, userAgent(ctx))
	if err != nil {
		return nil, err
	}
	switch status {
	case auth.LoginPending:
		return api.CompleteTelegramLogin202JSONResponse{Status: api.Pending}, nil
	case auth.LoginExpired:
		return api.CompleteTelegramLogin410JSONResponse{GoneJSONResponse: api.GoneJSONResponse{
			Code: "login_expired", Message: "вход отменён или время вышло — попробуйте ещё раз",
		}}, nil
	}
	me, err := h.me(ctx, user)
	if err != nil {
		return nil, err
	}
	cookie := h.cookies.session(sess)
	return api.CompleteTelegramLogin200JSONResponse{Body: me, Headers: api.CompleteTelegramLogin200ResponseHeaders{SetCookie: &cookie}}, nil
}

// ---- Mini App ----

func (h *Handler) TelegramWebAppLogin(ctx context.Context, req api.TelegramWebAppLoginRequestObject) (api.TelegramWebAppLoginResponseObject, error) {
	if h.telegram == nil {
		return nil, errTelegramDisabled
	}
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	tg, err := h.verifyInitData(req.Body.InitData)
	if err != nil {
		return nil, err
	}
	create := req.Body.Create != nil && *req.Body.Create
	user, sess, err := h.auth.LoginWithTelegram(ctx, tg, create, userAgent(ctx))
	if err != nil {
		return nil, err
	}
	me, err := h.me(ctx, user)
	if err != nil {
		return nil, err
	}
	cookie := h.cookies.session(sess)
	return api.TelegramWebAppLogin200JSONResponse{Body: me, Headers: api.TelegramWebAppLogin200ResponseHeaders{SetCookie: &cookie}}, nil
}

// verifyInitData checks Telegram's signature and returns the user. In a
// private chat with the bot, the chat id equals the user id.
func (h *Handler) verifyInitData(initData string) (auth.TelegramUser, error) {
	u, err := telegram.ValidateInitData(initData, h.telegram.BotToken, initDataMaxAge, time.Now())
	if errors.Is(err, telegram.ErrInvalidInitData) {
		return auth.TelegramUser{}, apperr.Unauthorized("данные Telegram не прошли проверку — откройте приложение из бота заново")
	}
	if err != nil {
		return auth.TelegramUser{}, err
	}
	return auth.TelegramUser{ID: u.ID, ChatID: u.ID, Username: u.Username, FirstName: u.FirstName, LastName: u.LastName}, nil
}

func toTelegramAccount(a store.TelegramAccount) api.TelegramAccount {
	return api.TelegramAccount{Username: a.Username, FirstName: a.FirstName, NotifyAdded: a.NotifyAdded, NotifyStock: a.NotifyStock}
}
