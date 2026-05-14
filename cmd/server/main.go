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
	"xinfeedsystem/internal/model/entity"
	"xinfeedsystem/internal/repository"
	"xinfeedsystem/internal/router"
	"xinfeedsystem/internal/service"
	"xinfeedsystem/pkg/jwt"
	"xinfeedsystem/pkg/logger"
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

	// 依赖注入：repository → service → handler
	userRepo    := repository.NewUserRepository(db)
	videoRepo   := repository.NewVideoRepository(db)
	likeRepo    := repository.NewLikeRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	followRepo  := repository.NewFollowRepository(db)

	userSvc    := service.NewUserService(userRepo)
	videoSvc   := service.NewVideoService(videoRepo, cfg.Storage)
	likeSvc    := service.NewLikeService(likeRepo)
	commentSvc := service.NewCommentService(commentRepo, userRepo)
	followSvc  := service.NewFollowService(followRepo, userRepo)
	feedSvc    := service.NewFeedService(
		service.NewLatestFetcher(videoRepo),
		service.NewFollowingFetcher(videoRepo),
		service.NewPopularityFetcher(videoRepo),
	)

	storageBase := cfg.Storage.BaseDir
	r := router.New(&router.Handlers{
		User:    api.NewUserHandler(userSvc),
		Video:   api.NewVideoHandler(videoSvc),
		Feed:    api.NewFeedHandler(feedSvc),
		Like:    api.NewLikeHandler(likeSvc),
		Comment: api.NewCommentHandler(commentSvc),
		Follow:  api.NewFollowHandler(followSvc),
	}, storageBase)

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

	logger.Info("shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
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
