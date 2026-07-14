package errors

import "testing"

func TestGRPCErrorRoundTripPreservesBusinessCode(t *testing.T) {
	converted := ToGRPC(New(CodeInventoryNotEnough, "not enough inventory"))
	if CodeOf(converted) != CodeInventoryNotEnough {
		t.Fatalf("code = %s error=%v", CodeOf(converted), converted)
	}
	if HTTPStatus(CodeOf(converted)) != 422 {
		t.Fatalf("http status = %d", HTTPStatus(CodeOf(converted)))
	}
}
