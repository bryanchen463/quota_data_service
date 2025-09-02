package client

import (
	"os"
	"strconv"
	"time"
)

// TestConfig 测试配置
type TestConfig struct {
	ServiceAddr string
	InitConn    int
	MaxConn     int
	IdleTimeout time.Duration
	MaxLifetime time.Duration
	EnableMock  bool
	MockDelay   time.Duration
}

// DefaultTestConfig 返回默认测试配置
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		ServiceAddr: "localhost:9090",
		InitConn:    1,
		MaxConn:     5,
		IdleTimeout: 30 * time.Second,
		MaxLifetime: 5 * time.Minute,
		EnableMock:  false,
		MockDelay:   0,
	}
}

// LoadTestConfigFromEnv 从环境变量加载测试配置
func LoadTestConfigFromEnv() *TestConfig {
	config := DefaultTestConfig()

	if addr := os.Getenv("TEST_GRPC_SERVICE_ADDR"); addr != "" {
		config.ServiceAddr = addr
	}

	if initConn := os.Getenv("TEST_INIT_CONN"); initConn != "" {
		if val, err := strconv.Atoi(initConn); err == nil {
			config.InitConn = val
		}
	}

	if maxConn := os.Getenv("TEST_MAX_CONN"); maxConn != "" {
		if val, err := strconv.Atoi(maxConn); err == nil {
			config.MaxConn = val
		}
	}

	if idleTimeout := os.Getenv("TEST_IDLE_TIMEOUT"); idleTimeout != "" {
		if val, err := time.ParseDuration(idleTimeout); err == nil {
			config.IdleTimeout = val
		}
	}

	if maxLifetime := os.Getenv("TEST_MAX_LIFETIME"); maxLifetime != "" {
		if val, err := time.ParseDuration(maxLifetime); err == nil {
			config.MaxLifetime = val
		}
	}

	if enableMock := os.Getenv("TEST_ENABLE_MOCK"); enableMock != "" {
		config.EnableMock = enableMock == "true"
	}

	if mockDelay := os.Getenv("TEST_MOCK_DELAY"); mockDelay != "" {
		if val, err := time.ParseDuration(mockDelay); err == nil {
			config.MockDelay = val
		}
	}

	return config
}

// IsIntegrationTestEnabled 检查是否启用集成测试
func IsIntegrationTestEnabled() bool {
	return os.Getenv("ENABLE_INTEGRATION_TESTS") == "true"
}

// IsShortTest 检查是否为短测试模式
func IsShortTest() bool {
	return os.Getenv("SHORT_TEST") == "true"
}
