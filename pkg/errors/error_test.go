package errors

import "testing"

func TestCodeOf(t *testing.T) {
	err := Wrap(CodeInventoryNotEnough, "remain number is not enough", nil)
	if got := CodeOf(err); got != CodeInventoryNotEnough {
		t.Fatalf("CodeOf() = %s", got)
	}
	if status := HTTPStatus(CodeInventoryNotEnough); status != 422 {
		t.Fatalf("HTTPStatus() = %d", status)
	}
}

func TestCaptchaErrorHTTPStatuses(t *testing.T) {
	if got := HTTPStatus(CodeCaptchaRequired); got != 428 {
		t.Fatalf("captcha required status = %d", got)
	}
	if got := HTTPStatus(CodeCaptchaInvalid); got != 400 {
		t.Fatalf("captcha invalid status = %d", got)
	}
}
