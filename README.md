# XinFeedSystem — 视频 Feed 流后端系统

> 纯后端 Go 项目，涵盖用户、视频、Feed 流、点赞、评论、关注六大模块。分三阶段演进（MySQL → Redis → Kafka），当前完成阶段一。

---

## 技术栈

| 类别 | 选型 |
|---|---|
| 语言 | Go 1.21+ |
| Web 框架 | Gin |
| ORM | GORM + MySQL 8.0 |
| 认证 | JWT（HS256，7 天有效期） |
| ID 生成 | 雪花算法（Snowflake） |
| 密码安全 | bcrypt（cost=10，自带盐） |
| 配置 | Viper（yaml + 环境变量） |
| 日志 | Zap（结构化日志） |
| 容器化依赖 | Docker Compose（MySQL 单容器） |

---

## 快速启动

```bash
# 1. 启动 MySQL
make up

# 2. 启动服务（debug 模式自动 AutoMigrate 建表）
make run

# 3. 验证
curl http://localhost:8080/healthz
# → {"status":"ok"}

# 4. 热重载开发
air
```

配置文件：`config/config.yaml`，MySQL 用户 `xfs / xfs123`，库名 `xinfeedsystem`。

---

## 项目分层结构

```
cmd/server/main.go          # 依赖装配入口（DI 手动注入）
internal/
  api/                      # Handler 层：参数绑定 → 调 Service → 返回 JSON
  service/                  # 业务逻辑层
  repository/               # 数据访问层（GORM）
  model/
    entity/                 # GORM 实体（数据库映射）
    dto/                    # 请求/响应 DTO（与实体解耦）
  middleware/               # JWT 强制/可选鉴权、Panic Recovery
  router/                   # 路由注册（公开 vs 鉴权分组）
  errcode/                  # 统一错误码 + ServiceError 类型
pkg/
  snowflake/  jwt/  hash/   # 可复用工具包
  cursor/                   # 游标编解码（base64+JSON）
  response/                 # 统一响应格式 {code, msg, data}
```

---

## 接口总览（阶段一，共 15 个）

| 模块 | 方法 | 路径 | 认证 |
|---|---|---|---|
| 用户 | POST | /api/v1/user/register | — |
| 用户 | POST | /api/v1/user/login | — |
| 用户 | GET | /api/v1/user/:id | — |
| 用户 | GET | /api/v1/user/me | JWT |
| 视频 | POST | /api/v1/video/publish | JWT |
| 视频 | GET | /api/v1/video/:id | — |
| 视频 | GET | /api/v1/video/list | — |
| Feed | GET | /api/v1/feed?type=latest | — |
| Feed | GET | /api/v1/feed?type=following | JWT |
| Feed | GET | /api/v1/feed?type=popularity | — |
| 点赞 | POST | /api/v1/like/action | JWT |
| 点赞 | GET | /api/v1/like/list | JWT |
| 评论 | POST | /api/v1/comment/action | JWT |
| 评论 | GET | /api/v1/comment/list | — |
| 关注 | POST | /api/v1/follow/action | JWT |
| 关注 | GET | /api/v1/follow/following | — |
| 关注 | GET | /api/v1/follow/follower | — |

---

## 核心设计亮点（简历要点）

### 1. Feed 流策略模式（Strategy Pattern）

定义 `FeedFetcher` 接口，将不同 Feed 算法封装为独立策略：

```go
type FeedFetcher interface {
    Type()    string
    Fetch(ctx context.Context, score, cursorID int64, limit int) ([]*entity.Video, error)
    ScoreOf(v *entity.Video) int64
}
```

目前已实现三种策略：
- **LatestFetcher** — 全站时间倒序（score = created_at）
- **FollowingFetcher** — 关注流拉模式，IN 子查询（score = created_at）
- **PopularityFetcher** — 热榜，按 heat 字段倒序（score = heat）

