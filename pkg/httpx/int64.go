package httpx

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// Int64 accepts both JSON numbers and quoted decimal strings. Public APIs use
// quoted IDs because Snowflake values exceed JavaScript's safe integer range.
type Int64 int64

func (v *Int64) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*v = 0
		return nil
	}
	if data[0] == '"' {
		var raw string
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return err
		}
		*v = Int64(parsed)
		return nil
	}
	parsed, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return err
	}
	*v = Int64(parsed)
	return nil
}

func (v Int64) Value() int64 {
	return int64(v)
}

type Int64Slice []int64

func (v *Int64Slice) UnmarshalJSON(data []byte) error {
	var values []json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	result := make([]int64, 0, len(values))
	for _, raw := range values {
		var item Int64
		if err := item.UnmarshalJSON(raw); err != nil {
			return err
		}
		result = append(result, item.Value())
	}
	*v = result
	return nil
}

func (v Int64Slice) Values() []int64 {
	return []int64(v)
}
