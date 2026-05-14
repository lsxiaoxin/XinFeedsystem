# 简历项目描述

## 一句话描述（填项目名称栏）

> 基于 Go + Gin + MySQL 的视频 Feed 流后端系统，实现用户、视频、Feed 流、点赞、评论、关注六大模块，共 17 个 RESTful 接口。

---

## 项目描述（100字左右，填简历正文）

设计并实现了一个类抖音视频 Feed 流后端服务。采用 **Go + Gin + GORM + MySQL 8.0** 技术栈，手动依赖注入完成分层架构（Handler / Service / Repository）。核心亮点：用策略模式实现三种 Feed 算法（最新流、热榜、关注流）；用 `(score, id)` 复合游标代替 OFFSET 分页；所有计数字段（点赞数、评论数、粉丝数、热度）在 MySQL 事务中原子更新，保证计数与数据的强一致性；JWT 单会话鉴权，token 存入数据库，登出即失效。

---

## 技术栈（填技能标签）

`Go` `Gin` `GORM` `MySQL 8.0` `JWT` `雪花算法` `bcrypt` `Docker Compose` `RESTful API`

---

## 项目亮点（逐条填，每条对应一个面试考察点）

**1. Feed 流策略模式（Strategy Pattern）**
定义 `FeedFetcher` 接口，将三种 Feed 算法（最新流 / 热榜 / 关注流）封装为独立策略，注册到 `FeedService`。新增算法只需实现接口并注册，不修改任何已有代码，符合开闭原则。

**2. 游标分页（Keyset Pagination）取代 OFFSET**
全部列表接口采用 `(score, id)` 复合游标，SQL 转化为 `WHERE score < ? OR (score = ? AND id < ?)`，翻到第 N 页与翻第 1 页性能相同；游标用 base64+JSON 编码为不透明字符串下发给客户端，前端无需感知字段语义。

**3. 热度评分 + 热榜 Feed**
视频表新增 `heat` 字段，点赞 / 评论在同一事务内同步更新 `heat +1`，取消点赞 / 删除评论同步 `heat -1`（加 `> 0` 防负数）。热榜按 `heat DESC, id DESC` 排序，复合游标保证翻页无重复。

**4. 事务保证计数一致性**
点赞、评论、关注的计数更新全部在 MySQL 事务内原子完成（共 6 处事务），杜绝主表写入成功但计数未更新的数据不一致问题。

**5. 软删除 + Upsert 语义**
点赞 / 关注用软删除（`deleted_at`）代替物理删除，配合 `Unscoped().First()` 实现：不存在→INSERT、软删除→恢复、已存在→返回业务错误。避免 ON DUPLICATE KEY UPDATE 在 GORM 层的兼容问题，同时天然支持重新点赞 / 重新关注。

**6. JWT 单会话鉴权 + 安全登出**
登录时将 token 写入 `users.token` 列，中间件每次请求均与数据库存储值比对，异地新登录自动使旧 token 失效，登出调用 `ClearToken` 将列清空，实现真正意义的服务端登出。

**7. 分层错误传播**
Repository 层定义 sentinel error（`ErrAlreadyLiked` 等），Service 层用 `errors.Is` 转为 `ServiceError{Code}` 业务错误，Handler 层统一映射为 HTTP 响应。三层职责清晰，底层细节不跨层泄露。

**8. 雪花 ID + JS 安全序列化**
主键统一使用雪花算法生成趋势递增 int64，B+ 树插入友好。因 JavaScript Number 仅 53 位精度，所有 ID 在 JSON 中序列化为字符串（`json:"id,string"`），防止前端精度丢失。

---

## 数据库设计要点（面试时可展开）

- 5 张核心表：`users` / `videos` / `video_likes` / `comments` / `follows`
- 主键全部为 `BIGINT UNSIGNED` 雪花 ID，无自增
- 时间字段 `DATETIME(3)` 毫秒精度，GORM autoCreateTime:milli
- 复合索引配合游标：`idx_author_created(author_id, created_at DESC)`、`idx_created(created_at DESC, id DESC)`、`idx_heat(heat DESC, id DESC)`
- `follows` 表单表双索引解决双向查询（关注列表 / 粉丝列表）
- 软删除 + 唯一键组合：`video_likes(user_id, video_id)`、`follows(follower_id, followee_id)` 防重

---

## 系统演进路线（体现设计前瞻性）

| 阶段 | 内容 |
|---|---|
| 阶段一（已完成） | 纯 MySQL，17 个接口全部跑通 |
| 阶段二（规划中） | Redis：Cache-Aside 缓存、计数器、singleflight 防击穿、Lua 限流 |
| 阶段三（规划中） | Kafka：点赞异步落库、视频发布写扩散、推拉结合关注流（大 V 走拉） |

---

## GitHub 项目地址

`XinFeedSystem` — 见仓库根目录 `README.md`
