package cache

import "context"

type LuaScript struct {
	Name string
	SHA  string
	Body string
}

type LuaExecutor interface {
	Eval(ctx context.Context, script LuaScript, keys []string, args ...any) (any, error)
}
