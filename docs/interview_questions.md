# 面试题库 — XinFeedSystem 项目

> 假设面试官已看过项目代码，按难度从基础到深入排列。

---

## 一、基础设计

### Q1：项目整体分层是怎样的？各层职责是什么？

**答：** 三层架构：
- **Handler（api/）**：参数绑定与校验（`c.ShouldBindJSON` / `c.ShouldBindQuery`）、调用 Service、将结果序列化为统一 JSON 响应。Handler 不含业务逻辑。
- **Service（service/）**：业务规则、错误转换（sentinel error → ServiceError）、DTO 与 Entity 转换。
- **Repository（repository/）**：纯粹的数据库操作，不关心业务含义；定义 sentinel error 供 Service 识别。

依赖方向单向：Handler → Service → Repository，任何层不反向引用上层。

---

### Q2：为什么用雪花算法生成 ID 而不是 MySQL 自增主键？

**答：** 三个理由：
1. **趋势递增**：自增 int64 对 B+ 树插入友好（避免页分裂），UUID 随机性差。
2. **分布式友好**：雪花 ID 不依赖数据库，未来分库分表不需要重新设计 ID 策略。
3. **JS 精度问题**：int64 最大值超过 JavaScript `Number.MAX_SAFE_INTEGER`（2^53-1），所以 JSON 序列化时加 `json:"id,string"` tag 以字符串传输，前端直接用，不会丢精度。

---

### Q3：密码是怎么存储的？为什么不用 MD5？

**答：** 用 `golang.org/x/crypto/bcrypt`，`GenerateFromPassword([]byte(password), 10)`。

MD5/SHA-256 是快速哈希，攻击者可以用显卡暴力枚举彩虹表。bcrypt 有三个优势：
1. **内置随机盐**：每次 hash 结果不同，彩虹表失效。
2. **算法慢**：cost 参数（默认 10）让每次计算需要数十毫秒，暴力破解成本极高。
3. **自描述**：hash 结果包含 cost + salt，`CompareHashAndPassword` 无需额外存盐。

---

### Q4：JWT 鉴权流程是怎样的？Token 过期怎么处理？

**答：** 
- 登录成功后，`jwt.Sign(userID)` 生成 HS256 token，有效期 7 天，包含 `user_id` + `exp` claim。
- 请求到达时，`JWTAuth()` 中间件从 `Authorization: Bearer <token>` 提取 token，用 `jwt.Parse` 验证签名和有效期。
- token 过期返回 `40102 TokenExpired`，伪造或格式错误返回 `40101 TokenInvalid`，客户端重新登录获取新 token。

当前没有 Refresh Token 机制（学习项目可接受），生产环境会加双 token（Access Token 短期 + Refresh Token 长期）。

---

## 二、核心功能

### Q5：游标分页（Keyset Pagination）和 OFFSET 分页有什么区别？为什么要用游标？

**答：** 
OFFSET 分页：`SELECT * FROM videos ORDER BY created_at DESC LIMIT 10 OFFSET 1000`。MySQL 需要扫描并丢弃前 1000 行，页码越深越慢，百万行时 P99 可能超秒。

游标分页：
```sql
WHERE (created_at < ? OR (created_at = ? AND id < ?))
ORDER BY created_at DESC, id DESC LIMIT 10
```
每次都从上次最后一条记录的位置开始，走索引范围扫描，无论第几页速度恒定。

**复合游标 `(created_at, id)` 的原因**：created_at 可能重复，只用时间会导致同一毫秒的视频被跳过或重复，加 id 做 tiebreaker 保证唯一定位。

游标用 `base64(json{s, i})` 编码为不透明字符串：对前端是黑盒，后端可以在不改接口的情况下修改游标字段语义（比如换成热度分）。

---

### Q6：Feed 策略模式是怎么设计的？扩展一种新 Feed 类型需要改哪些文件？

**答：** 定义 `FeedFetcher` 接口：
```go
type FeedFetcher interface {
    Type()    string
    Fetch(ctx, score, cursorID, limit) ([]*entity.Video, error)
    ScoreOf(v *entity.Video) int64
}
```
`FeedService` 维护一个 `map[string]FeedFetcher`，根据请求中的 `type` 参数路由到对应策略。分页逻辑（limit+1 判断 hasMore、编码 nextCursor）统一在 FeedService 里，各 Fetcher 只关心数据查询。

