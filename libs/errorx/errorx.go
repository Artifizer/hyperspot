package errorx

// This file contain typical errors that can be used by controller/model
// and associated Huma StatusErrors

import (
	"fmt"

	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// Error is the interface that all errors should implement
type Error interface {
	Error() string
	HumaError() huma.StatusError
}

// baseErrorxImpl is the base implementation of BaseError
type baseErrorImpl struct {
	err error
}

func (e *baseErrorImpl) Error() string { return e.err.Error() }

func NewErrorx(msg string, args ...any) Error {
	return &baseErrorImpl{err: fmt.Errorf(msg, args...)}
}

func (e *baseErrorImpl) HumaError() huma.StatusError {
	return huma.Error500InternalServerError(e.Error())
}

//
// Success responses (2xx)
//

// Accepted (202)

type StatusAccepted struct {
	baseErrorImpl
}

func NewStatusAccepted(msg string, args ...any) Error {
	return &StatusAccepted{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *StatusAccepted) HumaError() huma.StatusError {
	return huma.NewError(http.StatusAccepted, e.Error())
}

//
// Client errors (4xx)
//

// Bad request (400)

type ErrBadRequest struct {
	baseErrorImpl
}

func NewErrBadRequest(msg string, args ...any) Error {
	return &ErrBadRequest{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrBadRequest) HumaError() huma.StatusError {
	return huma.Error400BadRequest(e.Error())
}

// Forbidden (403)

type ErrForbidden struct {
	baseErrorImpl
}

func NewErrForbidden(msg string, args ...any) Error {
	return &ErrForbidden{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrForbidden) HumaError() huma.StatusError {
	return huma.Error403Forbidden(e.Error())
}

// Not found (404)

type ErrNotFound struct {
	baseErrorImpl
}

func NewErrNotFound(msg string, args ...any) Error {
	return &ErrNotFound{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrNotFound) HumaError() huma.StatusError {
	return huma.Error404NotFound(e.Error())
}

// Method not allowed (405)

type ErrMethodNotAllowed struct {
	baseErrorImpl
}

func NewErrMethodNotAllowed(msg string, args ...any) Error {
	return &ErrMethodNotAllowed{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrMethodNotAllowed) HumaError() huma.StatusError {
	return huma.Error405MethodNotAllowed(e.Error())
}

// Conflict (409)

type ErrConflict struct {
	baseErrorImpl
}

func NewErrConflict(msg string, args ...any) Error {
	return &ErrConflict{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrConflict) HumaError() huma.StatusError {
	return huma.Error409Conflict(e.Error())
}

//
// Server errors (5xx)
//

// Internal server error (500)

type ErrInternalServerError struct {
	baseErrorImpl
}

func NewErrInternalServerError(msg string, args ...any) Error {
	return &ErrInternalServerError{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrInternalServerError) HumaError() huma.StatusError {
	return huma.Error500InternalServerError(e.Error())
}

// Not implemented (501)

type ErrNotImplemented struct {
	baseErrorImpl
}

func NewErrNotImplemented(msg string, args ...any) Error {
	return &ErrNotImplemented{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrNotImplemented) HumaError() huma.StatusError {
	return huma.Error501NotImplemented(e.Error())
}
