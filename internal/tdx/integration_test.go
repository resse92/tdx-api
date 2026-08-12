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
	s := New(config.Config{PoolSize: 1, UpstreamTimeout: 8 * time.Second, RetryLimit: 1})
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cases := []struct {
		op string
		p  Params
	}{{"stock.count.sh", Params{Market: 1}}, {"stock.count.sz", Params{Market: 0}}, {"company.finance", Params{Market: 1, Code: "600519"}}, {"mac.symbol.info", Params{Market: 1, Code: "600519"}}}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			op := tc.op
			if op == "stock.count.sh" || op == "stock.count.sz" {
				op = "stock.count"
			}
			if _, err := s.Call(ctx, op, tc.p); err != nil {
				t.Fatal(err)
			}
		})
	}
}
