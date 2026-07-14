package sharding

import (
	"fmt"
	"sync"
	"time"
)

type WriteMode string

const (
	WritePrimaryOnly WriteMode = "PRIMARY_ONLY"
	WriteDual        WriteMode = "DUAL_WRITE"
)

type ShardMapping struct {
	VirtualShard int
	Primary      Location
	Shadow       *Location
	WriteMode    WriteMode
	Version      int64
}

type MappingOrderRouter struct {
	mu          sync.RWMutex
	fallback    GeneOrderRouter
	mappings    map[int]ShardMapping
	refreshedAt time.Time
}

func NewMappingOrderRouter(fallback GeneOrderRouter) *MappingOrderRouter {
	return &MappingOrderRouter{fallback: fallback, mappings: make(map[int]ShardMapping)}
}

func (r *MappingOrderRouter) ReplaceMappings(mappings []ShardMapping) error {
	maximum := r.virtualShardCount()
	next := make(map[int]ShardMapping, len(mappings))
	for _, mapping := range mappings {
		if mapping.VirtualShard < 0 || mapping.VirtualShard >= maximum {
			return fmt.Errorf("virtual shard %d is outside [0,%d)", mapping.VirtualShard, maximum)
		}
		if mapping.Primary.Database == "" || mapping.Primary.Table == "" {
			return fmt.Errorf("virtual shard %d primary location is required", mapping.VirtualShard)
		}
		if mapping.WriteMode == "" {
			mapping.WriteMode = WritePrimaryOnly
		}
		if mapping.WriteMode != WritePrimaryOnly && mapping.WriteMode != WriteDual {
			return fmt.Errorf("virtual shard %d has unsupported write mode %s", mapping.VirtualShard, mapping.WriteMode)
		}
		if mapping.WriteMode == WriteDual && mapping.Shadow == nil {
			return fmt.Errorf("virtual shard %d shadow location is required for dual write", mapping.VirtualShard)
		}
		if mapping.Shadow != nil {
			shadow := *mapping.Shadow
			mapping.Shadow = &shadow
		}
		next[mapping.VirtualShard] = mapping
	}
	r.mu.Lock()
	r.mappings = next
	r.refreshedAt = time.Now()
	r.mu.Unlock()
	return nil
}

func (r *MappingOrderRouter) MappingFresh(maxAge time.Duration) bool {
	if maxAge <= 0 {
		return false
	}
	r.mu.RLock()
	refreshedAt := r.refreshedAt
	r.mu.RUnlock()
	return !refreshedAt.IsZero() && time.Since(refreshedAt) <= maxAge
}

func (r *MappingOrderRouter) RouteOrder(orderNumber int64, userID int64) Location {
	virtualShard, fallback := r.routeVirtualShard(orderNumber, userID)
	r.mu.RLock()
	mapping, ok := r.mappings[virtualShard]
	r.mu.RUnlock()
	if ok {
		return mapping.Primary
	}
	return fallback
}

func (r *MappingOrderRouter) RouteOrderWrites(orderNumber int64, userID int64) []Location {
	virtualShard, fallback := r.routeVirtualShard(orderNumber, userID)
	r.mu.RLock()
	mapping, ok := r.mappings[virtualShard]
	r.mu.RUnlock()
	if !ok {
		return []Location{fallback}
	}
	locations := []Location{mapping.Primary}
	if mapping.WriteMode == WriteDual && mapping.Shadow != nil {
		locations = append(locations, *mapping.Shadow)
	}
	return locations
}

func (r *MappingOrderRouter) PrimaryLocations() []Location {
	fallbackLocations := r.fallback.PrimaryLocations()
	r.mu.RLock()
	defer r.mu.RUnlock()
	locations := make([]Location, 0, len(fallbackLocations))
	for virtualShard, fallback := range fallbackLocations {
		if mapping, ok := r.mappings[virtualShard]; ok {
			locations = append(locations, mapping.Primary)
			continue
		}
		locations = append(locations, fallback)
	}
	return locations
}

func (r *MappingOrderRouter) Snapshot() []ShardMapping {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]ShardMapping, 0, len(r.mappings))
	for virtualShard := 0; virtualShard < r.virtualShardCount(); virtualShard++ {
		if mapping, ok := r.mappings[virtualShard]; ok {
			if mapping.Shadow != nil {
				shadow := *mapping.Shadow
				mapping.Shadow = &shadow
			}
			result = append(result, mapping)
		}
	}
	return result
}

func (r *MappingOrderRouter) routeVirtualShard(orderNumber int64, userID int64) (int, Location) {
	fallback := r.fallback.RouteOrder(orderNumber, userID)
	return fallback.DBIndex*r.tableCount() + fallback.TblIndex, fallback
}

func (r *MappingOrderRouter) virtualShardCount() int {
	databaseCount := r.fallback.DBCount
	if databaseCount <= 0 {
		databaseCount = 1
	}
	return databaseCount * r.tableCount()
}

func (r *MappingOrderRouter) tableCount() int {
	tableCount := r.fallback.TableCount
	if tableCount <= 0 {
		tableCount = 1
	}
	return tableCount
}
