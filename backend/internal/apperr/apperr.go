// Package apperr defines domain errors that carry a stable machine-readable
// code and a user-facing (Russian) message. Services return them; the HTTP
// layer maps Kind to a status code in one place.
package apperr

import "errors"

type Kind int

const (
	KindInvalid Kind = iota + 1
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
	KindTooManyRequests
)

type Error struct {
	Kind    Kind
	Code    string
	Message string
	// Field names the request field a validation error refers to, if any.
	Field string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func Invalid(field, message string) *Error {
	return &Error{Kind: KindInvalid, Code: "invalid", Message: message, Field: field}
}

func Unauthorized(message string) *Error {
	return &Error{Kind: KindUnauthorized, Code: "unauthorized", Message: message}
}

func Forbidden(message string) *Error {
	return &Error{Kind: KindForbidden, Code: "forbidden", Message: message}
}

func NotFound(message string) *Error {
	return &Error{Kind: KindNotFound, Code: "not_found", Message: message}
}

// NotFoundCode is NotFound with a specific code, for when clients must tell
// apart what is missing (e.g. an item vs. access to the whole household).
func NotFoundCode(code, message string) *Error {
	return &Error{Kind: KindNotFound, Code: code, Message: message}
}

// CodeHouseholdNotFound means the household doesn't exist or the user isn't
// a member (we deliberately don't say which).
const CodeHouseholdNotFound = "household_not_found"

func Conflict(code, message string) *Error {
	return &Error{Kind: KindConflict, Code: code, Message: message}
}

func TooManyRequests() *Error {
	return &Error{Kind: KindTooManyRequests, Code: "rate_limited", Message: "слишком много попыток, подождите минуту"}
}

// As extracts an *Error from err's chain.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}
