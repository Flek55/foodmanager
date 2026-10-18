package telegram

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidInitData = errors.New("invalid Telegram init data")

// WebAppUser is who opened the Mini App, as vouched for by Telegram.
type WebAppUser struct {
	User
	AuthDate time.Time
}

// ValidateInitData checks the signature of Mini App init data
// (window.Telegram.WebApp.initData) and returns the user it describes.
//
// Telegram signs the data with a key derived from the bot token, so a valid
// signature proves Telegram issued it to this user for our bot:
//
//	secret = HMAC-SHA256(key="WebAppData", msg=botToken)
//	hash   = hex(HMAC-SHA256(key=secret, msg=dataCheckString))
//
// where dataCheckString is every field except "hash", sorted, as
// "key=value" lines. maxAge bounds replay of a leaked init string.
// https://core.telegram.org/bots/webapps#validating-data-received-via-the-mini-app
func ValidateInitData(initData, botToken string, maxAge time.Duration, now time.Time) (WebAppUser, error) {
	values, err := url.ParseQuery(initData)
	if err != nil {
		return WebAppUser{}, ErrInvalidInitData
	}
	gotHash := values.Get("hash")
	if gotHash == "" {
		return WebAppUser{}, ErrInvalidInitData
	}

	want := signInitData(values, botToken)
	got, err := hex.DecodeString(gotHash)
	if err != nil || !hmac.Equal(got, want) {
		return WebAppUser{}, ErrInvalidInitData
	}

	authUnix, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
	if err != nil {
		return WebAppUser{}, ErrInvalidInitData
	}
	authDate := time.Unix(authUnix, 0)
	if now.Sub(authDate) > maxAge {
		return WebAppUser{}, ErrInvalidInitData
	}

	var u User
	if err := json.Unmarshal([]byte(values.Get("user")), &u); err != nil || u.ID == 0 {
		return WebAppUser{}, ErrInvalidInitData
	}
	return WebAppUser{User: u, AuthDate: authDate}, nil
}

func signInitData(values url.Values, botToken string) []byte {
	keys := make([]string, 0, len(values))
	for k := range values {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	lines := make([]string, len(keys))
	for i, k := range keys {
		lines[i] = k + "=" + values.Get(k)
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(lines, "\n")))
	return mac.Sum(nil)
}

// SignInitData builds signed init data the way Telegram does. Used by
// tests to play the role of Telegram.
func SignInitData(values url.Values, botToken string) string {
	v := url.Values{}
	for k, vs := range values {
		v[k] = vs
	}
	v.Del("hash")
	v.Set("hash", hex.EncodeToString(signInitData(v, botToken)))
	return v.Encode()
}