新增策略只需实现接口并在 `main.go` 注册，无需修改已有代码（开闭原则）。

### 2. 游标分页（Keyset Pagination）

使用 `(score, id)` 复合游标代替 `OFFSET`，避免深翻页全表扫描：

```sql
WHERE (created_at < ? OR (created_at = ? AND id < ?))
ORDER BY created_at DESC, id DESC
LIMIT ?
```

游标用 base64+JSON 编码为不透明字符串，前端不感知字段语义；不同 Feed 策略的 score 字段由 `ScoreOf()` 解耦，FeedService 统一处理分页逻辑。

### 3. 热度评分系统

在视频表新增 `heat` 字段作为实时热度指标，在同一个数据库事务中同步更新：
- 用户**点赞**时：`heat + 1`（与 like_count +1 在同一事务）
- 用户**发评论**时：`heat + 1`（与 comment_count +1 在同一事务）

热榜 Feed 按 `heat DESC, id DESC` 排序，复合游标保证翻页无重复。

### 4. 事务保证计数一致性

涉及计数的操作全部在 MySQL 事务内原子完成，杜绝计数与数据不一致：
- 点赞事务：upsert video_likes + videos.like_count ±1 + videos.heat +1
- 评论事务：insert comment + videos.comment_count ±1 + videos.heat +1
- 关注事务：upsert follows + users.follow_count ±1 + users.follower_count ±1

### 5. 软删除 + 唯一键 Upsert

点赞、关注均用软删除（`deleted_at`）而非物理删除，配合 GORM Unscoped 查询实现 Upsert 语义：
- 先 `Unscoped().First()` 查找（包含已软删除行）
- 记录不存在 → INSERT；已软删除 → UPDATE deleted_at=NULL；存在且有效 → 返回业务错误

避免了 ON DUPLICATE KEY UPDATE 在 GORM 层的兼容性问题，同时支持重新点赞/重新关注。

### 6. 雪花 ID + JSON 字符串化

主键统一使用雪花算法生成 int64 ID（趋势递增，B+ 树插入友好）。由于 JavaScript Number 只有 53 位精度，所有 64 位 ID 在 JSON 响应中序列化为字符串：

```go
ID int64 `json:"id,string"`
```

### 7. 分层错误传播

Repository 层定义 sentinel error（`ErrAlreadyLiked`、`ErrNotFollowedYet` 等）。Service 层用 `errors.Is` 识别后转换为 `ServiceError{Code}` 业务错误。Handler 层统一调用 `handleSvcError(c, err)` 映射为 HTTP 响应，三层职责清晰，不跨层暴露底层细节。

### 8. 关注流拉模式（Pull Mode）

关注流采用**读时聚合**：用户请求 Feed 时实时执行 IN 子查询，取关注列表中所有人的最新视频并合并排序。写视频时只写 videos 表，不做推送扇出，适合阶段一低并发场景。阶段三将引入 Kafka 实现推拉结合（大 V 走拉，普通用户走推）。

---

## 数据库设计要点

- 主键全部使用 `BIGINT UNSIGNED` 雪花 ID，无自增
- 时间字段使用毫秒时间戳（`DATETIME(3)`），GORM `autoCreateTime:milli`
- 软删除字段 `deleted_at DATETIME(3) NULL`
- 复合索引配合游标查询：`idx_author_created(author_id, created_at DESC)`、`idx_created(created_at DESC, id DESC)`、`idx_heat(heat DESC)`
- follows 表单表双索引（`follower→followee` + `followee→follower`）解决双向查询

---

## 演进路线

| 阶段 | 内容 | 状态 |
|---|---|---|
| 阶段一 | 纯 MySQL，跑通所有业务接口 | **已完成** |
| 阶段二 | Redis 缓存（Cache-Aside）、计数器、限流、singleflight 防击穿 | 待实现 |
| 阶段三 | Kafka 异步点赞落库、视频发布写扩散、推拉结合关注流 | 待实现 |
