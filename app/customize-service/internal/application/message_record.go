package application

import (
	"context"

	"tickethub/app/customize-service/internal/domain/customize"
)

type MessageRecordRepository interface {
	Save(ctx context.Context, record customize.MessageRecord) error
	MarkFailed(ctx context.Context, messageID string, topic string, reason string) error
}

type MessageRecordService struct {
	repo MessageRecordRepository
}

func NewMessageRecordService(repo MessageRecordRepository) MessageRecordService {
	return MessageRecordService{repo: repo}
}

func (s MessageRecordService) Save(ctx context.Context, record customize.MessageRecord) error {
	return s.repo.Save(ctx, record)
}

func (s MessageRecordService) MarkFailed(ctx context.Context, messageID string, topic string, reason string) error {
	return s.repo.MarkFailed(ctx, messageID, topic, reason)
}
