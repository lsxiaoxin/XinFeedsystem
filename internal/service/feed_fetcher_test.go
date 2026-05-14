package service

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/pkg/cursor"
)

// ═══════════════════════════════════════════════════════
// SnapshotFetcher tests
// ═══════════════════════════════════════════════════════

// seedSnapshot writes a ZSet and the current pointer for a given snapType and epoch.
func seedSnapshot(t *testing.T, rdb *redis.Client, snapType string, epoch int64, videos []*videoSeed) {
	t.Helper()
	ctx := context.Background()
	snapKey := fmt.Sprintf("snapshot:%s:v%d", snapType, epoch)
	for _, vs := range videos {
		rdb.ZAdd(ctx, snapKey, redis.Z{Score: float64(vs.heat), Member: strconv.FormatInt(vs.id, 10)})
	}
	rdb.Set(ctx, fmt.Sprintf("snapshot:%s:current", snapType), epoch, 0)
}

func TestSnapshotFetcher_FirstPage_ReadsCurrentEpoch(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := context.Background()

	epoch := int64(1000)
	vids := []*videoSeed{{1, 900}, {2, 800}, {3, 700}, {4, 600}, {5, 500}}
	seedSnapshot(t, rdb, "popularity", epoch, vids)
	// Pre-warm video:detail cache so no DB call is needed.
	seedVideoDetailCache(t, rdb, makeVideos(vids)...)

	f := NewSnapshotFetcher("popularity", rdb, &stubVideoStore{})
	videos, nextCursor, hasMore, err := f.Fetch(ctx, "", 3)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos) != 3 {
		t.Fatalf("expected 3 videos, got %d", len(videos))
	}
	if !hasMore {
		t.Fatal("expected hasMore=true")
	}
	// Highest-heat videos should come first (ZREVRANGE).
	if videos[0].ID != 1 || videos[1].ID != 2 || videos[2].ID != 3 {
		t.Errorf("unexpected order: %v %v %v", videos[0].ID, videos[1].ID, videos[2].ID)
	}

	// nextCursor must encode epoch=1000, offset=3.
	sc, err := cursor.DecodeSnapshot(nextCursor)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if sc.Version != epoch || sc.Offset != 3 {
		t.Errorf("expected cursor {1000,3}, got {%d,%d}", sc.Version, sc.Offset)
	}
}

func TestSnapshotFetcher_Pagination_IsStable(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := context.Background()

	epoch := int64(1000)
	vids := []*videoSeed{{1, 900}, {2, 800}, {3, 700}, {4, 600}, {5, 500}}
	seedSnapshot(t, rdb, "popularity", epoch, vids)
	seedVideoDetailCache(t, rdb, makeVideos(vids)...)

	f := NewSnapshotFetcher("popularity", rdb, &stubVideoStore{})

	// Get first page.
	_, firstCursor, _, _ := f.Fetch(ctx, "", 3)

	// Simulate ranking change: update current to a brand-new epoch (epoch+1).
	// The second page request carries the old cursor, so it should still use epoch 1000.
	newEpoch := epoch + 1
	newVids := []*videoSeed{{10, 9999}, {11, 9998}} // completely different ranking
	seedSnapshot(t, rdb, "popularity", newEpoch, newVids)
	seedVideoDetailCache(t, rdb, makeVideos(newVids)...)

	// Second page using old cursor → must read from epoch 1000, not new epoch.
	videos, _, hasMore, err := f.Fetch(ctx, firstCursor, 3)
	if err != nil {
		t.Fatalf("second page error: %v", err)
	}
	// Remaining items from epoch 1000: videos 4 and 5.
	if len(videos) != 2 {
		t.Fatalf("expected 2 videos on second page, got %d", len(videos))
	}
	if videos[0].ID != 4 || videos[1].ID != 5 {
		t.Errorf("stability broken: got IDs %d %d, want 4 5", videos[0].ID, videos[1].ID)
	}
	if hasMore {
		t.Fatal("expected hasMore=false on last page")
	}
}

