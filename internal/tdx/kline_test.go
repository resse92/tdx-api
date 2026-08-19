package tdx

import (
	"testing"
	"time"

	"github.com/bensema/gotdx/proto"
)

func TestPlanBars(t *testing.T) {
	today := time.Date(2026, 8, 19, 15, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	tests := []struct {
		name                 string
		start, end           uint64
		period               string
		wantOffset, wantSize uint32
	}{
		{"单日", 20260819, 20260819, "daily", 0, 15},
		{"跨周末", 20260814, 20260819, "daily", 0, 20},
		{"分钟线", 20260818, 20260819, "5m", 0, 768},
		{"历史区间", 20260801, 20260805, "weekly", 0, 19},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, size, err := PlanBars(tt.start, tt.end, tt.period, today)
			if err != nil || offset != tt.wantOffset || size != tt.wantSize {
				t.Fatalf("offset=%d size=%d err=%v", offset, size, err)
			}
		})
	}
}

func TestPlanBarsRejectsInvalidOrOverflowingRange(t *testing.T) {
	for _, tt := range [][2]uint64{{20260820, 20260819}, {20260230, 20260301}, {19900101, 21001231}} {
		if _, _, err := PlanBars(tt[0], tt[1], "1m", time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)); err == nil {
			t.Fatalf("范围 %v 应失败", tt)
		}
	}
}

func TestFilterBarsPreservesShapeOrderAndRange(t *testing.T) {
	data := &proto.GetIndexBarsReply{List: []proto.IndexBar{{Year: 2026, Month: 8, Day: 19}, {Year: 2026, Month: 8, Day: 17}, {Year: 2026, Month: 8, Day: 18}}}
	filtered, ok := FilterBars(data, 20260818, 20260819).(*proto.GetIndexBarsReply)
	if !ok || len(filtered.List) != 2 || filtered.List[0].Day != 19 || filtered.List[1].Day != 18 {
		t.Fatalf("过滤结果错误: %#v", filtered)
	}
}
