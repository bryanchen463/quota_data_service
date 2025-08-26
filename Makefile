.PHONY: proto build run clean test

# 默认目标
all: proto build

# 生成protobuf代码
proto:
	@echo "Generating protobuf code..."
	@mkdir -p proto
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/quota_service.proto

# 构建项目
build: proto
	@echo "Building project..."
	go mod tidy
	go build -o quota_data_service .

# 运行项目
run: build
	@echo "Running quota data service..."
	./quota_data_service

# 清理生成的文件
clean:
	@echo "Cleaning generated files..."
	rm -f quota_data_service
	rm -f proto/*.pb.go

# 测试
test:
	@echo "Running tests..."
	go test ./...

# 安装依赖
deps:
	@echo "Installing dependencies..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# 帮助信息
help:
	@echo "Available targets:"
	@echo "  proto   - Generate protobuf code"
	@echo "  build   - Build the project"
	@echo "  run     - Build and run the project"
	@echo "  clean   - Clean generated files"
	@echo "  test    - Run tests"
	@echo "  deps    - Install protobuf dependencies"
	@echo "  help    - Show this help message" 