package consumer

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"xinfeedsystem/config"
	"xinfeedsystem/internal/repository"
	"xinfeedsystem/pkg/logger"
)

const (
	dedupeKeyPrefix = "processed:event:"
	dedupeTTL       = time.Hour
	maxRetries      = 3
)

// videoCounterStore 是 consumer 对 VideoRepository 所需方法的最小接口，便于测试注入。
type videoCounterStore interface {
	ApplyCounterDeltas(ctx context.Context, deltas map[int64]repository.CounterDelta) error
}

// CounterConsumer 从 like / comment 两个 topic 消费事件，
// 批量聚合后写入 DB，并失效对应的 Redis 视频详情缓存。
type CounterConsumer struct {
	reader    *kafka.Reader
	videoRepo videoCounterStore
	rdb       *redis.Client
	cfg       config.KafkaConfig
}

func NewCounterConsumer(
	reader *kafka.Reader,
	videoRepo videoCounterStore,
	rdb *redis.Client,
	cfg config.KafkaConfig,
) *CounterConsumer {
	return &CounterConsumer{reader: reader, videoRepo: videoRepo, rdb: rdb, cfg: cfg}
}

// Start 启动消费循环，阻塞直到 ctx 被取消。
// 收到取消信号后会处理完当前缓冲区再退出（优雅排空）。
func (c *CounterConsumer) Start(ctx context.Context) {
	logger.Info("kafka consumer started",
		zap.Strings("topics", []string{c.cfg.LikeTopic, c.cfg.CommentTopic}))
	defer func() {
		_ = c.reader.Close()
		logger.Info("kafka consumer stopped")
	}()

	for {
		batch, msgs := c.collectBatch(ctx)

		if len(batch) == 0 {
			// ctx 已取消且无待处理消息，退出
			if ctx.Err() != nil {
				return
			}
			continue
		}

		// 去重：过滤掉 Redis 中已处理的 event_id
		filtered, filteredMsgs := c.dedupe(ctx, batch, msgs)

		if len(filtered) > 0 {
			deltas := aggregateBatch(filtered, c.cfg.LikeTopic, c.cfg.CommentTopic)

			// 带重试写 DB
			if err := c.applyWithRetry(ctx, deltas); err != nil {
				logger.Error("kafka consumer: apply deltas failed", zap.Error(err))
				// 不 commit，下次重投（at-least-once）
				continue
			}

			// 标记去重 key + 失效缓存（best-effort pipeline）
			c.markAndInvalidate(ctx, filtered, deltas)
		}

		// 所有步骤成功后，一次性 commit 本批次 offset
		if err := c.reader.CommitMessages(ctx, filteredMsgs...); err != nil {
			if ctx.Err() == nil {
				logger.Error("kafka consumer: commit failed", zap.Error(err))
			}
		}
	}
}

// collectBatch 在 BatchTimeout 时间内或达到 BatchSize 时收集一批消息。
// ctx 取消后会返回已缓冲的消息（优雅排空）。
func (c *CounterConsumer) collectBatch(ctx context.Context) ([]rawEvent, []kafka.Message) {
	deadline := time.Now().Add(c.cfg.BatchTimeout)
	var raws []rawEvent
	var msgs []kafka.Message

	for len(raws) < c.cfg.BatchSize {
		fetchCtx, cancel := context.WithDeadline(ctx, deadline)
		msg, err := c.reader.FetchMessage(fetchCtx)
		cancel()

		if err != nil {
			// deadline 到了，或 ctx 被取消
			break
		}
		raws = append(raws, rawEvent{topic: msg.Topic, value: msg.Value})
		msgs = append(msgs, msg)
	}
	return raws, msgs
}

// dedupe 过滤已处理的事件（Redis SET NX），返回需要处理的子集和对应 kafka.Message。
func (c *CounterConsumer) dedupe(ctx context.Context, raws []rawEvent, msgs []kafka.Message) ([]rawEvent, []kafka.Message) {
	var filtered []rawEvent
	var filteredMsgs []kafka.Message

	pipe := c.rdb.Pipeline()
	cmds := make([]*redis.BoolCmd, len(raws))
	for i, r := range raws {
		eid := extractEventID(r.value)
		if eid == "" {
			filtered = append(filtered, r)
			filteredMsgs = append(filteredMsgs, msgs[i])
			continue
		}
		cmds[i] = pipe.SetNX(ctx, dedupeKeyPrefix+eid, 1, dedupeTTL)
	}
	_, _ = pipe.Exec(ctx)

	for i, r := range raws {
		if cmds[i] == nil {
			// event_id 为空，已直接加入 filtered
			continue
		}
		if cmds[i].Val() {
			filtered = append(filtered, r)
			filteredMsgs = append(filteredMsgs, msgs[i])
		} else {
			logger.Info("kafka consumer: duplicate event skipped",
				zap.String("event_id", extractEventID(r.value)))
		}
	}
	return filtered, filteredMsgs
}

// applyWithRetry 带指数退避的重试写 DB。
func (c *CounterConsumer) applyWithRetry(ctx context.Context, deltas map[int64]repository.CounterDelta) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		if err = c.videoRepo.ApplyCounterDeltas(ctx, deltas); err == nil {
			return nil
		}
		if i < maxRetries-1 {
			time.Sleep(time.Duration(1<<uint(i)) * 100 * time.Millisecond)
		}
	}
	return err
}

// markAndInvalidate 在 pipeline 内标记去重 key 的 EXPIRE 并 DEL 视频详情缓存。
func (c *CounterConsumer) markAndInvalidate(ctx context.Context, processed []rawEvent, deltas map[int64]repository.CounterDelta) {
	pipe := c.rdb.Pipeline()

	// 刷新去重 key TTL（SetNX 已设置，这里确保 TTL 正确）
	for _, r := range processed {
		eid := extractEventID(r.value)
		if eid != "" {
			pipe.Expire(ctx, dedupeKeyPrefix+eid, dedupeTTL)
		}
	}

	// 失效相关视频的 detail 缓存
	for videoID := range deltas {
		pipe.Del(ctx, fmt.Sprintf("video:detail:%d", videoID))
	}

	if _, err := pipe.Exec(ctx); err != nil && ctx.Err() == nil {
		logger.Error("kafka consumer: cache invalidation failed", zap.Error(err))
	}
}
