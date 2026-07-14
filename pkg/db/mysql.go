package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"tickethub/pkg/config"
	"tickethub/pkg/sharding"
)

func OpenMySQL(ctx context.Context, cfg config.MySQLConfig) (*sql.DB, error) {
	if cfg.DSN == "" {
		return nil, fmt.Errorf("mysql dsn is required")
	}
	client, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, err
	}
	if cfg.MaxOpenConns > 0 {
		client.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		client.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	client.SetConnMaxLifetime(30 * time.Minute)
	client.SetConnMaxIdleTime(10 * time.Minute)

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.PingContext(pingCtx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func OpenMySQLShardPool(ctx context.Context, cfg config.ShardingConfig) (*sharding.DBPool, error) {
	databases := make(map[string]*sql.DB, len(cfg.Databases))
	for name, databaseConfig := range cfg.Databases {
		client, err := OpenMySQL(ctx, databaseConfig)
		if err != nil {
			for _, opened := range databases {
				_ = opened.Close()
			}
			return nil, fmt.Errorf("open mysql shard %s: %w", name, err)
		}
		databases[name] = client
	}
	return sharding.NewDBPool(databases), nil
}
