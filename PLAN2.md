# 阶段二实施计划 — Redis 缓存、分布式锁、滑动窗口热榜

## Context

阶段一已完成纯 MySQL 实现（5 张表、18 个接口、4 种 Feed 策略）。当前问题：

1. **VideoDetail / UserInfo 每次请求都打 DB**，热点视频在高并发下会成为单点
2. **热榜 / 点赞数榜的翻页不稳定** — `heat` 与 `like_count` 是可变字段，用户翻第 2 页时榜单已经变了，导致同一视频出现两次或被跳过
3. **JWT 登出依赖 DB 查询** — 中间件每个请求都做 `SELECT token FROM users`，QPS 越高 DB 越累
4. **没有防御缓存击穿的机制** — 一旦引入缓存，热点 key 失效瞬间会有大量请求穿透到 DB

本计划解决这三类问题：
- 引入 Redis（go-redis/v9）作为缓存层
- 实现 SETNX + Lua 释放的轻量级分布式锁（`pkg/redislock`），保护所有缓存回源路径
- 用 Redis ZSet 做热榜「滑动窗口快照」：每 1 分钟重建一次，cursor 编码 `(snapshot_version, offset)` 确保翻页期间榜单不变
- Token 双写 Redis + DB，中间件优先读 Redis（DB 兜底）

不在本次范围：Kafka 异步落库、计数器迁移到 Redis（阶段三）。

---

## 1. 基础设施（依赖、配置、容器）

### 1.1 docker-compose 添加 Redis 服务

文件：`deploy/docker-compose.yaml`

```yaml
services:
  redis:
    image: redis:7-alpine
    container_name: xfs-redis
    restart: unless-stopped
    ports: ["6379:6379"]
    volumes: ["xfs-redis-data:/data"]
    command: ["redis-server", "--appendonly", "yes"]
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  xfs-redis-data:
```

### 1.2 配置结构

文件：`config/config.go` 新增 `RedisConfig`：

```go
type RedisConfig struct {
    Host         string
    Port         int
    Password     string
    DB           int
    PoolSize     int
    DialTimeout  time.Duration
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
}
```

挂到 `Config.Redis`。`config/config.yaml` 新增 `redis:` 节，默认 `127.0.0.1:6379`、pool=50。

### 1.3 Redis 客户端封装

新建 `pkg/redisclient/client.go`：

```go
func New(cfg config.RedisConfig) (*redis.Client, error)  // returns *redis.Client, ping验证
```

`go.mod` 引入 `github.com/redis/go-redis/v9`。

### 1.4 main.go 装配

在 `initDB()` 之后增加：

```go
rdb, err := redisclient.New(cfg.Redis)
if err != nil { logger.Fatal(...) }
defer rdb.Close()

lock := redislock.New(rdb)
snapshotSvc := service.NewSnapshotService(videoRepo, rdb, lock, logger)
go snapshotSvc.Start(ctx)  // 启动 1 分钟周期重建 goroutine
```

---

## 2. 分布式锁（`pkg/redislock`）

新建 `pkg/redislock/lock.go`，实现 SETNX + value=UUID + Lua 释放：

```go
type Lock struct { rdb *redis.Client }

func New(rdb *redis.Client) *Lock

// TryLock 立即尝试获取，成功返回 token（释放时用），失败 ok=false
func (l *Lock) TryLock(ctx context.Context, key string, ttl time.Duration) (token string, ok bool, err error)

// Unlock 用 Lua 原子比对 token 后删除（防止误删别人的锁）
func (l *Lock) Unlock(ctx context.Context, key, token string) error

// Do 是 Cache-Aside 防击穿的高阶封装：拿不到锁时短轮询等待最多 maxWait
//   loader 仅由持锁者执行；其他调用方等持锁者写回缓存后从 cache 读
func (l *Lock) Do(
    ctx context.Context,
    key string,                          // 锁 key
    ttl time.Duration,                   // 锁过期时间
    maxWait time.Duration,               // 等待持锁者的总超时
    loader func() (interface{}, error),  // 持锁者要执行的回源逻辑
    waiter func() (interface{}, bool, error), // 等待者重试读缓存的逻辑（hit→返回, miss→继续等）
) (interface{}, error)
```

Unlock Lua 脚本：
```lua
if redis.call("get", KEYS[1]) == ARGV[1]
then return redis.call("del", KEYS[1])
else return 0 end
```

使用场景：
- VideoDetail / UserInfo Cache-Aside 回源
- 热榜快照重建（key: `lock:snapshot:popularity` / `lock:snapshot:like_count`）

---

## 3. Token 双写 Redis + DB

