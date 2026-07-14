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