**扩展新策略只需两步**：
1. 新建一个 struct 实现三个接口方法（通常 10 行代码）
2. 在 `main.go` 的 `NewFeedService(...)` 调用中追加一个 fetcher 实例

不需要修改 FeedService、Handler、Router 任何现有代码——符合开闭原则。

---

### Q7：点赞事务是如何保证计数一致性的？如果事务中途失败会怎样？

**答：** 点赞操作的完整事务：
1. `Unscoped().First()` 查找 video_likes 记录（含软删除）
2. 首次 → INSERT；取消后重新点赞 → UPDATE deleted_at=NULL；已点赞 → 返回 ErrAlreadyLiked
3. `UPDATE videos SET like_count = like_count + 1 WHERE id = ?`
4. `UPDATE videos SET heat = heat + 1 WHERE id = ?`

全部在 `db.Transaction(func(tx) error {...})` 中执行。如果任何一步返回 error，GORM 自动 ROLLBACK，不会出现「like 记录插入成功但计数没更新」的脏状态。

**为什么用 `UPDATE column = column + 1` 而不是先 SELECT 再 UPDATE？** 避免并发下的读-改-写竞态（ABA 问题），数据库层面原子操作保证并发安全。

---

### Q8：软删除和唯一键如何配合？为什么不用 ON DUPLICATE KEY UPDATE？

**答：** video_likes 有 `UNIQUE KEY uk_user_video (user_id, video_id)`。如果用 `INSERT ... ON DUPLICATE KEY UPDATE deleted_at=NULL`，MySQL 会更新已有行，但 GORM 的 upsert 支持在不同 driver 下行为不一致，且无法精确控制哪些字段被更新。

现有做法：先 `Unscoped().First()` 查出记录（含软删除行），按结果分支处理：
- 不存在 → CREATE（触发 INSERT）
- 存在且 deleted_at 有值 → UPDATE deleted_at=NULL（精确恢复软删除）
- 存在且 deleted_at 为 NULL → 返回 ErrAlreadyLiked

这样逻辑清晰，每个分支意图明确，且不依赖数据库特定的 upsert 语法。

---

### Q9：关注流为什么用拉模式（Pull）而不是推模式（Push）？有什么局限性？

**答：** 
**拉模式**：用户请求关注流时，实时查询其关注的所有人最近的视频，IN 子查询聚合后排序返回。
- 优点：实现简单，写视频时只写一张表
- 缺点：关注人数多（>100）或粉丝量大时查询慢；IN 子查询随关注数线性增长

**推模式**：发布视频时，将视频 ID 写入所有粉丝的收件箱（feed_inbox 表或 Redis ZSet）。
- 优点：读 Feed 时只查自己的收件箱，O(1)
- 缺点：大 V 发一条视频可能要写百万行，同步推会导致发布接口超时

**当前阶段一选择拉模式的理由**：业务初期关注数量少，实现简单，优先跑通业务。阶段三会引入 Kafka 实现**推拉结合**：普通用户（粉丝 <10000）走推，大 V 走拉，读时归并排序。

---

### Q10：热度（heat）字段的更新策略是怎样的？有没有问题？

**答：** 当前策略：点赞 +1、发评论 +1，均在事务内原子更新，只增不减。

**优点**：实现极简，事务保证不丢更新，适合阶段一。

**潜在问题**：
1. **没有时间衰减**：3 年前的爆款视频会一直霸榜，新视频无法超越。生产环境会用衰减公式，比如 `score = likes * W1 + comments * W2 - age_in_hours * W3`，定时任务每小时重算。
2. **并发写瓶颈**：热点视频被大量点赞时，`UPDATE heat = heat + 1` 会引发行锁竞争。阶段二会改为 Redis INCR，批量异步回写 DB（Counter + Flush 模式）。
3. **取消点赞不扣分**：目前 unlike 不减 heat，使 heat 只能单调递增，是有意设计（衰减由时间因子处理）还是遗漏，需结合产品需求确认。

