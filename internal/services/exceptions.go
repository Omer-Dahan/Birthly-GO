// Package services holds business logic. It must not import the Telegram bot
// library — bot/ depends on services/, never the reverse (enforced by
// package boundaries here, where Python needed test_architecture.py to
// enforce it at test time).
package services

// NotFoundErr is returned when a requested entity doesn't exist or doesn't
// belong to the caller.
type NotFoundErr struct{ Message string }

func (e *NotFoundErr) Error() string { return e.Message }

// LimitErr is returned when a user-facing quota (events, reminder rules,
// templates, ...) is exceeded.
type LimitErr struct{ Message string }

func (e *LimitErr) Error() string { return e.Message }
