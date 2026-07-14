package idgen

import (
	"fmt"
	"sync"
)

const (
	UserGeneBits        = uint(10)
	UserGeneMask        = int64(1<<UserGeneBits - 1)
	orderSequenceBits   = uint(9)
	orderNodeBits       = uint(4)
	orderSequenceMask   = int64(1<<orderSequenceBits - 1)
	orderNodeMask       = int64(1<<orderNodeBits - 1)
	orderTimestampBits  = uint(40)
	orderTimestampMask  = int64(1<<orderTimestampBits - 1)
	orderSequenceShift  = UserGeneBits
	orderNodeShift      = orderSequenceShift + orderSequenceBits
	orderTimestampShift = orderNodeShift + orderNodeBits
)

type OrderNumberGenerator struct {
	mu       sync.Mutex
	ids      *Snowflake
	lastMS   int64
	sequence int64
}

func NewOrderNumberGenerator(ids *Snowflake) *OrderNumberGenerator {
	return &OrderNumberGenerator{ids: ids}
}

func (g *OrderNumberGenerator) NextOrderNumber(userID int64) (int64, error) {
	if g == nil || g.ids == nil {
		return 0, fmt.Errorf("snowflake generator is required")
	}
	if g.ids.nodeID > orderNodeMask {
		return 0, fmt.Errorf("order number node id must be between 0 and %d", orderNodeMask)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.ids.nowMS()
	if now < g.lastMS {
		return 0, fmt.Errorf("clock moved backwards: now=%d last=%d", now, g.lastMS)
	}
	if now == g.lastMS {
		g.sequence++
		if g.sequence > orderSequenceMask {
			now = g.ids.waitNextMillis(now)
			g.sequence = 0
		}
	} else {
		g.sequence = 0
	}
	g.lastMS = now

	timestamp := now - g.ids.epochMS
	if timestamp < 0 || timestamp > orderTimestampMask {
		return 0, fmt.Errorf("order number timestamp is outside the supported range: %d", timestamp)
	}
	return (timestamp << orderTimestampShift) |
		(g.ids.nodeID << orderNodeShift) |
		(g.sequence << orderSequenceShift) |
		UserGene(userID), nil
}

func UserGene(userID int64) int64 {
	if userID < 0 {
		userID = -userID
	}
	return userID & UserGeneMask
}

func ExtractUserGene(orderNumber int64) int64 {
	return orderNumber & UserGeneMask
}
