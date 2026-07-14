package observability

import "sync"

type Counter struct {
	mu    sync.Mutex
	value int64
}

func (c *Counter) Inc() {
	c.Add(1)
}

func (c *Counter) Add(delta int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.value += delta
}

func (c *Counter) Value() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}
