# XinFeedSystem — 视频 Feed 流后端系统

> 纯后端 Go 项目，涵盖用户、视频、Feed 流、点赞、评论、关注六大模块。分三阶段演进（MySQL → Redis → Kafka），当前完成阶段一、阶段二。

---

## 技术栈

| 类别 | 选型 |
|---|---|
| 语言 | Go 1.21+ |
| Web 框架 | Gin |
| ORM | GORM + MySQL 8.0 |
| 缓存 | Redis 7（go-redis/v9） |
| 认证 | JWT（HS256，7 天有效期）|
| ID 生成 | 雪花算法（Snowflake） |
| 密码安全 | bcrypt（cost=10，自带盐） |
| 配置 | Viper（yaml + 环境变量） |
| 日志 | Zap（结构化日志） |
| 容器化依赖 | Docker Compose（MySQL + Redis） |

---

## 快速启动

```bash
# 1. 启动 MySQL + Redis
docker compose -f deploy/docker-compose.yaml up -d

# 2. 启动服务（debug 模式自动 AutoMigrate 建表）
make run

# 3. 验证
curl http://localhost:8080/healthz
# → {"status":"ok"}

# 4. 热重载开发
air
```

配置文件：`config/config.yaml`，MySQL 用户 `xfs / xfs123`，库名 `xinfeedsystem`；Redis 默认 `127.0.0.1:6379`。

---

## 项目分层结构

```
cmd/server/main.go          # 依赖装配入口（DI 手动注入）
internal/
  api/                      # Handler 层：参数绑定 → 调 Service → 返回 JSON
  service/                  # 业务逻辑层
  repository/               # 数据访问层（GORM + Redis TokenCache）
  model/
    entity/                 # GORM 实体（数据库映射）
    dto/                    # 请求/响应 DTO（与实体解耦）
  middleware/               # JWT 强制/可选鉴权、Panic Recovery
  router/                   # 路由注册（公开 vs 鉴权分组）
  errcode/                  # 统一错误码 + ServiceError 类型
pkg/
  snowflake/  jwt/  hash/   # 可复用工具包
  cursor/                   # 游标编解码（base64+JSON，含 SnapshotCursor）
  response/                 # 统一响应格式 {code, msg, data}
  cache/                    # Cache-Aside 工具（GetJSON/SetJSON/SetNil/RandomizedTTL）
  redisclient/              # Redis 客户端工厂（带 Ping 验证）
  redislock/                # 分布式锁（SETNX + UUID + Lua 原子释放）
```

---

## 接口总览（共 18 个）

| 模块 | 方法 | 路径 | 认证 |
|---|---|---|---|
| 用户 | POST | /api/v1/user/register | — |
| 用户 | POST | /api/v1/user/login | — |
| 用户 | GET | /api/v1/user/:id | — |
| 用户 | GET | /api/v1/user/me | JWT |
| 用户 | POST | /api/v1/user/logout | JWT |
| 视频 | POST | /api/v1/video/publish | JWT |
| 视频 | GET | /api/v1/video/:id | — |
| 视频 | GET | /api/v1/video/list | — |
| Feed | GET | /api/v1/feed?type=latest | — |
| Feed | GET | /api/v1/feed?type=following | JWT |
| Feed | GET | /api/v1/feed?type=popularity | — |
| Feed | GET | /api/v1/feed?type=like_count | — |
| 点赞 | POST | /api/v1/like/action | JWT |
| 点赞 | GET | /api/v1/like/list | JWT |
| 评论 | POST | /api/v1/comment/action | JWT |
| 评论 | GET | /api/v1/comment/list | — |
| 关注 | POST | /api/v1/follow/action | JWT |
| 关注 | GET | /api/v1/follow/following | — |
| 关注 | GET | /api/v1/follow/follower | — |

---

## Redis 架构设计（阶段二）

### Key 布局

```
user:token:{uid}              → 当前登录 token（TTL = JWT 剩余有效期）
user:info:{uid}               → 用户信息 JSON（TTL 10min ± 30s）
video:detail:{id}             → 视频详情 JSON（TTL 5min ± 30s）
feed:following:{uid}:{cursor} → 关注流 video_id 列表（TTL 60s）
snapshot:{type}:current       → 热榜当前 epoch（string，永不过期）
snapshot:{type}:v{epoch}      → 热榜 ZSet：member=video_id, score=heat（TTL 15min）
lock:video:detail:{id}        → 视频详情回源锁（TTL 3s）
lock:user:info:{id}           → 用户信息回源锁（TTL 3s）
```

