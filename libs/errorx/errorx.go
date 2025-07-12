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
	GetStatus() int
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

func (e *baseErrorImpl) GetStatus() int {
	return http.StatusInternalServerError
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

func (e *StatusAccepted) GetStatus() int {
	return http.StatusAccepted
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

func (e *ErrBadRequest) GetStatus() int {
	return http.StatusBadRequest
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

func (e *ErrForbidden) GetStatus() int {
	return http.StatusForbidden
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

func (e *ErrNotFound) GetStatus() int {
	return http.StatusNotFound
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

func (e *ErrMethodNotAllowed) GetStatus() int {
	return http.StatusMethodNotAllowed
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

func (e *ErrConflict) GetStatus() int {
	return http.StatusConflict
}

// PayloadTooLarge (413)

type ErrPayloadTooLarge struct {
	baseErrorImpl
}

func NewErrPayloadTooLarge(msg string, args ...any) Error {
	return &ErrPayloadTooLarge{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrPayloadTooLarge) HumaError() huma.StatusError {
	return huma.NewError(http.StatusRequestEntityTooLarge, e.Error())
}

func (e *ErrPayloadTooLarge) GetStatus() int {
	return http.StatusRequestEntityTooLarge
}

// Unsupported Media Type (415)

type ErrUnsupportedMediaType struct {
	baseErrorImpl
}

func NewErrUnsupportedMediaType(msg string, args ...any) Error {
	return &ErrUnsupportedMediaType{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrUnsupportedMediaType) HumaError() huma.StatusError {
	return huma.Error415UnsupportedMediaType(e.Error())
}

func (e *ErrUnsupportedMediaType) GetStatus() int {
	return http.StatusUnsupportedMediaType
}

// Locked (423)

type ErrLocked struct {
	baseErrorImpl
}

func NewErrLocked(msg string, args ...any) Error {
	return &ErrLocked{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrLocked) HumaError() huma.StatusError {
	return huma.NewError(http.StatusLocked, e.Error())
}

func (e *ErrLocked) GetStatus() int {
	return http.StatusLocked
}

// Canceled (499)

type ErrCanceled struct {
	baseErrorImpl
}

func NewErrCanceled(msg string, args ...any) Error {
	return &ErrCanceled{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrCanceled) HumaError() huma.StatusError {
	return huma.NewError(499, e.Error())
}

func (e *ErrCanceled) GetStatus() int {
	return 499
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

func (e *ErrInternalServerError) GetStatus() int {
	return http.StatusInternalServerError
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

func (e *ErrNotImplemented) GetStatus() int {
	return http.StatusNotImplemented
}

// Timeout (504)

type ErrTimeout struct {
	baseErrorImpl
}

func NewErrTimeout(msg string, args ...any) Error {
	return &ErrTimeout{baseErrorImpl{err: fmt.Errorf(msg, args...)}}
}

func (e *ErrTimeout) HumaError() huma.StatusError {
	return huma.NewError(http.StatusGatewayTimeout, e.Error())
}

func (e *ErrTimeout) GetStatus() int {
	return http.StatusGatewayTimeout
}
