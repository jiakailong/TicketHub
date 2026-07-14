package memory

import (
	"context"

	"tickethub/app/base-data-service/internal/domain/base"
)

type BaseRepository struct {
	areas    []base.Area
	channels []base.ChannelData
}

func NewBaseRepository() BaseRepository {
	return BaseRepository{
		areas: []base.Area{
			{ID: 1, ParentID: 0, Name: "上海", Level: 1, Hot: true},
			{ID: 2, ParentID: 0, Name: "北京", Level: 1, Hot: true},
			{ID: 3, ParentID: 0, Name: "杭州", Level: 1, Hot: false},
		},
		channels: []base.ChannelData{
			{ID: 1, Code: "pay.alipay", Value: "支付宝"},
			{ID: 2, Code: "pay.wechat", Value: "微信支付"},
		},
	}
}

func (r BaseRepository) ListAreas(ctx context.Context, parentID int64) ([]base.Area, error) {
	out := make([]base.Area, 0)
	for _, item := range r.areas {
		if item.ParentID == parentID {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r BaseRepository) ListChannelData(ctx context.Context) ([]base.ChannelData, error) {
	return append([]base.ChannelData(nil), r.channels...), nil
}
