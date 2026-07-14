package errors

import (
	stderrors "errors"
	"fmt"
)

type AppError struct {
	Code    Code
	Message string
	Cause   error
}

func New(code Code, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

func Wrap(code Code, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, Cause: cause}
}

func (e *AppError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
}

func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func CodeOf(err error) Code {
	if err == nil {
		return CodeOK
	}
	var appErr *AppError
	if stderrors.As(err, &appErr) {
		return appErr.Code
	}
	if code, ok := codeFromGRPC(err); ok {
		return code
	}
	return CodeInternal
}

func IsCode(err error, code Code) bool {
	return CodeOf(err) == code
}