### 3.1 Redis 存储

key/value 设计：
- `user:token:{user_id}` → token 字符串，TTL = JWT expire（7d）

### 3.2 Repository 层扩展

新建 `internal/repository/token_cache.go`：

```go
type TokenCache struct { rdb *redis.Client }

func NewTokenCache(rdb *redis.Client) *TokenCache
func (c *TokenCache) Save(ctx, userID int64, token string, ttl time.Duration) error
func (c *TokenCache) Get(ctx, userID int64) (string, error)  // miss 返回 "" 无错误
func (c *TokenCache) Delete(ctx, userID int64) error
```

### 3.3 修改点

- `service/user_service.go`：
  - `Login()` 签名 token 后**先写 Redis 再写 DB**（DB 失败回滚 Redis）
  - `Logout()` 同时清 Redis 和 DB
- `middleware/jwt.go`：
  - `JWTAuth(userRepo, tokenCache)` 和 `OptionalAuth(userRepo, tokenCache)` 都加 tokenCache 参数
  - 验证逻辑：先 `tokenCache.Get`，hit 比对；miss 回退 `userRepo.FindTokenByUserID`（兜底）并回填 Redis

`users.token` 列保留不删，作为「Redis 挂了之后还能登录验证」的兜底（用户明确选择双写策略）。

---

## 4. VideoDetail / UserInfo 缓存（Cache-Aside）

### 4.1 Key 设计

- `video:detail:{id}` → JSON 序列化的 `entity.Video`，TTL 5min ± 30s 随机偏移（防雪崩）
- `user:info:{id}` → JSON 序列化的 `entity.User`（不含 token、password_hash），TTL 10min ± 30s
- 防穿透：DB miss 时写入 `__nil__` 占位，TTL 30s

### 4.2 实现位置

不做 Repository Decorator（改动大），改在 Service 层做 Cache-Aside：

- `service/video_service.go::GetDetail` 改造为：
  ```go
  // 1. 查 Redis → hit 直接返回
  // 2. miss → lock.Do(key="lock:video:detail:{id}", loader=查DB+回填Redis, waiter=重试查Redis)
  ```
- `service/user_service.go::GetUserInfo` 同款改造

新建 `pkg/cache/` 工具：
```go
// pkg/cache/json.go
func GetJSON(ctx, rdb, key string, v interface{}) (hit bool, isNil bool, err error)
func SetJSON(ctx, rdb, key string, v interface{}, ttl time.Duration) error
func SetNil(ctx, rdb, key string, ttl time.Duration) error
func RandomizedTTL(base time.Duration, jitter time.Duration) time.Duration
```

### 4.3 失效策略

写发生时 service 主动 `DEL` 对应 key：
- `LikeService.Action`（like/unlike）→ DEL `video:detail:{vid}`（like_count 变了）
- `CommentService.Post/Delete` → DEL `video:detail:{vid}`（comment_count 变了）
- `FollowService.Action` → DEL `user:info:{follower_id}` + DEL `user:info:{followee_id}`（follow_count/follower_count 变了）
- `VideoService.Publish` 时无需失效（新建无缓存）

LikeService/CommentService/FollowService 构造函数都加一个 `*redis.Client` 参数。

---

## 5. 滑动窗口热榜（核心）— ZSet 快照

### 5.1 数据结构

| Key | 类型 | 内容 |
|---|---|---|
| `snapshot:popularity:current` | String | 当前最新 epoch（unix 秒） |
| `snapshot:popularity:v{epoch}` | ZSet | member=video_id, score=heat（取 Top N=1000） |
| `snapshot:like_count:current` | String | 同上 |
| `snapshot:like_count:v{epoch}` | ZSet | member=video_id, score=like_count |
| `lock:snapshot:{type}` | String | 重建分布式锁 |

历史 epoch 设 TTL = 10 分钟（够翻页中的用户继续用旧版本）。
最新 epoch 设 TTL = 15 分钟（覆盖到下一次重建 + 兜底）。

### 5.2 SnapshotService（新建）

文件：`internal/service/snapshot_service.go`

```go
type SnapshotService struct {
    videoRepo *repository.VideoRepository
    rdb       *redis.Client
    lock      *redislock.Lock
}

// Start 启动后台 ticker，1 分钟重建一次 popularity + like_count
func (s *SnapshotService) Start(ctx context.Context)

// Rebuild 单次重建：加锁 → SELECT Top 1000 → ZADD pipeline → 切换 current 指针
func (s *SnapshotService) Rebuild(ctx context.Context, snapType string) error
```

