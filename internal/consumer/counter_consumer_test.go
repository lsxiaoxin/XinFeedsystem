package consumer

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"xinfeedsystem/config"
	"xinfeedsystem/internal/event"
	"xinfeedsystem/internal/repository"
	"xinfeedsystem/pkg/logger"
)

func TestMain(m *testing.M) {
	_ = logger.Init("error", "console") // suppress noise in test output
	os.Exit(m.Run())
}

// ─── fakes ────────────────────────────────────────────────────────────────────

type fakeVideoStore struct {
	calls  int
	deltas []map[int64]repository.CounterDelta
}

func (f *fakeVideoStore) ApplyCounterDeltas(_ context.Context, d map[int64]repository.CounterDelta) error {
	f.calls++
	f.deltas = append(f.deltas, d)
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newTestMiniRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func testCfg() config.KafkaConfig {
	return config.KafkaConfig{
		LikeTopic:    testLikeTopic,
		CommentTopic: testCommentTopic,
		BatchSize:    100,
		BatchTimeout: 50 * time.Millisecond,
	}
}

// ─── dedupe tests ─────────────────────────────────────────────────────────────

func dummyMsgs(n int) []kafka.Message {
	msgs := make([]kafka.Message, n)
	return msgs
}

func TestDedupe_NewEventPasses(t *testing.T) {
	rdb := newTestMiniRedis(t)
	c := &CounterConsumer{rdb: rdb, cfg: testCfg()}

	b, _ := json.Marshal(event.LikeEvent{EventID: "new-ev", VideoID: 1, Delta: 1})
	raws := []rawEvent{{topic: testLikeTopic, value: b}}

	filtered, _ := c.dedupe(context.Background(), raws, dummyMsgs(len(raws)))
	if len(filtered) != 1 {
		t.Fatalf("new event should pass dedupe, got %d events", len(filtered))
	}
}

func TestDedupe_DuplicateEventFiltered(t *testing.T) {
	rdb := newTestMiniRedis(t)
	ctx := context.Background()

	// Pre-mark event_id as processed
	_ = rdb.SetNX(ctx, dedupeKeyPrefix+"dup-ev", 1, dedupeTTL).Err()

	c := &CounterConsumer{rdb: rdb, cfg: testCfg()}

	b, _ := json.Marshal(event.LikeEvent{EventID: "dup-ev", VideoID: 1, Delta: 1})
	raws := []rawEvent{{topic: testLikeTopic, value: b}}

	filtered, _ := c.dedupe(ctx, raws, dummyMsgs(len(raws)))
	if len(filtered) != 0 {
		t.Fatalf("duplicate event should be filtered, got %d events", len(filtered))
	}
}

// ─── applyWithRetry tests ─────────────────────────────────────────────────────

func TestApplyWithRetry_Success(t *testing.T) {
	store := &fakeVideoStore{}
	c := &CounterConsumer{videoRepo: store, cfg: testCfg()}

	deltas := map[int64]repository.CounterDelta{1: {LikeDelta: 2, HeatDelta: 2}}
	if err := c.applyWithRetry(context.Background(), deltas); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store.calls != 1 {
		t.Errorf("expected 1 call, got %d", store.calls)
	}
}

// ─── markAndInvalidate tests ──────────────────────────────────────────────────

func TestMarkAndInvalidate_CacheKeyDeleted(t *testing.T) {
	rdb := newTestMiniRedis(t)
	ctx := context.Background()

	// Pre-populate cache
	_ = rdb.Set(ctx, "video:detail:42", "cached", time.Minute).Err()

	c := &CounterConsumer{rdb: rdb, cfg: testCfg()}

	b, _ := json.Marshal(event.LikeEvent{EventID: "ev-mark", VideoID: 42, Delta: 1})
	processed := []rawEvent{{topic: testLikeTopic, value: b}}
	deltas := map[int64]repository.CounterDelta{42: {LikeDelta: 1}}

	c.markAndInvalidate(ctx, processed, deltas)

	exists, _ := rdb.Exists(ctx, "video:detail:42").Result()
	if exists != 0 {
		t.Errorf("expected video:detail:42 to be deleted after invalidation")
	}
}
