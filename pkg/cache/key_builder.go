package cache

import (
	"fmt"
	"strings"
)

type KeyBuilder struct {
	prefix string
}

func NewKeyBuilder(prefix string) KeyBuilder {
	return KeyBuilder{prefix: strings.Trim(prefix, ":")}
}

func (b KeyBuilder) Build(parts ...any) string {
	values := make([]string, 0, len(parts)+1)
	if b.prefix != "" {
		values = append(values, b.prefix)
	}
	for _, part := range parts {
		text := strings.Trim(fmt.Sprint(part), ":")
		if text != "" {
			values = append(values, text)
		}
	}
	return strings.Join(values, ":")
}
