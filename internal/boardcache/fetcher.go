package boardcache

import (
	"context"
	"encoding/json"
	"github.com/resse/tdx-api/internal/tdx"
)

type CallerFetcher struct {
	Caller  tdx.Caller
	Timeout func()
}

func (f CallerFetcher) call(ctx context.Context, op string, p tdx.Params) (any, error) {
	return f.Caller.Call(ctx, op, p)
}
func normalize(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	err = json.Unmarshal(raw, &out)
	return out, err
}
func (f CallerFetcher) Boards(ctx context.Context) (any, error) {
	v, e := f.call(ctx, "mac.boards", tdx.Params{Limit: 10000})
	if e != nil {
		return nil, e
	}
	return normalize(v)
}
func (f CallerFetcher) Members(ctx context.Context, board string) (any, error) {
	v, e := f.call(ctx, "mac.board.members", tdx.Params{BoardSymbol: board, Limit: 10000})
	if e != nil {
		return nil, e
	}
	return normalize(v)
}
