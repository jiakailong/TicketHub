package migrate

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type TaskStatus string

const (
	TaskPending TaskStatus = "PENDING"
	TaskRunning TaskStatus = "RUNNING"
	TaskPaused  TaskStatus = "PAUSED"
	TaskDone    TaskStatus = "DONE"
	TaskFailed  TaskStatus = "FAILED"
)

type MigrationTask struct {
	ID                int64      `json:"id"`
	VirtualShard      int        `json:"virtual_shard"`
	SourceShard       string     `json:"source_shard"`
	TargetShard       string     `json:"target_shard"`
	Status            TaskStatus `json:"status"`
	BatchSize         int        `json:"batch_size"`
	CursorOrderNumber int64      `json:"cursor_order_number"`
	CopiedRows        int64      `json:"copied_rows"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

type ShardMapping struct {
	VirtualShard  int    `json:"virtual_shard"`
	PhysicalDB    string `json:"physical_db"`
	PhysicalTable string `json:"physical_table"`
	ShadowDB      string `json:"shadow_db,omitempty"`
	ShadowTable   string `json:"shadow_table,omitempty"`
	WriteMode     string `json:"write_mode"`
	Version       int64  `json:"version"`
}

var shardNamePattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func ParseShardLocation(value string) (database string, table string, err error) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) != 2 || !shardNamePattern.MatchString(parts[0]) || !shardNamePattern.MatchString(parts[1]) {
		return "", "", fmt.Errorf("shard location must use database.table")
	}
	return parts[0], parts[1], nil
}