---

## 三、架构与扩展

### Q11：阶段二 Redis 缓存怎么做？会遇到哪些缓存问题？

**答：** 计划使用 Cache-Aside 模式：
- 读：先查 Redis，miss 则查 DB，结果写回 Redis（TTL 30min）
- 写：先更新 DB，再删除 Redis key（延迟双删防脏读）

**三大缓存问题及对策**：
- **缓存穿透**（查不存在的 key）：null 值占位（TTL 1min）+ BloomFilter 前置过滤
- **缓存击穿**（热点 key 过期瞬间大量请求打 DB）：`golang.org/x/sync/singleflight` 合并并发请求，只有一个打 DB
- **缓存雪崩**（大量 key 同时过期）：TTL 加随机偏移 ±5min 打散过期时间

计数器（like_count、heat）改用 Redis INCR 作为权威源，定时任务或 Kafka consumer 批量回写 DB，解决高并发写竞争。

---

### Q12：阶段三 Kafka 用在哪些场景？为什么不直接用 goroutine？

**答：** 两个核心场景：
1. **点赞/评论计数异步落库**：API 只写 Redis 计数，发 Kafka 消息，consumer 批量 UPDATE DB，点赞接口 P99 从 10ms 降至 1ms。
2. **视频发布写扩散**：发布时发 `video.published` 消息，consumer 将视频 ID 写入所有普通粉丝的 Redis ZSet 收件箱。

**为什么不用 goroutine + channel**：
- goroutine 在进程崩溃时消息丢失，没有持久化
- Kafka 消费者可手动提交 offset，失败可重试
- 支持多消费者组，未来可扩展到多个下游（统计、推荐、通知）
- 幂等性：消费者业务幂等靠 `UNIQUE KEY uk_user_video`，重复消费不产生脏数据

---

### Q13：这个系统目前有哪些性能瓶颈？如果 QPS 从 100 增长到 10000 怎么办？

**答：** 
**当前瓶颈**：
1. 每次读 Feed 都查 DB，无缓存
2. 关注流 IN 子查询，关注 500 人时子查询规模大
3. 计数更新串行在事务里，热点视频行锁争抢
4. 没有连接池以外的限流，DB 直接暴露

**10x QPS 应对方案**（阶段二）：
- 热点视频/用户信息加 Redis 缓存（命中率预估 >90%）
- 计数改 Redis INCR，去除点赞时的行锁
- 加令牌桶限流（`golang.org/x/time/rate`），保护 DB

**100x QPS 应对方案**（阶段三）：
- Kafka 解耦点赞写入，异步批量落 DB
- 关注流推拉结合，大 V 收件箱用 Redis ZSet 预填充
- MySQL 读写分离或分库分表（按 user_id 取模）

---

### Q14：项目中的错误处理机制是怎样的？

**答：** 三层错误传播链：

```
Repository → sentinel error (errors.New("already liked"))
     ↓ errors.Is
Service    → errcode.New(errcode.AlreadyLiked)  // *ServiceError{Code: 30001}
     ↓ errors.As
Handler    → handleSvcError(c, err)
               errors.As → response.Fail(c, 30001, "已点赞")
               其他 error → response.FailWithErr(c, 50000)  // 内部错误不泄露细节
```

**好处**：
- Repository 不依赖任何 HTTP 或业务代码，可复用
- Service 集中处理业务语义，错误码有明确含义
- Handler 只做映射，从不直接 `err.Error()` 暴露堆栈给客户端

---

### Q15：为什么 Feed 接口要做「OptionalAuth」而不是强制鉴权？

**答：** Feed 的 `latest` 和 `popularity` 类型是公开内容，不登录的用户也应该能看，强制鉴权会损害用户体验（访客直接拒之门外）。

`OptionalAuth` 中间件尝试解析 token：成功则将 user_id 注入 context；失败或 token 缺失时直接放行，不中断请求。

`following` 类型在 Handler 内检查 `GetUserID(c) == 0` 来判断是否已登录，未登录返回 401。这样同一个路由（`GET /api/v1/feed`）对所有类型统一，不需要把 following 单独拎到 auth 分组，路由设计更简洁。
