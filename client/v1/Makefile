# 测试相关的 Makefile

.PHONY: test test-unit test-integration test-coverage test-benchmark clean

# 默认目标
all: test

# 运行所有测试
test:
	go test -v ./...

# 运行单元测试（跳过集成测试）
test-unit:
	go test -v -short ./...

# 运行集成测试（需要运行的服务）
test-integration:
	ENABLE_INTEGRATION_TESTS=true go test -v -run TestIntegration ./...

# 运行测试并生成覆盖率报告
test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

# 运行性能测试
test-benchmark:
	go test -v -bench=. -benchmem ./...

# 运行特定测试
test-specific:
	@echo "使用方法: make test-specific TEST=TestName"
	@echo "例如: make test-specific TEST=TestGetLatestTick"
	go test -v -run $(TEST) ./...

# 清理测试生成的文件
clean:
	rm -f coverage.out coverage.html
	go clean -testcache

# 安装测试依赖
deps:
	go mod tidy
	go get github.com/stretchr/testify/assert
	go get github.com/stretchr/testify/mock

# 检查代码质量
lint:
	golangci-lint run ./...

# 格式化代码
fmt:
	go fmt ./...

# 运行测试并检查覆盖率阈值
test-coverage-threshold:
	@echo "运行测试并检查覆盖率..."
	go test -v -coverprofile=coverage.out ./...
	@coverage=$$(go tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//'); \
	if [ $$(echo "$$coverage < 80" | bc -l) -eq 1 ]; then \
		echo "覆盖率 $$coverage% 低于 80% 的阈值"; \
		exit 1; \
	else \
		echo "覆盖率 $$coverage% 满足要求"; \
	fi

# 帮助信息
help:
	@echo "可用的测试命令:"
	@echo "  test                    - 运行所有测试"
	@echo "  test-unit              - 运行单元测试（跳过集成测试）"
	@echo "  test-integration       - 运行集成测试"
	@echo "  test-coverage          - 运行测试并生成覆盖率报告"
	@echo "  test-benchmark         - 运行性能测试"
	@echo "  test-specific TEST=xxx - 运行特定测试"
	@echo "  test-coverage-threshold- 检查覆盖率是否达到阈值"
	@echo "  clean                  - 清理测试文件"
	@echo "  deps                   - 安装测试依赖"
	@echo "  lint                   - 代码质量检查"
	@echo "  fmt                    - 格式化代码"