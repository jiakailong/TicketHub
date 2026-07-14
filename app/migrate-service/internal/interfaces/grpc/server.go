package grpcapi

import (
	"context"

	migratev1 "tickethub/api/proto/migrate/v1"
	"tickethub/app/migrate-service/internal/application"
	"tickethub/app/migrate-service/internal/domain/migrate"
)

type IDGenerator interface {
	NextID() (int64, error)
}

type Server struct {
	migratev1.UnimplementedMigrateServiceServer
	migrations application.MigrationService
	ids        IDGenerator
}

func NewServer(migrations application.MigrationService, ids IDGenerator) Server {
	return Server{migrations: migrations, ids: ids}
}

func (s Server) CreateTask(ctx context.Context, req *migratev1.CreateTaskRequest) (*migratev1.CreateTaskReply, error) {
	id, err := s.ids.NextID()
	if err != nil {
		return nil, err
	}
	batchSize := int(req.GetBatchSize())
	if batchSize <= 0 {
		batchSize = 500
	}
	if err := s.migrations.SaveTask(ctx, migrate.MigrationTask{
		ID:           id,
		VirtualShard: int(req.GetVirtualShard()),
		SourceShard:  req.GetSourceShard(),
		TargetShard:  req.GetTargetShard(),
		Status:       migrate.TaskPending,
		BatchSize:    batchSize,
	}); err != nil {
		return nil, err
	}
	return &migratev1.CreateTaskReply{TaskId: id, VirtualShard: req.GetVirtualShard(), Status: string(migrate.TaskPending)}, nil
}

func (s Server) ListShardMappings(ctx context.Context, req *migratev1.ListShardMappingsRequest) (*migratev1.ListShardMappingsReply, error) {
	mappings, err := s.migrations.LoadShardMappings(ctx)
	if err != nil {
		return nil, err
	}
	reply := &migratev1.ListShardMappingsReply{Mappings: make([]*migratev1.ShardMapping, 0, len(mappings))}
	for _, mapping := range mappings {
		reply.Mappings = append(reply.Mappings, &migratev1.ShardMapping{
			VirtualShard:  int32(mapping.VirtualShard),
			PhysicalDb:    mapping.PhysicalDB,
			PhysicalTable: mapping.PhysicalTable,
			ShadowDb:      mapping.ShadowDB,
			ShadowTable:   mapping.ShadowTable,
			WriteMode:     mapping.WriteMode,
			Version:       mapping.Version,
		})
	}
	return reply, nil
}

func (s Server) ResumeTask(ctx context.Context, req *migratev1.ResumeTaskRequest) (*migratev1.ResumeTaskReply, error) {
	if err := s.migrations.ResumeTask(ctx, req.GetTaskId()); err != nil {
		return nil, err
	}
	return &migratev1.ResumeTaskReply{TaskId: req.GetTaskId(), Status: string(migrate.TaskRunning)}, nil
}