func TestSnapshotFetcher_ExpiredSnapshot_FallsBackToCurrentEpoch(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := context.Background()

	// Only epoch 2000 is alive; epoch 1000 is gone (simulate TTL expiry).
	vids2000 := []*videoSeed{{10, 500}, {11, 400}}
	seedSnapshot(t, rdb, "popularity", 2000, vids2000)
	seedVideoDetailCache(t, rdb, makeVideos(vids2000)...)

	// Build a cursor that references the expired epoch 1000, offset 3.
	staleCursor := cursor.EncodeSnapshot(cursor.SnapshotCursor{Version: 1000, Offset: 3})

	f := NewSnapshotFetcher("popularity", rdb, &stubVideoStore{})
	videos, _, _, err := f.Fetch(ctx, staleCursor, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fall back to epoch 2000 from offset 0, returning videos 10 and 11.
	if len(videos) != 2 {
		t.Fatalf("expected 2 videos from fallback epoch, got %d", len(videos))
	}
	if videos[0].ID != 10 && videos[1].ID != 11 {
		t.Errorf("fallback returned wrong videos: %v", videos)
	}
}

func TestSnapshotFetcher_NoSnapshotYet_FallsBackToDB(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := context.Background()

	// Redis is empty (no snapshot built yet).
	dbVideos := []*videoSeed{{100, 50}, {101, 40}}
	store := &stubVideoStore{popular: makeVideos(dbVideos)}

	f := NewSnapshotFetcher("popularity", rdb, store)
	videos, _, hasMore, err := f.Fetch(ctx, "", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos) != 2 {
		t.Fatalf("expected 2 DB-fallback videos, got %d", len(videos))
	}
	if hasMore {
		t.Fatal("expected hasMore=false")
	}
}

func TestSnapshotFetcher_VideoDetailCache_Miss_QueriesDB(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := context.Background()

	epoch := int64(3000)
	vids := []*videoSeed{{1, 100}, {2, 90}}
	seedSnapshot(t, rdb, "popularity", epoch, vids)
	// Do NOT pre-warm video:detail cache → fetchVideosByIDs must call FindByIDs.

	byID := map[int64]*entity.Video{
		1: makeVideo(1, 100),
		2: makeVideo(2, 90),
	}
	store := &stubVideoStore{byID: byID}
	f := NewSnapshotFetcher("popularity", rdb, store)

	videos, _, _, err := f.Fetch(ctx, "", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos) != 2 {
		t.Fatalf("expected 2 videos from DB, got %d", len(videos))
	}
	// After first fetch, video:detail keys should now be cached.
	for _, id := range []int64{1, 2} {
		exists, _ := rdb.Exists(ctx, fmt.Sprintf("video:detail:%d", id)).Result()
		if exists == 0 {
			t.Errorf("video:detail:%d should be cached after fetch", id)
		}
	}
}

// ═══════════════════════════════════════════════════════
// FollowingFetcher tests
// ═══════════════════════════════════════════════════════

func ctxWithUser(userID int64) context.Context {
	return context.WithValue(context.Background(), FeedUserIDKey, userID)
}

func TestFollowingFetcher_NoUserID_ReturnsUnauthorized(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	f := NewFollowingFetcher(&stubVideoStore{}, rdb)

	_, _, _, err := f.Fetch(context.Background(), "", 5)
	if err == nil {
		t.Fatal("expected Unauthorized error")
	}
}

func TestFollowingFetcher_CacheMiss_QueriesDBAndCachesIDs(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := ctxWithUser(42)

	followingVids := makeVideos([]*videoSeed{{10, 0}, {11, 0}, {12, 0}})
	store := &stubVideoStore{following: followingVids}
	f := NewFollowingFetcher(store, rdb)

	videos, _, _, err := f.Fetch(ctx, "", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos) != 3 {
		t.Fatalf("expected 3 videos, got %d", len(videos))
	}
	// DB should have been called once.
	if store.followingCalls != 1 {
		t.Fatalf("expected 1 DB call, got %d", store.followingCalls)
	}
	// IDs must be cached now.
	cacheKey := "feed:following:42:first"
	exists, _ := rdb.Exists(ctx, cacheKey).Result()
	if exists == 0 {
		t.Errorf("following feed cache key not set after miss: %s", cacheKey)
	}
}

func TestFollowingFetcher_CacheHit_SkipsDB(t *testing.T) {
	rdb, _ := newMiniRedis(t)
	ctx := ctxWithUser(42)

	vids := makeVideos([]*videoSeed{{10, 0}, {11, 0}})

	// Pre-fill the feed cache (IDs) and the video:detail cache.
	seedVideoDetailCache(t, rdb, vids...)
	// Store IDs as JSON list (same format FollowingFetcher writes).
	import_json_list := `[10,11]`
	rdb.Set(ctx, "feed:following:42:first", import_json_list, 0)

	store := &stubVideoStore{} // no following videos → DB would return nothing
	f := NewFollowingFetcher(store, rdb)

	videos, _, _, err := f.Fetch(ctx, "", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(videos) != 2 {
		t.Fatalf("expected 2 videos from cache, got %d", len(videos))
	}
	// DB must NOT have been called.
	if store.followingCalls != 0 {
		t.Fatalf("DB should not be called on cache hit, got %d calls", store.followingCalls)
	}
}
