package cursor

import (
	"encoding/base64"
	"encoding/json"
)

// SnapshotCursor encodes a position within a Redis ZSet snapshot.
// Version is the snapshot epoch (unix seconds); Offset is the starting rank.
type SnapshotCursor struct {
	Version int64 `json:"v"`
	Offset  int64 `json:"o"`
}

func EncodeSnapshot(c SnapshotCursor) string {
	b, _ := json.Marshal(c)
	return base64.StdEncoding.EncodeToString(b)
}

func DecodeSnapshot(s string) (SnapshotCursor, error) {
	var c SnapshotCursor
	if s == "" {
		return c, nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return c, ErrInvalidCursor
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, ErrInvalidCursor
	}
	return c, nil
}
