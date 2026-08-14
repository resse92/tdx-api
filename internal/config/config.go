package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	GinMode         string
	CORSOrigins     []string
	CORSMethods     []string
	CORSHeaders     []string
	CORSAllowCreds  bool
	MainHosts       []string
	MACHosts        []string
	PoolSize        int
	UpstreamTimeout time.Duration
	RetryLimit      int
	MaxItems        uint32
	ShutdownTimeout time.Duration
	SQLitePath      string
	RefreshTimeout  time.Duration
}

func Load() (Config, error) {
	c := Config{
		HTTPAddr:        env("HTTP_ADDR", ":8080"),
		GinMode:         env("GIN_MODE", "release"),
		CORSOrigins:     csvEnv("CORS_ALLOWED_ORIGINS", "*"),
		CORSMethods:     csvEnv("CORS_ALLOWED_METHODS", "GET,POST,OPTIONS"),
		CORSHeaders:     csvEnv("CORS_ALLOWED_HEADERS", "Origin,Content-Type,Accept,X-Request-ID"),
		MainHosts:       csvEnv("TDX_MAIN_HOSTS", ""),
		MACHosts:        csvEnv("TDX_MAC_HOSTS", ""),
		PoolSize:        2,
		UpstreamTimeout: 6 * time.Second,
		RetryLimit:      1,
		MaxItems:        1000,
		ShutdownTimeout: 15 * time.Second,
		SQLitePath:      "./data/boards.sqlite",
		RefreshTimeout:  2 * time.Minute,
	}
	var err error
	if c.CORSAllowCreds, err = boolEnv("CORS_ALLOW_CREDENTIALS", false); err != nil {
		return Config{}, err
	}
	if c.PoolSize, err = intEnv("TDX_POOL_SIZE", c.PoolSize); err != nil {
		return Config{}, err
	}
	if c.UpstreamTimeout, err = durationEnv("TDX_TIMEOUT", c.UpstreamTimeout); err != nil {
		return Config{}, err
	}
	if c.RetryLimit, err = intEnv("TDX_RETRY_LIMIT", c.RetryLimit); err != nil {
		return Config{}, err
	}
	if c.MaxItems, err = uint32Env("MAX_ITEMS", c.MaxItems); err != nil {
		return Config{}, err
	}
	if c.ShutdownTimeout, err = durationEnv("SHUTDOWN_TIMEOUT", c.ShutdownTimeout); err != nil {
		return Config{}, err
	}
	c.SQLitePath = env("SQLITE_PATH", c.SQLitePath)
	if c.RefreshTimeout, err = durationEnv("BOARD_REFRESH_TIMEOUT", c.RefreshTimeout); err != nil {
		return Config{}, err
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Validate() error {
	if c.HTTPAddr == "" {
		return fmt.Errorf("HTTP_ADDR 不能为空")
	}
	if strings.TrimSpace(c.SQLitePath) == "" {
		return fmt.Errorf("SQLITE_PATH 不能为空")
	}
	if c.GinMode != "debug" && c.GinMode != "release" && c.GinMode != "test" {
		return fmt.Errorf("GIN_MODE 必须是 debug、release 或 test")
	}
	if c.PoolSize < 1 || c.PoolSize > 32 {
		return fmt.Errorf("TDX_POOL_SIZE 必须在 1 到 32 之间")
	}
	if c.RetryLimit < 0 || c.RetryLimit > 5 {
		return fmt.Errorf("TDX_RETRY_LIMIT 必须在 0 到 5 之间")
	}
	if c.UpstreamTimeout <= 0 || c.ShutdownTimeout <= 0 || c.RefreshTimeout <= 0 {
		return fmt.Errorf("超时时间必须大于 0")
	}
	if c.MaxItems == 0 {
		return fmt.Errorf("请求边界必须大于 0")
	}
	if contains(c.CORSOrigins, "*") && c.CORSAllowCreds {
		return fmt.Errorf("CORS 通配符来源不能与凭据同时启用")
	}
	return nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return strings.TrimSpace(v)
	}
	return fallback
}
func csvEnv(key, fallback string) []string {
	v := env(key, fallback)
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
func contains(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}
func intEnv(key string, fallback int) (int, error) {
	v := env(key, "")
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s 格式错误: %w", key, err)
	}
	return n, nil
}
func uint32Env(key string, fallback uint32) (uint32, error) {
	v := env(key, "")
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%s 格式错误: %w", key, err)
	}
	return uint32(n), nil
}
func boolEnv(key string, fallback bool) (bool, error) {
	v := env(key, "")
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s 格式错误: %w", key, err)
	}
	return n, nil
}
func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	v := env(key, "")
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s 格式错误: %w", key, err)
	}
	return d, nil
}
