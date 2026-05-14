package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"xinfeedsystem/internal/errcode"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/pkg/cache"
	"xinfeedsystem/pkg/cursor"
)

// feedCtxKey avoids collisions with other packages' string keys.
type feedCtxKey string

// FeedUserIDKey is set by the handler so FollowingFetcher can read the caller's ID.
const FeedUserIDKey feedCtxKey = "feed_user_id"

// videoStore is the subset of repository.VideoRepository used by feed services.
// Defined as an interface so tests can inject stubs without a real DB.
type videoStore interface {
	ListLatest(ctx context.Context, cursorTime, cursorID int64, limit int) ([]*entity.Video, error)
	ListByFollowing(ctx context.Context, followerID, cursorTime, cursorID int64, limit int) ([]*entity.Video, error)
	ListByHeat(ctx context.Context, cursorHeat, cursorID int64, limit int) ([]*entity.Video, error)
	ListByLikeCount(ctx context.Context, cursorLikes, cursorID int64, limit int) ([]*entity.Video, error)
	FindByIDs(ctx context.Context, ids []int64) ([]*entity.Video, error)
}

// FeedFetcher is the strategy interface for all feed types.
// Each fetcher owns its cursor format; FeedService is cursor-agnostic.
type FeedFetcher interface {
	Type() string
	// Fetch returns at most limit videos, the next opaque cursor, and whether more pages exist.
	Fetch(ctx context.Context, rawCursor string, limit int) (videos []*entity.Video, nextCursor string, hasMore bool, err error)
}

// ─── LatestFetcher ─────────────────────────────────────────────────────────

type LatestFetcher struct {
	videoRepo videoStore
}

func NewLatestFetcher(r videoStore) *LatestFetcher {
	return &LatestFetcher{videoRepo: r}
}
func (f *LatestFetcher) Type() string { return "latest" }

func (f *LatestFetcher) Fetch(ctx context.Context, rawCursor string, limit int) ([]*entity.Video, string, bool, error) {
	score, id, _ := cursor.Decode(rawCursor)
	videos, err := f.videoRepo.ListLatest(ctx, score, id, limit+1)
	if err != nil {
		return nil, "", false, err
	}
	return buildTimeResult(videos, limit)
}

// ─── FollowingFetcher ──────────────────────────────────────────────────────

type FollowingFetcher struct {
	videoRepo videoStore
	rdb       *redis.Client
}

func NewFollowingFetcher(r videoStore, rdb *redis.Client) *FollowingFetcher {
	return &FollowingFetcher{videoRepo: r, rdb: rdb}
}
func (f *FollowingFetcher) Type() string { return "following" }

func (f *FollowingFetcher) Fetch(ctx context.Context, rawCursor string, limit int) ([]*entity.Video, string, bool, error) {
	followerID, ok := ctx.Value(FeedUserIDKey).(int64)
	if !ok || followerID == 0 {
		return nil, "", false, errcode.New(errcode.Unauthorized)
	}

	cursorTag := rawCursor
	if cursorTag == "" {
		cursorTag = "first"
	}
	cacheKey := fmt.Sprintf("feed:following:%d:%s", followerID, cursorTag)

	// Try Redis: stored as a JSON list of video IDs (limit+1 for hasMore detection).
	var ids []int64
	if hit, isNil, _ := cache.GetJSON(ctx, f.rdb, cacheKey, &ids); hit && !isNil && len(ids) > 0 {
		videos, err := fetchVideosByIDs(ctx, f.rdb, f.videoRepo, ids)
		if err != nil {
			return nil, "", false, err
		}
		return buildTimeResult(videos, limit)
	}

	// Cache miss → DB query.
	score, id, _ := cursor.Decode(rawCursor)
	videos, err := f.videoRepo.ListByFollowing(ctx, followerID, score, id, limit+1)
	if err != nil {
		return nil, "", false, err
	}

	// Cache the IDs (including the extra item so the cached path can detect hasMore).
	idList := make([]int64, len(videos))
	for i, v := range videos {
		idList[i] = v.ID
	}
	_ = cache.SetJSON(ctx, f.rdb, cacheKey, idList, 60*time.Second)

	return buildTimeResult(videos, limit)
}

// ─── SnapshotFetcher (popularity / like_count) ─────────────────────────────

// SnapshotFetcher reads from a Redis ZSet snapshot for stable ranked pagination.
// Cursor encodes (snapshot epoch, rank offset) so the view is frozen mid-scroll.
type SnapshotFetcher struct {
	snapType  string // "popularity" or "like_count"
	rdb       *redis.Client
	videoRepo videoStore
}

func NewSnapshotFetcher(snapType string, rdb *redis.Client, r videoStore) *SnapshotFetcher {
	return &SnapshotFetcher{snapType: snapType, rdb: rdb, videoRepo: r}
}
func (f *SnapshotFetcher) Type() string { return f.snapType }

