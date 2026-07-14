package grpcapi

import (
	"context"

	customizev1 "tickethub/api/proto/customize/v1"
	"tickethub/app/customize-service/internal/application"
	"tickethub/app/customize-service/internal/domain/customize"
)

type Server struct {
	customizev1.UnimplementedCustomizeServiceServer
	messages application.MessageRecordService
}

func NewServer(messages application.MessageRecordService) Server {
	return Server{messages: messages}
}

func (s Server) RecordMessage(ctx context.Context, req *customizev1.RecordMessageRequest) (*customizev1.RecordMessageReply, error) {
	if err := s.messages.Save(ctx, customize.MessageRecord{
		MessageID: req.GetMessageId(),
		Topic:     req.GetTopic(),
		Status:    customize.MessageStatus(req.GetStatus()),
	}); err != nil {
		return nil, err
	}
	return &customizev1.RecordMessageReply{Success: true}, nil
}
