package boardcache

import (
	"context"
	"testing"
)

func TestSQLitePersistsLimitedAndEmptyResources(t *testing.T) {
	path := t.TempDir() + "/boards.sqlite"
	repo, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceBoards(context.Background(), []any{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceMembers(context.Background(), "BK1", []any{}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Close(); err != nil {
		t.Fatal(err)
	}
	repo, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	v, loaded, _, err := repo.Boards(context.Background(), 1)
	if err != nil || !loaded || len(v.([]any)) != 1 {
		t.Fatalf("v=%v loaded=%v err=%v", v, loaded, err)
	}
	v, loaded, _, err = repo.Members(context.Background(), "BK1", 10)
	if err != nil || !loaded || len(v.([]any)) != 0 {
		t.Fatalf("v=%v loaded=%v err=%v", v, loaded, err)
	}
}

func TestSQLitePublishKeepsActiveGenerationOnFailure(t *testing.T) {
	repo, err := Open(t.TempDir() + "/boards.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	if err := repo.Publish(context.Background(), []any{map[string]any{"code": "OLD"}}, map[string]any{"OLD": []any{"old"}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Publish(context.Background(), make(chan int), nil); err == nil {
		t.Fatal("应拒绝无法编码的数据")
	}
	v, loaded, _, err := repo.Boards(context.Background(), 10)
	if err != nil || !loaded || v.([]any)[0].(map[string]any)["code"] != "OLD" {
		t.Fatalf("v=%v loaded=%v err=%v", v, loaded, err)
	}
}
