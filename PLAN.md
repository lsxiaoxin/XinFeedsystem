# 视频 Feed 流系统 — 技术选型与数据库设计

## 快速启动

```bash
# 1. 启动 MySQL（首次需拉取镜像，约需 1 分钟）
make up
# 等价于：docker compose -f deploy/docker-compose.yaml up -d

# 2. 启动服务（直接命令行，无需容器）
make run
# 等价于：go run ./cmd/server

# 3. 验证
curl http://localhost:8080/healthz
# → {"status":"ok"}

# 4. 开发热重载（需先 go install github.com/cosmtrek/air@latest）
air

# 5. 停止依赖中间件
make down
```

> 环境要求：Go 1.21+，Docker（用于依赖中间件），MySQL 通过 docker-compose 管理。  
> 配置文件：`config/config.yaml`，MySQL 默认用户 `xfs / xfs123`，库名 `xinfeedsystem`。

## Context

从零搭建一个 Go 视频 Feed 流学习项目，**纯后端 API**，目标定位是简历项目。功能覆盖：用户、视频上传/播放、推荐流 + 关注流、点赞、评论、关注。

核心演进路线（**分三阶段，不要一次写完**）：
- **阶段 1**：纯 MySQL 跑通业务闭环
- **阶段 2**：引入 Redis（缓存 / 计数器 / 限流）
- **阶段 3**：引入 Kafka（点赞异步落库 / Feed 推拉结合）

本文件是技术选型 + 数据库 + 路线图蓝图，**实施分阶段执行**。

工作目录：`/home/xin/all/workspace/XinFeedsystem`。

---

## 1. 技术选型

| 类别 | 选型 | 理由 |
|---|---|---|
| 语言 | Go 1.21+ | 用户指定 |
| Web 框架 | `gin-gonic/gin` | 性能好、生态成熟、简历主流 |
| ORM | `gorm.io/gorm` + `gorm.io/driver/mysql` | 钩子/软删除/关联完备 |
| 配置 | `spf13/viper` | yaml + env 双源 |
| 日志 | `go.uber.org/zap` | 结构化、高性能 |
| JWT | `golang-jwt/jwt/v5` | 官方维护分支 |
| 参数校验 | `go-playground/validator/v10`（通过 `c.ShouldBindJSON` 触发） | Gin 自带集成；**所有接口统一用 `c.ShouldBindJSON(&req)` 做参数绑定 + 校验** |
| ID 生成 | `bwmarrin/snowflake` 雪花 ID | 趋势递增对 B+ 树友好；int8 比 UUID 小 |
| 密码 Hash | `golang.org/x/crypto/bcrypt` | 自带盐、可调 cost |
| API 文档 | `swaggo/swag` + `gin-swagger` | 注解生成 Swagger UI |
| 热重载 | `cosmtrek/air` | 开发期文件变更自动重启 |
| Redis 客户端（阶段 2） | `redis/go-redis/v9` | 官方推荐；Pipeline/Lua/Pub-Sub 完整 |
| Kafka 客户端（阶段 3） | `segmentio/kafka-go` | 纯 Go 无 CGO，部署简单 |
| 限流（阶段 2） | `golang.org/x/time/rate` + Redis Lua | 单机令牌桶 + 分布式 Lua |
| 测试 | `testing` + `stretchr/testify` | 标准 + 断言 |

**容器化策略**：
- `docker-compose.yaml` 一键拉起 mysql / redis / kafka 等依赖环境
- 应用本身**命令行直接 `go run` 启动**，不打包进容器
- 提供 `Makefile` 简化常用命令

---

## 2. 数据库设计（MySQL 8.0，InnoDB，utf8mb4_0900_ai_ci）

通用约定：
- 主键 `id BIGINT UNSIGNED`（雪花 ID）
- 时间统一 `DATETIME(3)`
- 软删除 `deleted_at DATETIME(3) NULL`（GORM 约定）

### 2.1 users 用户表

