package core

import "errors"

var (
	// ErrUserNotFound is returned when a user is not found.
	ErrUserNotFound = errors.New("user not found")

	// ErrTemporarilyUnavailable is returned when a service is temporarily unavailable.
	ErrTemporarilyUnavailable = errors.New("temporarily unavailable")

	// ErrNotImplemented is returned when a feature is not implemented.
	ErrNotImplemented = errors.New("not implemented")

	// ErrMaxKdCallAttemptsExceeded is returned when the maximum number of datacener call attempts is exceeded.
	ErrMaxKdCallAttemptsExceeded = errors.New("max kd call attempts exceeded")

	// ErrUserIsBlocked is returned when a user is blocked.
	ErrUserIsBlocked = errors.New("user is blocked")
)
