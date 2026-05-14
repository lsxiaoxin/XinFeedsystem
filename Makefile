.PHONY: run build swag up down tidy clean

APP := xinfeedsystem

## run: 启动服务（需先 make up）
run:
	go run ./cmd/server

## build: 编译二进制
build:
	go build -o bin/$(APP) ./cmd/server

## up: 用 docker-compose 启动依赖中间件
up:
	docker-compose -f deploy/docker-compose.yaml up -d

## down: 停止并移除容器
down:
	docker-compose -f deploy/docker-compose.yaml down

## swag: 生成 Swagger 文档
swag:
	swag init -g cmd/server/main.go -o docs

## tidy: 整理依赖
tidy:
	go mod tidy

## test: 运行测试
test:
	go test ./...

## lint: 静态检查
lint:
	golangci-lint run ./...

## clean: 清理编译产物
clean:
	rm -rf bin/ docs/swagger.*

## help: 显示帮助
help:
	@grep -E '^##' Makefile | sed 's/## //'
