package sharding

import (
	"database/sql"
	"fmt"
	"time"
)

type Location struct {
	Database string
	Table    string
	DBIndex  int
	TblIndex int
}

type OrderRouter interface {
	RouteOrder(orderNumber int64, userID int64) Location
}

type OrderWriteRouter interface {
	RouteOrderWrites(orderNumber int64, userID int64) []Location
}

type LocationCatalog interface {
	PrimaryLocations() []Location
}

type MappingFreshness interface {
	MappingFresh(maxAge time.Duration) bool
}

const DefaultMappingMaxStaleness = 15 * time.Second

type DatabaseResolver interface {
	Resolve(database string) (*sql.DB, error)
}

type DBPool struct {
	databases map[string]*sql.DB
}

func NewDBPool(databases map[string]*sql.DB) *DBPool {
	copyOfDatabases := make(map[string]*sql.DB, len(databases))
	for name, database := range databases {
		copyOfDatabases[name] = database
	}
	return &DBPool{databases: copyOfDatabases}
}

func (p *DBPool) Resolve(database string) (*sql.DB, error) {
	if p == nil {
		return nil, fmt.Errorf("shard database pool is not configured")
	}
	databaseClient, ok := p.databases[database]
	if !ok || databaseClient == nil {
		return nil, fmt.Errorf("shard database %s is not configured", database)
	}
	return databaseClient, nil
}

func (p *DBPool) Close() error {
	if p == nil {
		return nil
	}
	var firstErr error
	for _, database := range p.databases {
		if err := database.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
