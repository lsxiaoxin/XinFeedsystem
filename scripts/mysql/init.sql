-- XinFeedsystem 阶段 1 schema (MySQL 8.0)
-- 字符集 utf8mb4 / 字节序 utf8mb4_0900_ai_ci
-- 所有 ID 使用应用层雪花算法生成，DB 仅声明类型

SET NAMES utf8mb4;
SET time_zone = '+08:00';

CREATE DATABASE IF NOT EXISTS xinfeedsystem
  DEFAULT CHARACTER SET utf8mb4
  DEFAULT COLLATE utf8mb4_0900_ai_ci;

USE xinfeedsystem;

-- ---------------------------------------------------------------
-- 1. users 用户表
-- ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `users` (
  `id`             BIGINT UNSIGNED NOT NULL COMMENT '雪花 ID',
  `username`       VARCHAR(32)     NOT NULL COMMENT '登录名',
  `password_hash`  VARCHAR(72)     NOT NULL COMMENT 'bcrypt 60 字节',
  `nickname`       VARCHAR(32)     NOT NULL COMMENT '展示名',
  `avatar`         VARCHAR(255)    DEFAULT NULL COMMENT '头像 URL',
  `signature`      VARCHAR(140)    DEFAULT NULL COMMENT '个性签名',
  `follow_count`   INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT '关注数（冗余）',
  `follower_count` INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT '粉丝数（冗余）',
  `token`          VARCHAR(512)    NOT NULL DEFAULT '' COMMENT '当前登录 token，单会话',
  `created_at`     DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`     DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`     DATETIME(3)     DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='用户表';

-- ---------------------------------------------------------------
-- 2. videos 视频表
-- ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `videos` (
  `id`            BIGINT UNSIGNED NOT NULL COMMENT '雪花 ID',
  `author_id`     BIGINT UNSIGNED NOT NULL COMMENT '作者',
  `title`         VARCHAR(128)    NOT NULL COMMENT '标题',
  `play_url`      VARCHAR(512)    NOT NULL COMMENT '播放地址（本地相对路径）',
  `cover_url`     VARCHAR(512)    NOT NULL COMMENT '封面 URL',
  `duration`      INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT '时长（秒）',
  `like_count`    INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT '点赞数（冗余）',
  `comment_count` INT UNSIGNED    NOT NULL DEFAULT 0 COMMENT '评论数（冗余）',
  `play_count`    BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '播放数（冗余）',
  `heat`          BIGINT          NOT NULL DEFAULT 0 COMMENT '热度分（点赞+评论累计）',
  `status`        TINYINT         NOT NULL DEFAULT 1 COMMENT '0审核 1正常 2下架',
  `created_at`    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at`    DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at`    DATETIME(3)     DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_author_created` (`author_id`, `created_at` DESC),
  KEY `idx_created`        (`created_at` DESC, `id` DESC),
  KEY `idx_heat`           (`heat` DESC, `id` DESC),
  KEY `idx_like_count`     (`like_count` DESC, `id` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='视频表';

-- ---------------------------------------------------------------
-- 3. video_likes 视频点赞表（专表）
-- ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `video_likes` (
  `id`         BIGINT UNSIGNED NOT NULL,
  `user_id`    BIGINT UNSIGNED NOT NULL COMMENT '点赞用户',
  `video_id`   BIGINT UNSIGNED NOT NULL COMMENT '被点赞视频',
  `created_at` DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3)     DEFAULT NULL COMMENT '软删除（取消点赞）',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_video` (`user_id`, `video_id`),
  KEY `idx_video` (`video_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='视频点赞表';

-- ---------------------------------------------------------------
-- 4. comments 评论表（二级展平）
-- ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `comments` (
  `id`                BIGINT UNSIGNED NOT NULL,
  `video_id`          BIGINT UNSIGNED NOT NULL,
  `user_id`           BIGINT UNSIGNED NOT NULL COMMENT '评论者',
  `parent_id`         BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0=一级评论',
  `root_id`           BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '所属一级评论 ID',
  `reply_to_user_id`  BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '@谁',
  `content`           VARCHAR(1000)   NOT NULL,
  `like_count`        INT UNSIGNED    NOT NULL DEFAULT 0,
  `created_at`        DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `deleted_at`        DATETIME(3)     DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_video_root_created` (`video_id`, `root_id`, `created_at`),
  KEY `idx_user_created`       (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='评论表';

-- ---------------------------------------------------------------
-- 5. follows 关注表
-- ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `follows` (
  `id`           BIGINT UNSIGNED NOT NULL,
  `follower_id`  BIGINT UNSIGNED NOT NULL COMMENT '发起关注的人',
  `followee_id`  BIGINT UNSIGNED NOT NULL COMMENT '被关注的人',
  `created_at`   DATETIME(3)     NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `deleted_at`   DATETIME(3)     DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_follower_followee` (`follower_id`, `followee_id`),
  KEY `idx_followee_follower` (`followee_id`, `follower_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='关注关系表';
