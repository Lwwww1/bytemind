package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aicoding/internal/llm"
)

func TestStorePreservesUTF8Content(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	utf8Text := "\u4f60\u597d\uff0c\u4e16\u754c"
	sess := New(`E:\\workspace`)
	sess.Messages = append(sess.Messages, llm.Message{
		Role:    "user",
		Content: utf8Text,
	})

	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, sess.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), utf8Text) {
		t.Fatalf("expected raw json to contain utf-8 text %q, got %q", utf8Text, string(data))
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != 1 || loaded.Messages[0].Content != utf8Text {
		t.Fatalf("expected utf-8 content after load, got %#v", loaded.Messages)
	}
}

func TestStoreListReturnsRecentSessions(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	older := New(`E:\\repo-old`)
	older.ID = "older"
	older.CreatedAt = time.Date(2026, 3, 24, 8, 0, 0, 0, time.UTC)
	older.Messages = []llm.Message{{Role: "user", Content: "first question"}}
	if err := store.Save(older); err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)

	newer := New(`E:\\repo-new`)
	newer.ID = "newer"
	newer.CreatedAt = time.Date(2026, 3, 24, 9, 0, 0, 0, time.UTC)
	newer.Messages = []llm.Message{{Role: "assistant", Content: "thinking"}, {Role: "user", Content: "second question with more detail"}}
	if err := store.Save(newer); err != nil {
		t.Fatal(err)
	}

	summaries, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0].ID != "newer" {
		t.Fatalf("expected newest session first, got %#v", summaries)
	}
	if summaries[0].LastUserMessage != "second question with more detail" {
		t.Fatalf("unexpected preview: %#v", summaries[0])
	}
	if summaries[0].MessageCount != 2 {
		t.Fatalf("expected message count 2, got %#v", summaries[0])
	}

	limited, err := store.List(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 || limited[0].ID != "newer" {
		t.Fatalf("expected limited list to keep newest summary, got %#v", limited)
	}
}
