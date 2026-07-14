package order

import (
	"testing"
	"time"

	therrors "tickethub/pkg/errors"
)

func TestOrderStateTransitions(t *testing.T) {
	o := New(1, 2, 3, 100, time.Now())
	if err := o.Cancel(time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := o.MarkPaid(time.Now()); !therrors.IsCode(err, therrors.CodeOrderStateConflict) {
		t.Fatalf("expected state conflict, got %v", err)
	}
	if err := o.MarkRefund(time.Now()); err != nil {
		t.Fatal(err)
	}
	if o.Status != StatusRefund {
		t.Fatalf("status = %s", o.Status)
	}
}
