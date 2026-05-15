package consumer

import (
	"encoding/json"
	"testing"

	"xinfeedsystem/internal/event"
)

const (
	testLikeTopic    = "xfs.like.events"
	testCommentTopic = "xfs.comment.events"
)

func likeMsg(videoID int64, delta int8) rawEvent {
	b, _ := json.Marshal(event.LikeEvent{EventID: "ev1", VideoID: videoID, Delta: delta})
	return rawEvent{topic: testLikeTopic, value: b}
}

func commentMsg(videoID int64, delta int8) rawEvent {
	b, _ := json.Marshal(event.CommentEvent{EventID: "ev2", VideoID: videoID, Delta: delta})
	return rawEvent{topic: testCommentTopic, value: b}
}

func TestAggregateBatch_LikeAndComment(t *testing.T) {
	msgs := []rawEvent{
		likeMsg(1, +1),
		likeMsg(1, +1),
		likeMsg(2, -1),
		commentMsg(1, +1),
		commentMsg(2, +1),
	}

	result := aggregateBatch(msgs, testLikeTopic, testCommentTopic)

	v1 := result[1]
	if v1.LikeDelta != 2 {
		t.Errorf("video 1 LikeDelta: want 2, got %d", v1.LikeDelta)
	}
	if v1.CommentDelta != 1 {
		t.Errorf("video 1 CommentDelta: want 1, got %d", v1.CommentDelta)
	}
	if v1.HeatDelta != 3 { // 2 likes + 1 comment
		t.Errorf("video 1 HeatDelta: want 3, got %d", v1.HeatDelta)
	}

	v2 := result[2]
	if v2.LikeDelta != -1 {
		t.Errorf("video 2 LikeDelta: want -1, got %d", v2.LikeDelta)
	}
	if v2.CommentDelta != 1 {
		t.Errorf("video 2 CommentDelta: want 1, got %d", v2.CommentDelta)
	}
}

func TestAggregateBatch_Empty(t *testing.T) {
	result := aggregateBatch(nil, testLikeTopic, testCommentTopic)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d entries", len(result))
	}
}

func TestAggregateBatch_UnknownTopicIgnored(t *testing.T) {
	msgs := []rawEvent{
		{topic: "xfs.unknown", value: []byte(`{"video_id":1,"delta":1}`)},
		likeMsg(5, +1),
	}
	result := aggregateBatch(msgs, testLikeTopic, testCommentTopic)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

func TestAggregateBatch_100EventsOn5Videos(t *testing.T) {
	var msgs []rawEvent
	for range 100 {
		msgs = append(msgs, likeMsg(int64(len(msgs)%5+1), +1))
	}
	result := aggregateBatch(msgs, testLikeTopic, testCommentTopic)
	if len(result) != 5 {
		t.Fatalf("expected 5 video keys, got %d", len(result))
	}
	for id, d := range result {
		if d.LikeDelta != 20 {
			t.Errorf("video %d: want LikeDelta=20, got %d", id, d.LikeDelta)
		}
	}
}

func TestExtractEventID(t *testing.T) {
	b, _ := json.Marshal(event.LikeEvent{EventID: "abc-123"})
	if got := extractEventID(b); got != "abc-123" {
		t.Errorf("want abc-123, got %q", got)
	}
	if got := extractEventID([]byte("invalid json")); got != "" {
		t.Errorf("want empty on bad JSON, got %q", got)
	}
}
