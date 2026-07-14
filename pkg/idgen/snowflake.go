package idgen

import (
	"fmt"
	"sync"
	"time"
)

const (
	defaultEpochMS = int64(1704067200000) // 2024-01-01T00:00:00Z
	nodeIDBits     = uint(10)
	sequenceBits   = uint(12)
	maxNodeID      = int64(1<<nodeIDBits - 1)
	sequenceMask   = int64(1<<sequenceBits - 1)
)

type Clock func() time.Time

type Snowflake struct {
	mu        sync.Mutex
	epochMS   int64
	nodeID    int64
	lastMS    int64
	sequence  int64
	clock     Clock
	waitSleep time.Duration
}

func NewSnowflake(nodeID int64) (*Snowflake, error) {
	return NewSnowflakeWithClock(nodeID, time.Now)
}

func NewSnowflakeWithClock(nodeID int64, clock Clock) (*Snowflake, error) {
	if nodeID < 0 || nodeID > maxNodeID {
		return nil, fmt.Errorf("node id must be between 0 and %d", maxNodeID)
	}
	if clock == nil {
		clock = time.Now
	}
	return &Snowflake{
		epochMS:   defaultEpochMS,
		nodeID:    nodeID,
		clock:     clock,
		waitSleep: time.Millisecond,
	}, nil
}

func (g *Snowflake) NextID() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.nowMS()
	if now < g.lastMS {
		return 0, fmt.Errorf("clock moved backwards: now=%d last=%d", now, g.lastMS)
	}
	if now == g.lastMS {
		g.sequence = (g.sequence + 1) & sequenceMask
		if g.sequence == 0 {
			now = g.waitNextMillis(now)
		}
	} else {
		g.sequence = 0
	}
	g.lastMS = now

	return ((now - g.epochMS) << (nodeIDBits + sequenceBits)) |
		(g.nodeID << sequenceBits) |
		g.sequence, nil
}

func (g *Snowflake) nowMS() int64 {
	return g.clock().UnixNano() / int64(time.Millisecond)
}

func (g *Snowflake) waitNextMillis(last int64) int64 {
	now := g.nowMS()
	for now <= last {
		time.Sleep(g.waitSleep)
		now = g.nowMS()
	}
	return now
}
