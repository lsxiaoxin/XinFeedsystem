package consumer

import (
	"encoding/json"

	"xinfeedsystem/internal/event"
	"xinfeedsystem/internal/repository"
)

// rawEvent holds an unparsed message payload along with its topic.
type rawEvent struct {
	topic string
	value []byte
}

// aggregateBatch 将一批原始消息聚合为每个 videoID 对应的 CounterDelta。
// 纯函数，不依赖任何 I/O，便于单测。
func aggregateBatch(msgs []rawEvent, likeTopic, commentTopic string) map[int64]repository.CounterDelta {
	result := make(map[int64]repository.CounterDelta, len(msgs))

	for _, m := range msgs {
		switch m.topic {
		case likeTopic:
			var ev event.LikeEvent
			if err := json.Unmarshal(m.value, &ev); err != nil {
				continue
			}
			d := result[ev.VideoID]
			d.LikeDelta += int64(ev.Delta)
			d.HeatDelta += int64(ev.Delta)
			result[ev.VideoID] = d

		case commentTopic:
			var ev event.CommentEvent
			if err := json.Unmarshal(m.value, &ev); err != nil {
				continue
			}
			d := result[ev.VideoID]
			d.CommentDelta += int64(ev.Delta)
			d.HeatDelta += int64(ev.Delta)
			result[ev.VideoID] = d
		}
	}
	return result
}

// extractEventID 从原始消息 JSON 中提取 event_id 字段，失败返回空字符串。
func extractEventID(value []byte) string {
	var m struct {
		EventID string `json:"event_id"`
	}
	_ = json.Unmarshal(value, &m)
	return m.EventID
}
