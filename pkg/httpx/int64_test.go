package httpx

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestInt64AcceptsNumberAndString(t *testing.T) {
	for _, input := range []string{`9007199254740993`, `"9007199254740993"`} {
		var value Int64
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Fatalf("unmarshal %s: %v", input, err)
		}
		if value.Value() != 9007199254740993 {
			t.Fatalf("value = %d", value.Value())
		}
	}
}

func TestInt64SliceAcceptsMixedValues(t *testing.T) {
	var values Int64Slice
	if err := json.Unmarshal([]byte(`[1,"9007199254740993"]`), &values); err != nil {
		t.Fatal(err)
	}
	want := []int64{1, 9007199254740993}
	if !reflect.DeepEqual(values.Values(), want) {
		t.Fatalf("values = %v want %v", values, want)
	}
}
