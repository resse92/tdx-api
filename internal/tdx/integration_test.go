package tdx

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/resse/tdx-api/internal/config"
)

func TestLiveMainAndMAC(t *testing.T) {
	if os.Getenv("TDX_INTEGRATION") != "1" {
		t.Skip("设置 TDX_INTEGRATION=1 后运行真实上游集成测试")
	}
	cases := []struct {
		op string
		p  Params
	}{{"stock.announcement", Params{}}, {"company.finance", Params{Market: 1, Code: "600519"}}, {"mac.symbol.info", Params{Market: 1, Code: "600519"}}}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			s, err := New(config.Config{PoolSize: 1, UpstreamTimeout: 8 * time.Second, RetryLimit: 1})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if !s.Ready() {
				t.Fatal("启动连接成功后服务应就绪")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if _, err := s.Call(ctx, tc.op, tc.p); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLiveStartupConnectionFailure(t *testing.T) {
	if os.Getenv("TDX_INTEGRATION") != "1" {
		t.Skip("设置 TDX_INTEGRATION=1 后运行真实上游集成测试")
	}
	s, err := New(config.Config{MainHosts: []string{"127.0.0.1:1"}, MACHosts: []string{"127.0.0.1:1"}, PoolSize: 1, UpstreamTimeout: time.Second})
	if err == nil || s != nil {
		t.Fatalf("不可达节点应导致启动失败: service=%v err=%v", s, err)
	}
}
