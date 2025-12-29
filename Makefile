# Lite SD-WAN Makefile

.PHONY: all build test clean install controller agent

# 版本信息
VERSION ?= 1.0.0
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go 编译参数
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# 输出目录
BUILD_DIR := build

all: build

# 编译所有二进制文件
build: controller agent

# 编译 Controller
controller:
	@echo "Building controller..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/sdwan-controller ./cmd/controller

# 编译 Agent
agent:
	@echo "Building agent..."
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/sdwan-agent ./cmd/agent

# 运行测试
test:
	@echo "Running tests..."
	go test -v -race -cover ./...

# 运行测试并生成覆盖率报告
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# 代码检查
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# 格式化代码
fmt:
	@echo "Formatting code..."
	go fmt ./...

# 清理构建产物
clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# 安装到系统
install: build
	@echo "Installing..."
	sudo cp $(BUILD_DIR)/sdwan-controller /usr/local/bin/
	sudo cp $(BUILD_DIR)/sdwan-agent /usr/local/bin/
	sudo mkdir -p /etc/sdwan
	@if [ ! -f /etc/sdwan/controller_config.yaml ]; then \
		sudo cp config/controller_config.yaml /etc/sdwan/; \
	fi
	@if [ ! -f /etc/sdwan/agent_config.yaml ]; then \
		sudo cp config/agent_config.yaml /etc/sdwan/; \
	fi
	@echo "Installation complete!"

# 卸载
uninstall:
	@echo "Uninstalling..."
	sudo rm -f /usr/local/bin/sdwan-controller
	sudo rm -f /usr/local/bin/sdwan-agent
	@echo "Uninstallation complete!"

# 交叉编译 Linux amd64
build-linux:
	@echo "Building for Linux amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/sdwan-controller-linux-amd64 ./cmd/controller
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/sdwan-agent-linux-amd64 ./cmd/agent

# 交叉编译 Linux arm64
build-linux-arm64:
	@echo "Building for Linux arm64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/sdwan-controller-linux-arm64 ./cmd/controller
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/sdwan-agent-linux-arm64 ./cmd/agent

# 编译所有平台
build-all: build build-linux build-linux-arm64

# 运行 Controller（开发模式）
run-controller:
	go run ./cmd/controller -config config/controller_config.yaml

# 运行 Agent（开发模式，需要 root）
run-agent:
	sudo go run ./cmd/agent -config config/agent_config.yaml

# 下载依赖
deps:
	go mod download
	go mod tidy

# 帮助
help:
	@echo "Lite SD-WAN Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build          - Build controller and agent"
	@echo "  make controller     - Build controller only"
	@echo "  make agent          - Build agent only"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make lint           - Run linter"
	@echo "  make fmt            - Format code"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make install        - Install to system"
	@echo "  make uninstall      - Uninstall from system"
	@echo "  make build-linux    - Cross-compile for Linux amd64"
	@echo "  make build-all      - Build for all platforms"
	@echo "  make run-controller - Run controller (dev mode)"
	@echo "  make run-agent      - Run agent (dev mode, needs root)"
	@echo "  make deps           - Download dependencies"