> `{type}` 取值：`popularity`（按 heat）、`like_count`（按点赞数）

### 视频详情 Cache-Aside + 分布式锁（防击穿）

```
GET /video/:id
        │
        ▼
cache.GetJSON("video:detail:{id}")
        │ 命中 ──────────────────────────────────────▶ 返回
        │ miss
        ▼
redislock.TryLock("lock:video:detail:{id}", TTL=3s)
        ├─ 拿到锁（loader）
        │       │
        │       ▼
        │   DB: FindByID
        │       ├─ 找到 ──▶ SetJSON(TTL=5min±30s) ──▶ Unlock ──▶ 返回
        │       └─ 不存在 ▶ SetNil(TTL=30s，防穿透) ──▶ Unlock ──▶ 404
        │
        └─ 未拿到（waiter）
                │
                ▼
        轮询 GetJSON（每 20ms，最多 500ms）
                │ 命中 ──▶ 返回
                │ 超时 ──▶ 503
```

同一时刻 N 个请求同一个 key 冷缓存：**只有 1 个 goroutine 打 DB**，其余等待缓存写回后读 Redis。

### 热榜滑动窗口快照（防翻页重复/漏数）

```
SnapshotService（后台 goroutine，每 1 分钟）
        │
        ▼
DB: SELECT TOP 1000 WHERE status=1 ORDER BY heat DESC
        │
        ▼
Pipeline ZADD snapshot:popularity:v{epoch}  ← score=heat, member=video_id
        │
        ▼
EXPIRE snapshot:popularity:v{epoch} 15min
        │
        ▼
SET snapshot:popularity:current = epoch     ← 最后更新指针，保证原子性

───────────────────────────────────────────────────────────

GET /feed?type=popularity
        │  cursor="" (第 1 页)
        ▼
GET snapshot:popularity:current → epoch=1000
        │
        ▼
ZREVRANGE snapshot:popularity:v1000  0  limit
        │
        ▼
MGET video:detail:{id} ...（命中直接返回，miss → DB + 回填）
        │
        ▼
返回 videos + nextCursor = base64{v:1000, o:limit}

        │  cursor=base64{v:1000, o:10}（第 2 页）
        ▼
检查 EXISTS snapshot:popularity:v1000
        ├─ 存在  ──▶ ZREVRANGE offset=10，继续读同一快照（榜单稳定）
        └─ 已过期 ──▶ 回退到 current epoch，offset=0（透明重置）
```

**关键保证**：翻页期间点赞/评论导致排名变动，不会影响当前用户正在翻的这一份快照。

### Token 双写（Redis 优先，DB 兜底）

```
登录
  ▶ pkgjwt.Sign(uid)
  ▶ Redis SET user:token:{uid}  TTL=7d   （best-effort）
  ▶ DB  UPDATE users SET token=...       （source of truth）
        └─ 失败 → 回滚 Redis DEL

中间件验证（每次请求）
  ▶ Redis GET user:token:{uid}
        ├─ 命中且匹配  ──▶ 放行（不走 DB）
        ├─ 命中不匹配  ──▶ 401（已登出或被顶替）
        └─ miss/错误
              ▶ DB FindTokenByUserID
                    ├─ 匹配  ──▶ 回填 Redis（TTL=JWT 剩余有效期）──▶ 放行
                    └─ 不匹配 ──▶ 401

登出
  ▶ Redis DEL user:token:{uid}
  ▶ DB  UPDATE users SET token=""
```

---

## 核心设计亮点

### 1. Feed 流策略模式（Strategy Pattern）

`FeedFetcher` 接口将不同 Feed 算法封装为独立策略，每个 fetcher 自己持有游标的 encode/decode 逻辑：

```go
type FeedFetcher interface {
    Type() string
    Fetch(ctx context.Context, rawCursor string, limit int) (
        videos []*entity.Video, nextCursor string, hasMore bool, err error)
}
```

目前已实现四种策略：

| 策略 | 类型 | cursor 格式 | 数据来源 |
|---|---|---|---|
| `LatestFetcher` | 全站最新 | `(created_at, id)` | MySQL |
| `FollowingFetcher` | 关注流 | `(created_at, id)` | Redis ID 列表 + 视频缓存 |
| `SnapshotFetcher` | 热榜/点赞榜 | `(epoch, offset)` | Redis ZSet 快照 |

