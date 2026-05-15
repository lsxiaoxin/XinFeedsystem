package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"xinfeedsystem/config"
	"xinfeedsystem/internal/api"
	"xinfeedsystem/internal/consumer"
	"xinfeedsystem/internal/event"
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
	"xinfeedsystem/internal/router"
	"xinfeedsystem/internal/service"
	"xinfeedsystem/pkg/jwt"
	"xinfeedsystem/pkg/kafkaclient"
	"xinfeedsystem/pkg/logger"
	"xinfeedsystem/pkg/redisclient"
	"xinfeedsystem/pkg/snowflake"
)

func main() {
	cfg, err := config.Load("./config")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	if err := logger.Init(cfg.Log.Level, cfg.Log.Encoding); err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	if err := snowflake.Init(cfg.Snowflake.NodeID); err != nil {
		logger.Fatal("init snowflake", zap.Error(err))
	}

	jwt.Init(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.Expire)

	db, err := initDB(cfg)
	if err != nil {
		logger.Fatal("init db", zap.Error(err))
	}

	rdb, err := redisclient.New(cfg.Redis)
	if err != nil {
		logger.Fatal("init redis", zap.Error(err))
	}
	defer rdb.Close()
	logger.Info("redis connected", zap.String("addr", fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)))

	// Kafka writer（生产者）
	kafkaWriter := kafkaclient.NewWriter(cfg.Kafka)
	defer kafkaWriter.Close()

	// 验证 broker 连通性（非阻塞，失败只打 warn 不退出——Kafka 可能比服务慢启动）
	pingCtx, pingCancel := context.WithTimeout(context.Background(), cfg.Kafka.DialTimeout)
	if err := kafkaclient.Ping(pingCtx, cfg.Kafka.Brokers); err != nil {
		logger.Warn("kafka ping failed, continuing without Kafka", zap.Error(err))
	} else {
		logger.Info("kafka writer ready", zap.Strings("brokers", cfg.Kafka.Brokers))
	}
	pingCancel()

	producer := event.NewProducer(kafkaWriter, cfg.Kafka)

	// appCtx 用于所有后台 goroutine 的生命周期管理
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	// 依赖注入：repository → service → handler
	userRepo    := repository.NewUserRepository(db)
	videoRepo   := repository.NewVideoRepository(db)
	likeRepo    := repository.NewLikeRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	followRepo  := repository.NewFollowRepository(db)

	userSvc    := service.NewUserService(userRepo, rdb)
	videoSvc   := service.NewVideoService(videoRepo, cfg.Storage, rdb)
	likeSvc    := service.NewLikeService(likeRepo, rdb, producer)
	commentSvc := service.NewCommentService(commentRepo, userRepo, rdb, producer)
	followSvc  := service.NewFollowService(followRepo, userRepo, rdb)

	snapshotSvc := service.NewSnapshotService(videoRepo, rdb)
	snapshotSvc.Start(appCtx)
	logger.Info("snapshot service started")

	feedSvc := service.NewFeedService(
		service.NewLatestFetcher(videoRepo),
		service.NewFollowingFetcher(videoRepo, rdb),
		service.NewSnapshotFetcher("popularity", rdb, videoRepo),
		service.NewSnapshotFetcher("like_count", rdb, videoRepo),
	)

	// Kafka consumer（counter group）
	kafkaReader := kafkaclient.NewReader(cfg.Kafka, []string{cfg.Kafka.LikeTopic, cfg.Kafka.CommentTopic})
	counterConsumer := consumer.NewCounterConsumer(kafkaReader, videoRepo, rdb, cfg.Kafka)

	consumerDone := make(chan struct{})
	go func() {
		counterConsumer.Start(appCtx)
		close(consumerDone)
	}()

	tokenCache := repository.NewTokenCache(rdb)
	storageBase := cfg.Storage.BaseDir
	r := router.New(&router.Handlers{
		User:    api.NewUserHandler(userSvc),
		Video:   api.NewVideoHandler(videoSvc),
		Feed:    api.NewFeedHandler(feedSvc),
		Like:    api.NewLikeHandler(likeSvc),
		Comment: api.NewCommentHandler(commentSvc),
		Follow:  api.NewFollowHandler(followSvc),
	}, userRepo, tokenCache, storageBase)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		logger.Info("server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down: draining kafka consumer...")
	appCancel() // 通知后台 goroutine 停止

	// 等待 consumer 排空并 commit offset，最多 15s
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer drainCancel()
	select {
	case <-consumerDone:
		logger.Info("kafka consumer drained")
	case <-drainCtx.Done():
		logger.Warn("kafka consumer drain timeout")
	}

	logger.Info("shutting down http server...")
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}
	logger.Info("server exited")
}

func initDB(cfg *config.Config) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.MySQL.ConnMaxLifetime)

	// AutoMigrate 仅在 debug 模式下自动建表（生产环境用 init.sql）
	if cfg.Server.Mode == "debug" {
		if err := db.AutoMigrate(
			&entity.User{},
			&entity.Video{},
			&entity.VideoLike{},
			&entity.Comment{},
			&entity.Follow{},
		); err != nil {
			return nil, err
		}
	}

	return db, nil
}
