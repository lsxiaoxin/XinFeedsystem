.PHONY: run build swag up down tidy clean redis-cli kafka-cli kafka-topics kafka-like kafka-comment

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

## redis-cli: 进入 Redis 容器交互式命令行
redis-cli:
	docker exec -it xfs-redis redis-cli

## kafka-cli: 查看 like events（consumer，Ctrl+C 退出）
kafka-cli:
	docker exec -it xfs-kafka kafka-console-consumer.sh \
		--bootstrap-server localhost:9092 \
		--topic xfs.like.events --from-beginning

## kafka-topics: 列出所有 topic 及分区信息
kafka-topics:
	docker exec xfs-kafka kafka-topics.sh \
		--bootstrap-server localhost:9092 --list

## kafka-like: 手动向 like topic 投递一条消息（用于测试幂等）
## 使用: make kafka-like MSG='{"event_id":"test-1","video_id":1,"user_id":1,"delta":1,"ts":0}'
kafka-like:
	@echo '$(MSG)' | docker exec -i xfs-kafka kafka-console-producer.sh \
		--bootstrap-server localhost:9092 --topic xfs.like.events

## kafka-comment: 手动向 comment topic 投递一条消息
kafka-comment:
	@echo '$(MSG)' | docker exec -i xfs-kafka kafka-console-producer.sh \
		--bootstrap-server localhost:9092 --topic xfs.comment.events

## clean: 清理编译产物
clean:
	rm -rf bin/ docs/swagger.*

## help: 显示帮助
help:
	@grep -E '^##' Makefile | sed 's/## //'
