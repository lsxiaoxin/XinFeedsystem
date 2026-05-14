package service

import (
	"context"

	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
	"xinfeedsystem/pkg/cursor"
)

type FeedService struct {
	fetchers map[string]FeedFetcher
}

// NewFeedService 接收任意数量的 FeedFetcher，每种类型注册一次。
func NewFeedService(fetchers ...FeedFetcher) *FeedService {
	s := &FeedService{fetchers: make(map[string]FeedFetcher, len(fetchers))}
	for _, f := range fetchers {
		s.fetchers[f.Type()] = f
	}
	return s
}

func (s *FeedService) GetFeed(ctx context.Context, req *dto.FeedRequest) (*dto.FeedResponse, error) {
	fetcher, ok := s.fetchers[req.Type]
	if !ok {
		return nil, errcode.New(errcode.InvalidParam)
	}

	score, cursorID, err := cursor.Decode(req.Cursor)
	if err != nil {
		return nil, errcode.New(errcode.InvalidParam)
	}

	limit := normalizeLimit(req.Limit)
	// 多取 1 条用于判断 hasMore，避免再发一次 count 查询
	videos, err := fetcher.Fetch(ctx, score, cursorID, limit+1)
	if err != nil {
		return nil, err
	}

	hasMore := len(videos) > limit
	if hasMore {
		videos = videos[:limit]
	}

	vos := make([]*dto.VideoVO, len(videos))
	for i, v := range videos {
		vos[i] = dto.ToVideoVO(v)
	}

	resp := &dto.FeedResponse{Videos: vos, HasMore: hasMore}
	if hasMore && len(videos) > 0 {
		last := videos[len(videos)-1]
		// 由 fetcher 决定该策略下的 score 字段，FeedService 无需感知
		resp.NextCursor = cursor.Encode(fetcher.ScoreOf(last), last.ID)
	}
	return resp, nil
}
