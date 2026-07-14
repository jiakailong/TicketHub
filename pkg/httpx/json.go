package httpx

import (
	"encoding/json"
	"net/http"

	therrors "tickethub/pkg/errors"
)

type Response struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func DecodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return therrors.Wrap(therrors.CodeInvalidArgument, "invalid json body", err)
	}
	return nil
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func WriteOK(w http.ResponseWriter, data any) {
	WriteJSON(w, http.StatusOK, Response{
		Code:    string(therrors.CodeOK),
		Message: "OK",
		Data:    data,
	})
}

func WriteError(w http.ResponseWriter, err error) {
	code := therrors.CodeOf(err)
	WriteJSON(w, therrors.HTTPStatus(code), Response{
		Code:    string(code),
		Message: therrors.PublicMessage(code),
	})
}

func InvalidArgument(message string) error {
	return therrors.New(therrors.CodeInvalidArgument, message)
}

func Unauthenticated(message string) error {
	return therrors.New(therrors.CodeUnauthenticated, message)
}

func Forbidden(message string) error {
	return therrors.New(therrors.CodeForbidden, message)
}
