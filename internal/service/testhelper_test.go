package service

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"xinfeedsystem/internal/model/entity"
)

// newMiniRedis spins up an in-process Redis server for a single test.
func newMiniRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis start: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return rdb, mr
}

// makeVideo returns a Video with the given id and heat, with LikeCount == heat for convenience.
func makeVideo(id, heat int64) *entity.Video {
	return &entity.Video{
		ID:        id,
		Title:     "video-" + strconv.FormatInt(id, 10),
		Heat:      heat,
		LikeCount: int(heat),
		Status:    1,
	}
}

// seedVideoDetailCache writes video objects to Redis as video:detail:{id}.
func seedVideoDetailCache(t *testing.T, rdb *redis.Client, videos ...*entity.Video) {
	t.Helper()
	ctx := context.Background()
	for _, v := range videos {
		b, _ := json.Marshal(v)
		if err := rdb.Set(ctx, "video:detail:"+strconv.FormatInt(v.ID, 10), b, 0).Err(); err != nil {
			t.Fatalf("seed video:detail: %v", err)
		}
	}
}

// ─── stub videoStore ──────────────────────────────────────────────────────────

// stubVideoStore satisfies videoStore without a real database.
type stubVideoStore struct {
	popular   []*entity.Video
	likeCount []*entity.Video
	following []*entity.Video
	byID      map[int64]*entity.Video

	followingCalls int // incremented each time ListByFollowing is called
}

func (s *stubVideoStore) ListLatest(_ context.Context, _, _ int64, _ int) ([]*entity.Video, error) {
	return nil, nil
}

func (s *stubVideoStore) ListByFollowing(_ context.Context, _, _, _ int64, limit int) ([]*entity.Video, error) {
	s.followingCalls++
	if limit >= len(s.following) {
		return s.following, nil
	}
	return s.following[:limit], nil
}

func (s *stubVideoStore) ListByHeat(_ context.Context, _, _ int64, limit int) ([]*entity.Video, error) {
	if limit >= len(s.popular) {
		return s.popular, nil
	}
	return s.popular[:limit], nil
}

func (s *stubVideoStore) ListByLikeCount(_ context.Context, _, _ int64, limit int) ([]*entity.Video, error) {
	if limit >= len(s.likeCount) {
		return s.likeCount, nil
	}
	return s.likeCount[:limit], nil
}

func (s *stubVideoStore) FindByIDs(_ context.Context, ids []int64) ([]*entity.Video, error) {
	var out []*entity.Video
	for _, id := range ids {
		if v, ok := s.byID[id]; ok {
			out = append(out, v)
		}
	}
	return out, nil
}