func (f *SnapshotFetcher) Fetch(ctx context.Context, rawCursor string, limit int) ([]*entity.Video, string, bool, error) {
	sc, err := cursor.DecodeSnapshot(rawCursor)
	if err != nil {
		return nil, "", false, errcode.New(errcode.InvalidParam)
	}

	epoch, offset, err := f.resolveEpoch(ctx, sc)
	if err != nil {
		// No snapshot yet (fresh startup) → fall back to DB.
		return f.fetchFromDB(ctx, limit)
	}

	snapKey := fmt.Sprintf("snapshot:%s:v%d", f.snapType, epoch)
	// Request limit+1 to detect hasMore without an extra ZCARD call.
	members, err := f.rdb.ZRevRange(ctx, snapKey, offset, offset+int64(limit)).Result()
	if err != nil || len(members) == 0 {
		return f.fetchFromDB(ctx, limit)
	}

	hasMore := int64(len(members)) > int64(limit)
	if hasMore {
		members = members[:limit]
	}

	ids := make([]int64, len(members))
	for i, m := range members {
		ids[i], _ = strconv.ParseInt(m, 10, 64)
	}

	videos, err := fetchVideosByIDs(ctx, f.rdb, f.videoRepo, ids)
	if err != nil {
		return nil, "", false, err
	}

	var nextCursor string
	if hasMore {
		nextCursor = cursor.EncodeSnapshot(cursor.SnapshotCursor{
			Version: epoch,
			Offset:  offset + int64(limit),
		})
	}
	return videos, nextCursor, hasMore, nil
}

// resolveEpoch returns the (epoch, startOffset) to use.
// If the client's stored snapshot has expired, it falls back to the current epoch
// and resets the offset to 0 (transparent restart from the new ranking).
func (f *SnapshotFetcher) resolveEpoch(ctx context.Context, sc cursor.SnapshotCursor) (epoch, offset int64, err error) {
	currentKey := fmt.Sprintf("snapshot:%s:current", f.snapType)

	if sc.Version == 0 {
		// First page: use the latest snapshot.
		epochStr, rerr := f.rdb.Get(ctx, currentKey).Result()
		if rerr != nil {
			return 0, 0, rerr
		}
		epoch, err = strconv.ParseInt(epochStr, 10, 64)
		return epoch, 0, err
	}

	// Subsequent page: check if the client's snapshot is still alive.
	snapKey := fmt.Sprintf("snapshot:%s:v%d", f.snapType, sc.Version)
	exists, _ := f.rdb.Exists(ctx, snapKey).Result()
	if exists > 0 {
		return sc.Version, sc.Offset, nil
	}

	// Snapshot expired → transparent fallback to current epoch, offset=0.
	epochStr, rerr := f.rdb.Get(ctx, currentKey).Result()
	if rerr != nil {
		return 0, 0, rerr
	}
	epoch, err = strconv.ParseInt(epochStr, 10, 64)
	return epoch, 0, err
}

// fetchFromDB is used when no ZSet snapshot is available yet (e.g. right after startup).
func (f *SnapshotFetcher) fetchFromDB(ctx context.Context, limit int) ([]*entity.Video, string, bool, error) {
	var (
		videos []*entity.Video
		err    error
	)
	switch f.snapType {
	case "popularity":
		videos, err = f.videoRepo.ListByHeat(ctx, 0, 0, limit+1)
	default:
		videos, err = f.videoRepo.ListByLikeCount(ctx, 0, 0, limit+1)
	}
	if err != nil {
		return nil, "", false, err
	}
	hasMore := len(videos) > limit
	if hasMore {
		videos = videos[:limit]
	}
	return videos, "", hasMore, nil
}

// ─── shared helpers ────────────────────────────────────────────────────────

// buildTimeResult cuts videos to limit and generates a (created_at, id) next cursor.
func buildTimeResult(videos []*entity.Video, limit int) ([]*entity.Video, string, bool, error) {
	hasMore := len(videos) > limit
	if hasMore {
		videos = videos[:limit]
	}
	var nextCursor string
	if hasMore && len(videos) > 0 {
		last := videos[len(videos)-1]
		nextCursor = cursor.Encode(last.CreatedAt, last.ID)
	}
	return videos, nextCursor, hasMore, nil
}

// fetchVideosByIDs fetches video details using Cache-Aside, preserving the order of ids.
// Videos that no longer exist in DB or cache are silently dropped.
func fetchVideosByIDs(ctx context.Context, rdb *redis.Client, repo videoStore, ids []int64) ([]*entity.Video, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	result := make([]*entity.Video, len(ids))
	missIdx := make(map[int64]int) // video id → index in result

	for i, id := range ids {
		var v entity.Video
		hit, isNil, err := cache.GetJSON(ctx, rdb, fmt.Sprintf("video:detail:%d", id), &v)
		if err == nil && hit && !isNil {
			result[i] = &v
		} else if !isNil {
			missIdx[id] = i
		}
	}

	if len(missIdx) > 0 {
		missIDs := make([]int64, 0, len(missIdx))
		for id := range missIdx {
			missIDs = append(missIDs, id)
		}
		dbVideos, err := repo.FindByIDs(ctx, missIDs)
		if err != nil {
			return nil, err
		}
		for _, v := range dbVideos {
			_ = cache.SetJSON(ctx, rdb, fmt.Sprintf("video:detail:%d", v.ID), v,
				cache.RandomizedTTL(5*time.Minute, 30*time.Second))
			if idx, ok := missIdx[v.ID]; ok {
				result[idx] = v
			}
		}
	}

	// Compact: drop nils (deleted or not-found videos).
	out := result[:0]
	for _, v := range result {
		if v != nil {
			out = append(out, v)
		}
	}
	return out, nil
}
