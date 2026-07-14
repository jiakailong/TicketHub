package application

import (
	"context"

	"tickethub/app/base-data-service/internal/domain/base"
)

type BaseRepository interface {
	ListAreas(ctx context.Context, parentID int64) ([]base.Area, error)
	ListChannelData(ctx context.Context) ([]base.ChannelData, error)
}

type BaseQueryService struct {
	repo BaseRepository
}

func NewBaseQueryService(repo BaseRepository) BaseQueryService {
	return BaseQueryService{repo: repo}
}

func (s BaseQueryService) ListAreas(ctx context.Context, parentID int64) ([]base.Area, error) {
	return s.repo.ListAreas(ctx, parentID)
}

func (s BaseQueryService) ListChannelData(ctx context.Context) ([]base.ChannelData, error) {
	return s.repo.ListChannelData(ctx)
}
