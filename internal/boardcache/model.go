package boardcache

import "context"

type Board struct {
	Market uint16 `json:"market"`
	Code   string `json:"code"`
	Name   string `json:"name"`
}

type Member struct {
	Name   string `json:"name"`
	Market uint16 `json:"market"`
	Symbol string `json:"symbol"`
}

type Repository interface {
	Boards(context.Context, uint32) (data any, loaded bool, updated string, err error)
	Members(context.Context, string, uint32) (data any, loaded bool, updated string, err error)
	ReplaceBoards(context.Context, any) error
	ReplaceMembers(context.Context, string, any) error
	Publish(context.Context, any, map[string]any) error
	Close() error
}

type Fetcher interface {
	Boards(context.Context) (any, error)
	Members(context.Context, string) (any, error)
}