| 字段 | 类型 | NULL | 说明 |
|---|---|---|---|
| id | BIGINT UNSIGNED | NOT NULL | 雪花 ID |
| username | VARCHAR(32) | NOT NULL | 登录名，唯一 |
| password_hash | VARCHAR(72) | NOT NULL | bcrypt 输出 60 字节 |
| nickname | VARCHAR(32) | NOT NULL | 展示名 |
| avatar | VARCHAR(255) | NULL | 头像 URL |
| signature | VARCHAR(140) | NULL | 个性签名 |
| follow_count | INT UNSIGNED | NOT NULL DEFAULT 0 | 关注数（冗余） |
| follower_count | INT UNSIGNED | NOT NULL DEFAULT 0 | 粉丝数（冗余） |
| created_at / updated_at / deleted_at | DATETIME(3) | — | — |

索引：
- PRIMARY KEY (`id`)
- UNIQUE KEY `uk_username` (`username`)

### 2.2 videos 视频表

| 字段 | 类型 | NULL | 说明 |
|---|---|---|---|
| id | BIGINT UNSIGNED | NOT NULL | 雪花 ID |
| author_id | BIGINT UNSIGNED | NOT NULL | 作者 |
| title | VARCHAR(128) | NOT NULL | — |
| play_url | VARCHAR(512) | NOT NULL | 本地存储相对路径 |
| cover_url | VARCHAR(512) | NOT NULL | 封面 |
| duration | INT UNSIGNED | NOT NULL DEFAULT 0 | 秒 |
| like_count | INT UNSIGNED | NOT NULL DEFAULT 0 | 冗余 |
| comment_count | INT UNSIGNED | NOT NULL DEFAULT 0 | 冗余 |
| play_count | BIGINT UNSIGNED | NOT NULL DEFAULT 0 | 冗余 |
| status | TINYINT | NOT NULL DEFAULT 1 | 0审核 1正常 2下架 |
| created_at / updated_at / deleted_at | DATETIME(3) | — | — |

索引：
- PRIMARY KEY (`id`)
- KEY `idx_author_created` (`author_id`, `created_at` DESC) — 作者主页 + 关注流拉模式核心
- KEY `idx_created` (`created_at` DESC, `id` DESC) — 推荐流游标分页

**冗余字段策略**：强烈建议冗余计数字段。视频列表必然展示，`COUNT(*)` 关联千万行会爆。一致性：阶段 1 用事务保证；阶段 2 走 Redis 计数 + 定时回写；阶段 3 用 Kafka 异步落库。

### 2.3 video_likes 视频点赞表（**专表**）

| 字段 | 类型 | NULL | 说明 |
|---|---|---|---|
| id | BIGINT UNSIGNED | NOT NULL | — |
| user_id | BIGINT UNSIGNED | NOT NULL | 点赞用户 |
| video_id | BIGINT UNSIGNED | NOT NULL | 被点赞视频 |
| created_at | DATETIME(3) | NOT NULL | — |
| deleted_at | DATETIME(3) | NULL | 软删除（取消点赞） |

索引：
- PRIMARY KEY (`id`)
- UNIQUE KEY `uk_user_video` (`user_id`, `video_id`) — 防重复点赞 + upsert
- KEY `idx_video` (`video_id`, `created_at`) — 查"谁点了这个视频"

**点赞/取消语义**：取消点赞用软删（`deleted_at` 置非空），再次点赞 update `deleted_at=NULL`，避免唯一键冲突。

> 评论点赞如未来需要再加 `comment_likes` 表，结构同理。

### 2.4 comments 评论表

二级展平（一级评论 + 回复，对齐抖音/B 站交互）：

| 字段 | 类型 | NULL | 说明 |
|---|---|---|---|
| id | BIGINT UNSIGNED | NOT NULL | — |
| video_id | BIGINT UNSIGNED | NOT NULL | — |
| user_id | BIGINT UNSIGNED | NOT NULL | 评论者 |
| parent_id | BIGINT UNSIGNED | NOT NULL DEFAULT 0 | 0=一级评论 |
| root_id | BIGINT UNSIGNED | NOT NULL DEFAULT 0 | 所属一级评论 ID |
| reply_to_user_id | BIGINT UNSIGNED | NOT NULL DEFAULT 0 | @谁 |
| content | VARCHAR(1000) | NOT NULL | — |
| like_count | INT UNSIGNED | NOT NULL DEFAULT 0 | 冗余 |
| created_at / deleted_at | DATETIME(3) | — | — |