Rebuild 步骤：
1. `lock.TryLock("lock:snapshot:"+snapType, ttl=30s)` → 拿不到说明别的实例正在做，直接 return
2. 生成新 epoch = `time.Now().Unix()`
3. 查 DB：`SELECT id, heat FROM videos WHERE status=1 ORDER BY heat DESC, id DESC LIMIT 1000`
4. Pipeline ZADD 到 `snapshot:popularity:v{epoch}`，EXPIRE 15min
5. SET `snapshot:popularity:current` = epoch（无 TTL，永远指向最新）
6. 释放锁
7. （旧的 epoch 自然 TTL 过期，无需手动清理）

服务启动时立即触发一次 Rebuild（避免首请求时还没快照）。

### 5.3 SnapshotFetcher（替换 PopularityFetcher / LikeCountFetcher）

`FeedFetcher` 接口需要扩展，让 Fetcher 自己处理 cursor —— 因为通用 `(score, cursorID)` 的语义和「snapshot version + offset」不兼容。

新接口（修改 `internal/service/feed_fetcher.go`）：

```go
type FeedFetcher interface {
    Type() string
    // Fetch 接收原始字符串 cursor，自行解析；返回视频、下一页 cursor、是否还有更多
    Fetch(ctx context.Context, rawCursor string, limit int) (
        videos []*entity.Video,
        nextCursor string,
        hasMore bool,
        err error,
    )
}
```

`FeedService.GetFeed` 简化：仅做策略分发 + 调用 `fetcher.Fetch(req.Cursor, limit)`，不再操心 cursor 解码。

- `LatestFetcher` / `FollowingFetcher` 内部仍用 `pkg/cursor`（封装 `(created_at, id)`）
- `SnapshotFetcher`（新增，替换 PopularityFetcher + LikeCountFetcher）用新 cursor：

```go
// pkg/cursor/snapshot.go
type SnapshotCursor struct {
    Version int64 `json:"v"` // snapshot epoch
    Offset  int64 `json:"o"` // 已读偏移
}
func EncodeSnapshot(SnapshotCursor) string
func DecodeSnapshot(string) (SnapshotCursor, error)
```

SnapshotFetcher.Fetch 逻辑：
1. 首页（cursor 空）：读 `snapshot:{type}:current` → epoch；offset=0
2. 翻页：解 cursor → (epoch, offset)；先检查 `EXISTS snapshot:{type}:v{epoch}`：
   - 存在 → 用这个 epoch 继续
   - 不存在（过期了）→ 回退到 current epoch + offset=0（前端透明地从新榜单第 1 页继续，避免 401-cursor-expired 这种坏体验）
3. `ZREVRANGE snapshot:{type}:v{epoch} {offset} {offset+limit}` 拿 video_id 列表
4. 批量 `MGET video:detail:{id}`（复用第 4 节的视频缓存），miss 部分回查 DB + 回填
5. 编码 `nextCursor = SnapshotCursor{Version: epoch, Offset: offset+limit}`，hasMore = ZSet 中还有更多
6. 注册两个 SnapshotFetcher 实例：`Type() = "popularity"` 和 `Type() = "like_count"`，构造时区分 prefix

main.go 注册：
```go
service.NewSnapshotFetcher(rdb, videoRepo, "popularity"),
service.NewSnapshotFetcher(rdb, videoRepo, "like_count"),
```

LatestFetcher / FollowingFetcher 也要更新签名以匹配新接口（cursor 自己解码/编码）。

---

## 6. 修改清单（按文件归纳）

### 新建文件
- `pkg/redisclient/client.go` — Redis 客户端
- `pkg/redislock/lock.go` — 分布式锁 + Lua
- `pkg/cache/json.go` — JSON GET/SET + nil 占位 + TTL 抖动
- `pkg/cursor/snapshot.go` — Snapshot cursor 编解码
- `internal/repository/token_cache.go` — Token Redis 读写
- `internal/service/snapshot_service.go` — 后台快照重建
- 新 fetcher 类型在 `internal/service/feed_fetcher.go` 内（不新建文件）

