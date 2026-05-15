package event

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
	"xinfeedsystem/config"
	"xinfeedsystem/pkg/logger"
)

const producerTimeout = 500 * time.Millisecond

// Producer 封装 Kafka Writer，提供 EmitLike / EmitComment 方法。
// 发送失败仅记日志，不向调用方返回错误（主记录已落 DB，不阻断请求链路）。
type Producer struct {
	writer *kafka.Writer
	cfg    config.KafkaConfig
}

func NewProducer(writer *kafka.Writer, cfg config.KafkaConfig) *Producer {
	return &Producer{writer: writer, cfg: cfg}
}

func (p *Producer) EmitLike(ctx context.Context, ev LikeEvent) {
	p.emit(ctx, p.cfg.LikeTopic, ev.VideoID, ev)
}

func (p *Producer) EmitComment(ctx context.Context, ev CommentEvent) {
	p.emit(ctx, p.cfg.CommentTopic, ev.VideoID, ev)
}

// Close 关闭底层 Writer，由 main.go 在关停时调用。
func (p *Producer) Close() error {
	return p.writer.Close()
}

func (p *Producer) emit(ctx context.Context, topic string, videoID int64, payload any) {
	val, err := json.Marshal(payload)
	if err != nil {
		logger.Error("kafka marshal", zap.String("topic", topic), zap.Error(err))
		return
	}

	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(videoID))

	writeCtx, cancel := context.WithTimeout(ctx, producerTimeout)
	defer cancel()

	if err := p.writer.WriteMessages(writeCtx, kafka.Message{
		Topic: topic,
		Key:   key,
		Value: val,
	}); err != nil {
		logger.Error("kafka write", zap.String("topic", topic),
			zap.Int64("video_id", videoID), zap.Error(err))
	}
}
