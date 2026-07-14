package grpcapi

import (
	"context"

	basev1 "tickethub/api/proto/base/v1"
	"tickethub/app/base-data-service/internal/application"
)

type Server struct {
	basev1.UnimplementedBaseDataServiceServer
	base application.BaseQueryService
}

func NewServer(base application.BaseQueryService) Server {
	return Server{base: base}
}

func (s Server) ListAreas(ctx context.Context, req *basev1.ListAreasRequest) (*basev1.ListAreasReply, error) {
	items, err := s.base.ListAreas(ctx, req.GetParentId())
	if err != nil {
		return nil, err
	}
	reply := &basev1.ListAreasReply{Areas: make([]*basev1.Area, 0, len(items))}
	for _, item := range items {
		reply.Areas = append(reply.Areas, &basev1.Area{
			Id:       item.ID,
			ParentId: item.ParentID,
			Name:     item.Name,
			Level:    int32(item.Level),
			Hot:      item.Hot,
		})
	}
	return reply, nil
}

func (s Server) ListChannelData(ctx context.Context, req *basev1.ListChannelDataRequest) (*basev1.ListChannelDataReply, error) {
	items, err := s.base.ListChannelData(ctx)
	if err != nil {
		return nil, err
	}
	reply := &basev1.ListChannelDataReply{Channels: make([]*basev1.ChannelData, 0, len(items))}
	for _, item := range items {
		reply.Channels = append(reply.Channels, &basev1.ChannelData{Id: item.ID, Code: item.Code, Value: item.Value})
	}
	return reply, nil
}
