package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("HTTP_ADDR", ":8080")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.HTTPAddr != ":8080" || c.PoolSize != 2 || len(c.CORSOrigins) != 1 || c.CORSOrigins[0] != "*" {
		t.Fatalf("默认配置错误: %+v", c)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9000")
	t.Setenv("TDX_POOL_SIZE", "4")
	t.Setenv("TDX_TIMEOUT", "3s")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.example,https://b.example")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "true")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.PoolSize != 4 || c.UpstreamTimeout.String() != "3s" || len(c.CORSOrigins) != 2 {
		t.Fatalf("覆盖配置错误: %+v", c)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct{ key, value string }{{"TDX_POOL_SIZE", "0"}, {"TDX_TIMEOUT", "bad"}, {"GIN_MODE", "invalid"}}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			if _, err := Load(); err == nil {
				t.Fatal("应返回配置错误")
			}
		})
	}
}

func TestLoadRejectsWildcardCredentials(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "*")
	t.Setenv("CORS_ALLOW_CREDENTIALS", "true")
	if _, err := Load(); err == nil {
		t.Fatal("应拒绝通配符来源与凭据组合")
	}
}
