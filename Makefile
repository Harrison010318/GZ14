.PHONY: build run server bot proto clean help

# 默认目标
build: proto server bot

# 编译 protobuf
proto:
	protoc --go_out=. --go_opt=paths=source_relative proto/gz14.proto
	mv proto/gz14.pb.go internal/protocol/

# 编译服务器
server:
	go build -o bin/gz14-server ./cmd/server

# 编译压测工具
bot:
	go build -o bin/gz14-bot ./cmd/bot

# 运行服务器（需要先启动 MySQL 和 Redis）
run:
	go run ./cmd/server

# 启动 10 个机器人测试
test-bot:
	go run ./cmd/bot -bots 10 -mode move -duration-sec 10

# 压测：100 个机器人登录 + 移动
bench-bot:
	go run ./cmd/bot -bots 100 -rate 50 -mode move -duration-sec 30

# 压测：反复登录登出
bench-relogin:
	go run ./cmd/bot -bots 50 -rate 25 -mode relogin -duration-sec 30

# 压测：仅登录测试
bench-login:
	go run ./cmd/bot -bots 200 -rate 100 -mode login

# 清理
clean:
	rm -rf bin/

# 开发帮助
help:
	@echo "Usage:"
	@echo "  make proto       - 编译 protobuf 定义"
	@echo "  make build       - 编译所有二进制"
	@echo "  make run         - 启动服务端（需 MySQL+Redis）"
	@echo "  make test-bot    - 10 个机器人测试"
	@echo "  make bench-bot   - 100 个机器人压测"
	@echo "  make bench-login - 200 个机器人登录压测"
