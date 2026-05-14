package service

import (
	"context"
	"strconv"
	"testing"

	"xinfeedsystem/internal/model/entity"
)

func TestSnapshotService_Rebuild_PopulatesZSetAndCurrentPointer(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := context.Background()

	videos := []*videoSeed{
		{id: 1, heat: 900},
		{id: 2, heat: 800},
		{id: 3, heat: 700},
	}
	store := &stubVideoStore{popular: makeVideos(videos)}
	svc := NewSnapshotService(store, rdb)

	svc.rebuild(ctx, "popularity")

	// current pointer must be set
	epochStr, err := rdb.Get(ctx, "snapshot:popularity:current").Result()
	if err != nil {
		t.Fatalf("current pointer not set: %v", err)
	}
	epoch, _ := strconv.ParseInt(epochStr, 10, 64)
	if epoch == 0 {
		t.Fatal("epoch should be non-zero")
	}

	// ZSet must contain all 3 members with correct scores
	snapKey := "snapshot:popularity:v" + epochStr
	members, err := rdb.ZRevRangeWithScores(ctx, snapKey, 0, -1).Result()
	if err != nil {
		t.Fatalf("ZREVRANGE: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}

	expected := []struct {
		id    string
		score float64
	}{
		{"1", 900}, {"2", 800}, {"3", 700},
	}
	for i, m := range members {
		if m.Member != expected[i].id {
			t.Errorf("rank %d: expected member %s, got %s", i, expected[i].id, m.Member)
		}
		if m.Score != expected[i].score {
			t.Errorf("rank %d: expected score %.0f, got %.0f", i, expected[i].score, m.Score)
		}
	}
}

func TestSnapshotService_Rebuild_EmptyVideos_DoesNotSetCurrentPointer(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := context.Background()

	store := &stubVideoStore{popular: nil}
	svc := NewSnapshotService(store, rdb)
	svc.rebuild(ctx, "popularity")

	_, err := rdb.Get(ctx, "snapshot:popularity:current").Result()
	if err == nil {
		t.Fatal("current pointer should not be set when there are no videos")
	}
}

func TestSnapshotService_Rebuild_LikeCount(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := context.Background()

	videos := []*videoSeed{{id: 10, heat: 5}, {id: 20, heat: 3}}
	store := &stubVideoStore{likeCount: makeVideos(videos)}
	svc := NewSnapshotService(store, rdb)
	svc.rebuild(ctx, "like_count")

	epochStr, _ := rdb.Get(ctx, "snapshot:like_count:current").Result()
	count, _ := rdb.ZCard(ctx, "snapshot:like_count:v"+epochStr).Result()
	if count != 2 {
		t.Fatalf("expected 2 members in like_count ZSet, got %d", count)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

type videoSeed struct{ id, heat int64 }

func makeVideos(seeds []*videoSeed) []*entity.Video {
	out := make([]*entity.Video, len(seeds))
	for i, s := range seeds {
		out[i] = makeVideo(s.id, s.heat)
	}
	return out
}
