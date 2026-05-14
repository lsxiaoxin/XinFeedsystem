# API 文档 — XinFeedSystem

Base URL: `http://localhost:8080/api/v1`

统一响应格式：
```json
{ "code": 0, "msg": "ok", "data": { ... } }
```
错误时 `code != 0`，`data` 为 `null`。

所有 ID 字段在 JSON 中均为**字符串类型**（防止 JS 精度丢失）。

需要鉴权的接口须在 Header 中携带：`Authorization: Bearer <token>`

---

## 错误码

| code | 含义 |
|---|---|
| 0 | 成功 |
| 40001 | 参数错误 |
| 40100 | 未登录 |
| 40101 | Token 无效 |
| 40102 | Token 过期 |
| 10002 | 用户名已存在 |
| 10003 | 用户名或密码错误 |
| 20001 | 视频不存在 |
| 30001 | 已点赞 |
| 30002 | 未点赞 |
| 50001 | 已关注 |
| 50002 | 未关注 |
| 60001 | 评论不存在 |
| 60002 | 无权限删除该评论 |
| 50000 | 服务器内部错误 |

---

## 用户模块

### POST /user/register 注册

**Request Body (JSON)**
```json
{
  "username": "alice",
  "password": "secret123",
  "nickname": "Alice"
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| username | string | ✓ | 3~32 字符 |
| password | string | ✓ | 6~32 字符 |
| nickname | string | ✓ | 最多 32 字符 |

**Response**
```json
{
  "code": 0, "msg": "ok",
  "data": {
    "token": "eyJ...",
    "user": {
      "id": "2054846077474967552",
      "username": "alice",
      "nickname": "Alice",
      "avatar": "",
      "signature": "",
      "follow_count": 0,
      "follower_count": 0
    }
  }
}
```

---

### POST /user/login 登录

**Request Body (JSON)**
```json
{ "username": "alice", "password": "secret123" }
```

**Response** 同注册。

---

### GET /user/:id 获取用户信息

**Path Param**: `id` — 用户 ID（字符串）

**Response**
```json
{
  "code": 0, "msg": "ok",
  "data": {
    "id": "2054846077474967552",
    "username": "alice",
    "nickname": "Alice",
    "avatar": "",
    "signature": "",
    "follow_count": 3,
    "follower_count": 10
  }
}
```

---

### GET /user/me 获取当前登录用户信息（需鉴权）

**Response** 同上。

---

## 视频模块

### POST /video/publish 发布视频（需鉴权）

**Request**: `multipart/form-data`

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| title | string | ✓ | 最多 128 字符 |
| duration | int | — | 视频时长（秒） |
| video | file | ✓ | 视频文件 |
| cover | file | — | 封面图片 |

**Response**
```json
{
  "code": 0, "msg": "ok",
  "data": {
    "id": "2054848219438911488",
    "author_id": "2054846077902786560",
    "title": "我的视频",
    "play_url": "/static/videos/2054848219438911488.mp4",
    "cover_url": "/static/covers/2054848219438911488.jpg",
    "duration": 60,
    "like_count": 0,
    "comment_count": 0,
    "play_count": 0,
    "heat": 0,
    "created_at": 1778748967406
  }
}
```

---

### GET /video/:id 获取视频详情

**Response** 同发布响应的 data。

---

### GET /video/list 作者视频列表（游标分页）

**Query Params**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| author_id | int64 | ✓ | 作者 ID |
| cursor_time | int64 | — | 上次返回的 next_cursor_time，首页不传 |
| cursor_id | int64 | — | 上次返回的 next_cursor_id，首页不传 |
| limit | int | — | 默认 10，最大 50 |

**Response**
```json
{
  "code": 0, "msg": "ok",
  "data": {
    "videos": [ { ...VideoVO... } ],
    "has_more": true,
    "next_cursor_time": 1778748967000,
    "next_cursor_id": "2054848219438911488"
  }
}
```

---

## Feed 流模块

### GET /feed Feed 流（支持三种策略）

**Query Params**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| type | string | ✓ | `latest` \| `following` \| `popularity` |
| cursor | string | — | 上次返回的 next_cursor，首页不传 |
| limit | int | — | 默认 10，最大 50 |

`following` 类型需要在 Header 中携带有效 JWT。

**策略说明**

| type | 排序规则 | 游标 score 含义 |
|---|---|---|
| latest | 全站发布时间倒序 | created_at（毫秒时间戳） |
| following | 关注的人发布的视频，时间倒序 | created_at（毫秒时间戳） |
| popularity | 全站热度倒序（热度=点赞次数+评论次数） | heat 值 |

**Response**
```json
{
  "code": 0, "msg": "ok",
  "data": {
    "videos": [
      {
        "id": "2054848219438911488",
        "author_id": "2054846077902786560",
        "title": "B的测试视频",
        "play_url": "/static/videos/2054848219438911488.mp4",
        "cover_url": "",
        "duration": 60,
        "like_count": 1,
        "comment_count": 1,
        "play_count": 0,
        "heat": 3,
        "created_at": 1778748967406
      }
    ],
    "next_cursor": "eyJzIjozLCJpIjoyMDU0ODQ4MjE5NDM4OTExNDg4fQ==",
    "has_more": false
  }
}
```

---

## 点赞模块

### POST /like/action 点赞/取消点赞（需鉴权）

**Request Body (JSON)**
```json
{
  "video_id": "2054848219438911488",
  "action_type": 1
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| video_id | string | ✓ | 视频 ID |
| action_type | int | ✓ | 1=点赞 2=取消 |

**Response** `data: null`

---

### GET /like/list 我点赞的视频列表（需鉴权，游标分页）

**Query Params**

| 参数 | 类型 | 说明 |
|---|---|---|
| cursor_time | int64 | 游标时间，首页不传 |
| cursor_id | int64 | 游标 ID，首页不传 |
| limit | int | 默认 10 |

**Response**
```json
{
  "code": 0, "msg": "ok",
  "data": {
    "videos": [ { ...VideoVO... } ],
    "has_more": false,
    "next_cursor_time": 0,
    "next_cursor_id": "0"
  }
}
```

---

## 评论模块

### POST /comment/action 发评论/删评论（需鉴权）

**发评论** `action_type: 1`
```json
{
  "action_type": 1,
  "video_id": "2054848219438911488",
  "content": "好看！"
}
```

**删评论** `action_type: 2`
```json
{
  "action_type": 2,
  "comment_id": "2054848557587894272"
}
```

**发评论 Response**
```json
{
  "code": 0, "msg": "ok",
  "data": {
    "id": "2054848557587894272",
    "video_id": "2054848219438911488",
    "user": { ...UserVO... },
    "content": "好看！",
    "like_count": 0,
    "created_at": 1778749048027
  }
}
```

删评论 `data: null`。

---

### GET /comment/list 评论列表（游标分页）

**Query Params**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| video_id | int64 | ✓ | 视频 ID |
| cursor_time | int64 | — | 首页不传 |
| cursor_id | int64 | — | 首页不传 |
| limit | int | — | 默认 10 |

**Response**
```json
{
  "code": 0, "msg": "ok",
  "data": {
    "comments": [ { ...CommentVO... } ],
    "has_more": false,
    "next_cursor_time": 0,
    "next_cursor_id": "0"
  }
}
```

---

## 关注模块

### POST /follow/action 关注/取关（需鉴权）

**Request Body (JSON)**
```json
{
  "followee_id": "2054846077902786560",
  "action_type": 1
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| followee_id | string | ✓ | 被关注用户 ID |
| action_type | int | ✓ | 1=关注 2=取关 |

不能关注自己，返回 `40001`。

**Response** `data: null`

---

### GET /follow/following 关注列表（游标分页）

**Query Params**

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| user_id | int64 | ✓ | 查询谁的关注列表 |
| cursor_time | int64 | — | 首页不传 |
| cursor_id | int64 | — | 首页不传 |
| limit | int | — | 默认 10 |

**Response**
```json
{
  "code": 0, "msg": "ok",
  "data": {
    "users": [ { ...UserVO... } ],
    "has_more": false,
    "next_cursor_time": 0,
    "next_cursor_id": 0
  }
}
```

---

### GET /follow/follower 粉丝列表（游标分页）

参数与关注列表相同，`user_id` 为查询谁的粉丝。