索引：
- PRIMARY KEY (`id`)
- KEY `idx_video_root_created` (`video_id`, `root_id`, `created_at`)
- KEY `idx_user_created` (`user_id`, `created_at`)

**查询模式**：先查 `root_id=0` 的一级评论 LIMIT 10，再 `IN` 批量查每条的前 N 条回复。

### 2.5 follows 关注表

| 字段 | 类型 | NULL | 说明 |
|---|---|---|---|
| id | BIGINT UNSIGNED | NOT NULL | — |
| follower_id | BIGINT UNSIGNED | NOT NULL | 发起关注的人 |
| followee_id | BIGINT UNSIGNED | NOT NULL | 被关注的人 |
| created_at / deleted_at | DATETIME(3) | — | — |

索引：
- PRIMARY KEY (`id`)
- UNIQUE KEY `uk_follower_followee` (`follower_id`, `followee_id`)
- KEY `idx_followee_follower` (`followee_id`, `follower_id`) — 反向查"谁关注我"

**双向查询**：单表 + 双索引解决，不要建两张表。

### 2.6 feed_inbox 关注流收件箱（**阶段 3 才建**）

阶段 1/2 走拉模式不需要这张表；阶段 3 引入 Kafka 后才创建。生产实际多用 Redis ZSet，本项目两者都做以便对比。

| 字段 | 类型 | 说明 |
|---|---|---|
| id | BIGINT UNSIGNED | — |
| user_id | BIGINT UNSIGNED | 收件人（粉丝） |
| video_id | BIGINT UNSIGNED | — |
| author_id | BIGINT UNSIGNED | 视频作者 |
| publish_time | DATETIME(3) | 发布时间，用于排序 |

索引：PK + UNIQUE `uk_user_video` + `idx_user_publish (user_id, publish_time DESC)`

### 2.7 接口参数校验统一规范

**所有 POST / PUT / DELETE 接口的入参一律使用 `c.ShouldBindJSON(&req)` 进行绑定 + 校验**，禁止 `ShouldBind` 等混用（GET 接口用 `ShouldBindQuery` 绑定 query 参数）。

约定：
- 每个 handler 在 `internal/model/dto/` 定义 `XxxRequest` struct，字段加 `binding:"required,..."` tag
- handler 模板（统一写法）：
  ```go
  func (h *VideoHandler) Publish(c *gin.Context) {
      var req dto.VideoPublishRequest
      if err := c.ShouldBindJSON(&req); err != nil {
          response.Fail(c, errcode.InvalidParam, err.Error())
          return
      }
      // 调用 service ...
      response.OK(c, data)
  }
  ```
- 错误统一由 `response.Fail` 包装为 `{code: 40001, msg: "...", data: null}`

理由：单一校验入口便于全局错误处理 / 日志追踪；validator tag 驱动校验声明式更易维护。

### 2.8 关键设计要点

- **分页**：所有列表强制游标分页，`WHERE (created_at, id) < (?, ?) ORDER BY created_at DESC, id DESC LIMIT 10`，禁用 `OFFSET`
- **软删除 + 唯一键**：用户名复用问题暂不处理（学习项目可接受）
- **计数一致性**：阶段 1 用事务保证；点赞接口事务伪代码：
  ```
  BEGIN;
    INSERT video_likes ... ON DUPLICATE KEY UPDATE deleted_at = NULL;
    UPDATE videos SET like_count = like_count + 1 WHERE id = ?;
  COMMIT;
  ```

---

## 3. Feed 流算法（阶段 1，纯 MySQL）

### 3.1 推荐流（全站时间倒序）
```sql
SELECT id, author_id, title, play_url, cover_url,
       like_count, comment_count, created_at
FROM videos
WHERE status = 1
  AND (created_at, id) < (?, ?)   -- 游标
ORDER BY created_at DESC, id DESC
LIMIT 10;
```
走 `idx_created`。

