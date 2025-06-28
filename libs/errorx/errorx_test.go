package errorx

import (
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewErrorx(t *testing.T) {
	msg := "test error message"
	err := NewErrorx(msg)

	assert.Equal(t, msg, err.Error())

	humaErr := err.HumaError()
	assert.Equal(t, 500, humaErr.GetStatus())
	assert.Equal(t, msg, humaErr.Error())
}

func TestNewErrNotFound(t *testing.T) {
	msg := "resource not found"
	err := NewErrNotFound(msg)

	assert.Equal(t, msg, err.Error())

	humaErr := err.HumaError()
	assert.Equal(t, 404, humaErr.GetStatus())
	assert.Equal(t, msg, humaErr.Error())
}

func TestNewErrForbidden(t *testing.T) {
	msg := "access forbidden"
	err := NewErrForbidden(msg)

	assert.Equal(t, msg, err.Error())

	humaErr := err.HumaError()
	assert.Equal(t, 403, humaErr.GetStatus())
	assert.Equal(t, msg, humaErr.Error())
}

func TestNewErrBadRequest(t *testing.T) {
	msg := "bad request"
	err := NewErrBadRequest(msg)

	assert.Equal(t, msg, err.Error())

	humaErr := err.HumaError()
	assert.Equal(t, 400, humaErr.GetStatus())
	assert.Equal(t, msg, humaErr.Error())
}

func TestNewErrInternalServerError(t *testing.T) {
	msg := "internal server error"
	err := NewErrInternalServerError(msg)

	assert.Equal(t, msg, err.Error())

	humaErr := err.HumaError()
	assert.Equal(t, 500, humaErr.GetStatus())
	assert.Equal(t, msg, humaErr.Error())
}

func TestNewErrConflict(t *testing.T) {
	msg := "resource conflict"
	err := NewErrConflict(msg)

	assert.Equal(t, msg, err.Error())

	humaErr := err.HumaError()
	assert.Equal(t, 409, humaErr.GetStatus())
	assert.Equal(t, msg, humaErr.Error())
}

func TestNewErrMethodNotAllowed(t *testing.T) {
	msg := "method not allowed"
	err := NewErrMethodNotAllowed(msg)

	assert.Equal(t, msg, err.Error())

	humaErr := err.HumaError()
	assert.Equal(t, 405, humaErr.GetStatus())
	assert.Equal(t, msg, humaErr.Error())
}

func TestErrorInterface(t *testing.T) {
	// Test that all error types implement the Error interface
	var _ Error = &baseErrorImpl{}
	var _ Error = &ErrNotFound{}
	var _ Error = &ErrForbidden{}
	var _ Error = &ErrBadRequest{}
	var _ Error = &ErrInternalServerError{}
	var _ Error = &ErrConflict{}
	var _ Error = &ErrMethodNotAllowed{}
}

func TestHumaErrorInterface(t *testing.T) {
	// Test that all error types return a huma.StatusError
	var _ huma.StatusError = NewErrorx("test").HumaError()
	var _ huma.StatusError = NewErrNotFound("test").HumaError()
	var _ huma.StatusError = NewErrForbidden("test").HumaError()
	var _ huma.StatusError = NewErrBadRequest("test").HumaError()
	var _ huma.StatusError = NewErrInternalServerError("test").HumaError()
	var _ huma.StatusError = NewErrConflict("test").HumaError()
	var _ huma.StatusError = NewErrMethodNotAllowed("test").HumaError()
}
