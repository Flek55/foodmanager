package httpapi

import (
	"context"
	"strings"

	"foodmanager/internal/api"
	"foodmanager/internal/apperr"
	"foodmanager/internal/store"
)

func (h *Handler) Register(ctx context.Context, req api.RegisterRequestObject) (api.RegisterResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	user, sess, err := h.auth.Register(ctx, req.Body.Email, req.Body.Password, req.Body.DisplayName, userAgent(ctx))
	if err != nil {
		return nil, err
	}
	me, err := h.me(ctx, user)
	if err != nil {
		return nil, err
	}
	cookie := h.cookies.session(sess)
	return api.Register201JSONResponse{Body: me, Headers: api.Register201ResponseHeaders{SetCookie: &cookie}}, nil
}

func (h *Handler) Login(ctx context.Context, req api.LoginRequestObject) (api.LoginResponseObject, error) {
	if req.Body == nil {
		return nil, apperr.Invalid("", "пустой запрос")
	}
	// Per-account limit on top of the per-IP one: guessing one person's
	// password from many IPs is capped too.
	if !h.limits.loginPerEmail.allow(strings.ToLower(strings.TrimSpace(req.Body.Email))) {
		return nil, apperr.TooManyRequests()
	}
	user, sess, err := h.auth.Login(ctx, req.Body.Email, req.Body.Password, userAgent(ctx))
	if err != nil {
		return nil, err
	}
	me, err := h.me(ctx, user)
	if err != nil {
		return nil, err
	}
	cookie := h.cookies.session(sess)
	return api.Login200JSONResponse{Body: me, Headers: api.Login200ResponseHeaders{SetCookie: &cookie}}, nil
}

// Logout always succeeds and clears the cookie, even without a valid session.
func (h *Handler) Logout(ctx context.Context, _ api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	if token := sessionToken(ctx); token != "" {
		if err := h.auth.Logout(ctx, token); err != nil {
			return nil, err
		}
	}
	cookie := h.cookies.cleared()
	return api.Logout204Response{Headers: api.Logout204ResponseHeaders{SetCookie: &cookie}}, nil
}

func (h *Handler) GetMe(ctx context.Context, _ api.GetMeRequestObject) (api.GetMeResponseObject, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	me, err := h.me(ctx, user)
	if err != nil {
		return nil, err
	}
	return api.GetMe200JSONResponse(me), nil
}

func (h *Handler) me(ctx context.Context, user store.User) (api.Me, error) {
	rows, err := h.households.ListForUser(ctx, user.ID)
	if err != nil {
		return api.Me{}, err
	}
	households := make([]api.HouseholdSummary, len(rows))
	for i, r := range rows {
		households[i] = api.HouseholdSummary{Id: r.ID, Name: r.Name, Role: api.Role(r.Role)}
	}
	me := api.Me{
		User: api.User{
			Id:          user.ID,
			DisplayName: user.DisplayName,
			Email:       user.Email,
			AvatarUrl:   user.AvatarUrl,
		},
		Households: households,
	}
	acc, err := h.auth.TelegramAccount(ctx, user.ID)
	if err != nil {
		return api.Me{}, err
	}
	if acc != nil {
		tg := toTelegramAccount(*acc)
		me.Telegram = &tg
	}
	return me, nil
}