### 3.2 关注流（**拉模式 Pull**）
```sql
SELECT v.* FROM videos v
WHERE v.author_id IN (
    SELECT followee_id FROM follows
    WHERE follower_id = ? AND deleted_at IS NULL
)
  AND v.status = 1
  AND (v.created_at, v.id) < (?, ?)
ORDER BY v.created_at DESC, v.id DESC
LIMIT 10;
```
走 `videos.idx_author_created`。

### 3.3 为什么阶段 1 只做拉模式
- 拉模式：读时聚合、写视频时只写 videos 表；简单
- 推模式（写扩散）：作者发视频要写入所有粉丝收件箱；同步实现会接口超时
- 阶段 3 加 Kafka 后才做**推拉结合**：粉丝数 < 阈值（如 10000）的作者走推；大 V 走拉

---

## 4. 项目分层结构

```
/home/xin/all/workspace/XinFeedsystem/
├── cmd/server/main.go                 # 入口
├── internal/                          # 不对外暴露
│   ├── api/                           # HTTP handler（绑定参数、调 service）
│   │   ├── user_handler.go
│   │   ├── video_handler.go
│   │   ├── feed_handler.go
│   │   ├── like_handler.go
│   │   ├── comment_handler.go
│   │   └── follow_handler.go
│   ├── service/                       # 业务逻辑
│   ├── repository/                    # 数据访问（GORM）
│   │   └── cache/                     # 阶段 2 加入
│   ├── model/
│   │   ├── entity/                    # GORM 实体
│   │   └── dto/                       # 请求/响应
│   ├── middleware/                    # jwt / cors / trace / ratelimit / recovery
│   ├── router/router.go
│   ├── mq/                            # 阶段 3 加入
│   └── errcode/                       # 统一错误码
├── pkg/                               # 可复用工具
│   ├── snowflake/  jwt/  hash/  logger/  response/
├── config/
│   ├── config.yaml
│   └── config.go
├── docs/                              # swag 生成
├── scripts/mysql/init.sql             # DDL
├── deploy/docker-compose.yaml         # mysql + redis(阶段2) + kafka(阶段3)
├── storage/                           # 本地视频存储（gitignore）
│   ├── videos/  covers/
├── .air.toml
├── Makefile
├── go.mod
└── README.md
```

---

## 5. 三阶段实施路线图

### 阶段 1 — MySQL only（先跑通业务）

**接口清单**：

| 模块 | 方法 | 路径 | 核心实现 |
|---|---|---|---|
| 用户 | POST | /api/v1/user/register | INSERT users + bcrypt |
| 用户 | POST | /api/v1/user/login | bcrypt 比对 + 签发 JWT |
| 用户 | GET | /api/v1/user/:id | SELECT users |
| 视频 | POST | /api/v1/video/publish | multipart 接收，落盘 storage/videos/{snowflake}.mp4，INSERT videos |
| 视频 | GET | /api/v1/video/:id | SELECT videos |
| 视频 | GET | /api/v1/video/list?user_id= | 作者视频列表，游标分页 |
| Feed | GET | /api/v1/feed/recommend?cursor= | 见 3.1 |
| Feed | GET | /api/v1/feed/follow?cursor= | 见 3.2 |
| 点赞 | POST | /api/v1/like/action | 事务：upsert video_likes + UPDATE videos.like_count |
| 点赞 | GET | /api/v1/like/list | 我点赞过的视频 |
| 评论 | POST | /api/v1/comment/action | 事务：INSERT comments + UPDATE videos.comment_count |
| 评论 | GET | /api/v1/comment/list?video_id= | 两次查询：一级 + IN 二级 |
| 关注 | POST | /api/v1/follow/action | 事务：INSERT follows + UPDATE 双方计数 |
| 关注 | GET | /api/v1/follow/following | 关注列表 |
| 关注 | GET | /api/v1/follow/follower | 粉丝列表 |

**Done 条件**：所有接口能用 Swagger UI 自测通过；`go test` 通过。

### 阶段 2 — 引入 Redis

接口完全兼容阶段 1，仅替换 repository 内部实现。

