package service

import (
	"context"

	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/dto"
)

type FeedService struct {
	fetchers map[string]FeedFetcher
}

// NewFeedService accepts any number of FeedFetcher implementations.
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

	limit := normalizeLimit(req.Limit)
	videos, nextCursor, hasMore, err := fetcher.Fetch(ctx, req.Cursor, limit)
	if err != nil {
		return nil, err
	}

	vos := make([]*dto.VideoVO, len(videos))
	for i, v := range videos {
		vos[i] = dto.ToVideoVO(v)
	}
	return &dto.FeedResponse{Videos: vos, HasMore: hasMore, NextCursor: nextCursor}, nil
}