### 修改文件
- `config/config.go` — 加 `RedisConfig`
- `config/config.yaml` — 加 `redis:` 节
- `deploy/docker-compose.yaml` — 加 redis service
- `Makefile` — `up` target 起 redis；新加 `redis-cli` target 方便调试
- `go.mod` / `go.sum` — `go get github.com/redis/go-redis/v9`
- `cmd/server/main.go` — 装配 Redis client、SnapshotService、改造 fetcher 注册
- `internal/middleware/jwt.go` — 加 tokenCache 参数，优先查 Redis
- `internal/router/router.go` — 把 tokenCache 透传给 middleware
- `internal/service/user_service.go` — Login/Logout 双写
- `internal/service/video_service.go` — GetDetail 加 Cache-Aside + 锁
- `internal/service/user_service.go::GetUserInfo` — Cache-Aside + 锁
- `internal/service/like_service.go` — 写后 DEL `video:detail:{vid}`
- `internal/service/comment_service.go` — 写后 DEL `video:detail:{vid}`
- `internal/service/follow_service.go` — 写后 DEL `user:info:{两个 uid}`
- `internal/service/feed_fetcher.go` — 接口签名扩展、删 PopularityFetcher / LikeCountFetcher 旧实现、加 SnapshotFetcher
- `internal/service/feed_service.go` — 简化 GetFeed（不再处理 cursor）
- `README.md` — 阶段二状态改为「已完成」，新增 Redis 设计章节

---

## 7. 验证步骤（端到端）

```bash
# 1. 起容器
docker compose -f deploy/docker-compose.yaml up -d
make run

# 2. Token 验证
TOKEN=$(curl -s -X POST .../user/login -d '{"username":"x","password":"y"}' | jq -r .data.token)
docker exec xfs-redis redis-cli GET "user:token:<uid>"   # 期望命中
# 在 Redis 中 DEL 这个 key，重新请求 /user/me，应该走 DB 兜底并回填 Redis

# 3. VideoDetail Cache-Aside
curl .../video/<id>   # 第一次走 DB
docker exec xfs-redis redis-cli GET "video:detail:<id>"   # 期望存在
# 并发 100 个相同请求（hey -n 100 -c 100），观察 DB log 只查询 1 次（分布式锁生效）

# 4. 缓存失效
curl -X POST .../like/action -d '{"video_id":"<id>","action_type":1}' -H "Authorization: Bearer $TOKEN"
docker exec xfs-redis redis-cli EXISTS "video:detail:<id>"   # 期望 0（已被 DEL）

# 5. 滑动窗口热榜稳定性测试
curl ".../feed?type=popularity&limit=5"        # 拿到 cursor1, 记录 5 个视频ID
# 在 5 秒内对榜单中某条视频疯狂点赞 20 次（模拟榜单变动）
curl ".../feed?type=popularity&limit=5&cursor=<cursor1>"   # 期望返回原快照的下 5 条，无重复
# 等待 70 秒（超过快照重建周期），再用同一个旧 cursor 翻页
# 期望：旧 epoch 还在 TTL 内（10 min）→ 继续返回旧快照数据；过 10 分钟后 → 自动回退到 current

# 6. 快照重建锁验证
docker exec xfs-redis redis-cli MONITOR &
# 观察每分钟有且只有一次 ZADD snapshot:popularity:v{epoch} 的批量写入

# 7. 单元测试
go test ./pkg/redislock/...
go test ./pkg/cursor/...
```

---

## 8. 风险与权衡

| 风险 | 缓解 |
|---|---|
| Redis 挂了导致全站不可用 | Token 双写兜底 DB；VideoDetail/UserInfo Cache-Aside 自然降级到查 DB（只是没缓存）；快照失败时 SnapshotFetcher 降级查 DB ListByHeat |
| 锁的 token 持锁者执行慢于 ttl 导致提前释放 | loader 控制在 ttl 的 1/3 内完成；DB 查询有 ctx 超时 |
| 快照过期时翻页用户体验 | 自动回退到 current epoch 第一页（透明），避免 401-cursor-expired |
| 数据一致性：写 DB 成功但 DEL cache 失败 | TTL 5min 兜底；故意不引入复杂的 2PC，符合 Cache-Aside 模式 |
| Top 1000 不够大（用户翻得很深） | 学习项目可接受；阶段三可改为分段加载 |

---

## 9. 完成定义

- [ ] `docker compose up -d` 起 mysql + redis 两个容器
- [ ] `make run` 启动服务，日志显示「redis connected」「snapshot rebuild started」
- [ ] 登录 → Redis 有 `user:token:{uid}` → 重启 Redis（清空）→ 旧 token 仍可用（DB 兜底）→ 重新请求后 Redis 又有了（回填）
- [ ] 并发 100 个 `/video/:id` 相同 ID 请求，DB 只查 1 次
- [ ] 点赞后 `video:detail:{id}` 被 DEL
- [ ] 启动后等 1 分钟，`snapshot:popularity:v{epoch}` 存在且有数据
- [ ] 翻页期间疯狂改动 heat，分页结果稳定（按旧快照返回），无重复无遗漏
- [ ] README 阶段二勾选「已完成」