新增策略只需实现接口并在 `main.go` 注册，无需修改已有代码（开闭原则）。

### 2. 游标分页（Keyset Pagination）

使用 `(score, id)` 复合游标代替 `OFFSET`，避免深翻页全表扫描：

```sql
WHERE (created_at < ? OR (created_at = ? AND id < ?))
ORDER BY created_at DESC, id DESC LIMIT ?
```

游标用 base64+JSON 编码为不透明字符串，前端不感知字段语义。

### 3. 分布式锁（SETNX + Lua）

```go
// TryLock：单次 SETNX，返回 UUID token
// Unlock：Lua 原子比对 token 后删除（防误删他人持有的锁）
// Do：封装 Cache-Aside 防击穿——持锁者回源写缓存，等待者轮询缓存
```

Unlock Lua 脚本保证"比对 + 删除"原子性：即使持锁者因超时丢锁，也不会删掉新持锁者的 key。

### 4. 缓存三件套

| 问题 | 解法 |
|---|---|
| **缓存穿透** | DB miss 写 `__nil__` 占位（TTL 30s），后续请求直接返回 404 |
| **缓存雪崩** | `RandomizedTTL(base ± jitter)` 打散过期时间 |
| **缓存击穿** | `redislock.Do()` 序列化回源——只有 1 个 goroutine 查 DB |

### 5. 热度评分系统

在视频表维护 `heat` 字段，与计数更新在同一 MySQL 事务中原子完成：
- 点赞/取消点赞：`heat ± 1`（与 `like_count ± 1` 同事务）
- 发评论/删评论：`heat ± 1`（与 `comment_count ± 1` 同事务）

### 6. 事务保证计数一致性

涉及计数的操作全部在 MySQL 事务内原子完成：
- 点赞事务：upsert video_likes + `videos.like_count ±1` + `videos.heat ±1`
- 评论事务：insert comment + `videos.comment_count ±1` + `videos.heat ±1`
- 关注事务：upsert follows + `users.follow_count ±1` + `users.follower_count ±1`

### 7. 软删除 + 唯一键 Upsert

点赞、关注均用软删除（`deleted_at`）配合 Unscoped 实现幂等 Upsert：
- 先 `Unscoped().First()` 查找（包含软删除行）
- 不存在 → INSERT；已软删除 → UPDATE `deleted_at=NULL`；存在且有效 → 返回业务错误

### 8. 雪花 ID + JSON 字符串化

主键统一使用雪花算法生成 int64（趋势递增，B+ 树插入友好）。JavaScript Number 只有 53 位精度，64 位 ID 在 JSON 响应中序列化为字符串：

```go
ID int64 `json:"id,string"`
```

### 9. 分层错误传播

Repository 层定义 sentinel error → Service 层用 `errors.Is` 转换为 `ServiceError{Code}` → Handler 层统一映射为 HTTP 响应，三层职责清晰，不跨层暴露底层细节。

### 10. 关注流拉模式（Pull Mode）

关注流采用**读时聚合**：用户请求 Feed 时实时执行 IN 子查询，取关注列表中所有人的最新视频并合并排序。写视频时只写 videos 表，不做推送扇出，适合当前规模。阶段三将引入 Kafka 实现推拉结合（大 V 走拉，普通用户走推）。

---

## 数据库设计要点

- 主键全部使用 `BIGINT UNSIGNED` 雪花 ID，无自增
- 时间字段使用毫秒时间戳（`DATETIME(3)`），GORM `autoCreateTime:milli`
- 软删除字段 `deleted_at DATETIME(3) NULL`
- 复合索引配合游标查询：`idx_author_created`、`idx_created`、`idx_heat`、`idx_like_count`
- follows 表单表双索引（`follower→followee` + `followee→follower`）解决双向查询

---

## 演进路线

| 阶段 | 内容 | 状态 |
|---|---|---|
| 阶段一 | 纯 MySQL，跑通所有业务接口（18 个 API，5 张表） | **已完成** |
| 阶段二 | Redis Cache-Aside、分布式锁防击穿、ZSet 热榜快照、Token 双写 | **已完成** |
| 阶段三 | Kafka 异步点赞落库、视频发布写扩散、推拉结合关注流 | 待实现 |
