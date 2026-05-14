// Package cursor 提供 Feed 分页游标的编解码。
// 游标对客户端透明（base64 opaque），内部存储 (Score, ID) 两个 int64。
// Score 的含义由各 FeedFetcher 决定：
//   - LatestFetcher   → created_at (ms)
//   - LikeCountFetcher → like_count
//   - PopularityFetcher → 综合分
package cursor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

var ErrInvalidCursor = errors.New("invalid cursor")

type data struct {
	Score int64 `json:"s"`
	ID    int64 `json:"i"`
}

func Encode(score, id int64) string {
	b, _ := json.Marshal(data{Score: score, ID: id})
	return base64.StdEncoding.EncodeToString(b)
}

func Decode(s string) (score, id int64, err error) {
	if s == "" {
		return 0, 0, nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return 0, 0, ErrInvalidCursor
	}
	var d data
	if err = json.Unmarshal(b, &d); err != nil {
		return 0, 0, ErrInvalidCursor
	}
	return d.Score, d.ID, nil
}