1. **缓存对象（Cache-Aside）**：`user:info:{id}` / `video:info:{id}`，TTL 30min。读：先 Redis miss 后回源；写：先更新 DB 再删 Cache
2. **计数器**（Redis 为权威源）：`counter:video:like:{vid}` / `counter:video:comment:{vid}` / `counter:user:follower:{uid}`。点赞接口：Redis INCR + 异步落 DB
3. **热点 Feed**：`feed:recommend:page1` ZSet 缓存最新 100 条 video_id（score=created_at），TTL 1min
4. **三大缓存问题防护**：穿透（null 占位 + BloomFilter）/ 击穿（singleflight + SETNX）/ 雪崩（TTL 随机偏移 ±5min）
5. **限流**：Lua 脚本固定窗口 / 令牌桶，按 user + action 维度
6. **关注关系判断**：`follow:set:{follower_id}` SISMEMBER O(1)

**Done 条件**：阶段 1 接口在 Redis 缓存下功能等价；README 文档对比 QPS。

### 阶段 3 — 引入 Kafka

1. **异步化操作**
   - 点赞/评论计数落 DB：API 写 Redis 计数 → Kafka 消息 → consumer 批量落表
   - 视频发布写扩散：发布接口发 `xfs.video.published` → consumer 推到粉丝 Redis ZSet `feed:inbox:{follower_id}`
2. **推拉结合关注流**：读自己收件箱（小 V 推来的） + 拉模式查关注的大 V → 归并排序；大 V 阈值粉丝 ≥ 10000
3. **可靠性**：生产者 `acks=all` + 幂等；消费者手动提交 offset；业务幂等靠 `uk_user_video` 唯一索引；失败 N 次入死信 topic
4. **Topic 设计**：`xfs.like.action`（3 分区）/ `xfs.comment.action`（3 分区）/ `xfs.video.published`（3 分区，按 author_id hash）

**Done 条件**：发布视频后粉丝关注流秒级可见；点赞接口 P99 < 10ms。

---

## 6. 关键文件路径（实施时创建）

- `cmd/server/main.go` — 入口装配
- `internal/router/router.go` — 路由注册
- `internal/model/entity/*.go` — GORM 实体
- `internal/service/feed_service.go` — Feed 核心
- `internal/repository/video_repo.go` — 视频 DAO
- `scripts/mysql/init.sql` — DDL（按 §2 设计）
- `config/config.yaml` — 配置
- `deploy/docker-compose.yaml` — mysql/redis/kafka 一键起
- `Makefile` — `make up` / `make run` / `make swag`
- `pkg/snowflake/snowflake.go` — ID 生成

---

## 7. 验证方式

### 阶段 1
- `make up`（docker-compose 起 mysql）→ `go run ./cmd/server` 启动服务
- 浏览器打开 `http://localhost:8080/swagger/index.html` 查看 API 文档
- 用 Swagger UI 依次自测每个接口：注册 → 登录拿 token → 上传视频 → 推荐流可见 → 点赞 → 关注 → 关注流可见 → 评论
- MySQL CLI 抽查计数一致性：`videos.like_count` 应 = `SELECT COUNT(*) FROM video_likes WHERE video_id=? AND deleted_at IS NULL`

### 阶段 2
- `make up` 起 redis；切换 repository 实现
- 重复阶段 1 测试，业务等价
- 抽查：Redis `GET counter:video:like:{vid}` 与 DB 计数最终一致

### 阶段 3
- `make up` 起 kafka；切换点赞/发布路径为发消息
- 验证：发布视频后小 V 粉丝的 `feed:inbox:{uid}` 秒级出现 video_id
- 用 `kafka-console-consumer` 抽查 topic 消息体；故意杀掉 consumer 验证消息不丢

---

## 不做的事（避免堆砌）

- 微服务拆分 / gRPC / 服务注册发现
- ES 全文搜索
- 推荐算法（协同过滤等）
- 自研 RPC / 框架
- k6 压测、GitHub Actions CI
- 应用容器化（应用直接 `go run`，仅依赖中间件用 docker-compose）
