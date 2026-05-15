package kafkaclient

import (
	"context"
	"fmt"
	"net"

	"github.com/segmentio/kafka-go"
	"xinfeedsystem/config"
)

// NewWriter 构造生产者 Writer。topic 在写消息时每条单独指定，支持多 topic。
func NewWriter(cfg config.KafkaConfig) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireAll,
		BatchTimeout: cfg.BatchTimeout / 10, // 生产端微批：1/10 的消费批间隔
		Async:        false,
	}
}

// NewReader 构造消费者 Reader，订阅多个 topic，使用 consumer group 管理 offset。
func NewReader(cfg config.KafkaConfig, topics []string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        cfg.Brokers,
		GroupID:        cfg.ConsumerGroup,
		GroupTopics:    topics,
		MinBytes:       1,
		MaxBytes:       10 * 1024 * 1024, // 10 MB
		CommitInterval: 0,                // 关闭自动提交，业务成功后手动 commit
	})
}

// Ping 通过获取 broker 元数据验证连通性。
func Ping(ctx context.Context, brokers []string) error {
	if len(brokers) == 0 {
		return fmt.Errorf("kafka: no brokers configured")
	}
	conn, err := kafka.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("kafka dial %s: %w", brokers[0], err)
	}
	defer conn.Close()

	if _, err := conn.ReadPartitions(); err != nil {
		// 如果是网络错误直接返回；如果是空 topic 列表也算连通
		if netErr, ok := err.(net.Error); ok || netErr != nil {
			return fmt.Errorf("kafka read partitions: %w", err)
		}
	}
	return nil
}
