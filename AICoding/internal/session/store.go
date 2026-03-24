package session

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aicoding/internal/llm"
)

type PlanItem struct {
	Step   string `json:"step"`
	Status string `json:"status"`
}

type Session struct {
	ID        string        `json:"id"`
	Workspace string        `json:"workspace"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Messages  []llm.Message `json:"messages"`
	Plan      []PlanItem    `json:"plan,omitempty"`
}

type Store struct {
	dir string
}

type Summary struct {
	ID              string    `json:"id"`
	Workspace       string    `json:"workspace"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	LastUserMessage string    `json:"last_user_message,omitempty"`
	MessageCount    int       `json:"message_count"`
}

func New(workspace string) *Session {
	now := time.Now().UTC()
	return &Session{
		ID:        newID(),
		Workspace: workspace,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  make([]llm.Message, 0, 32),
		Plan:      make([]PlanItem, 0, 8),
	}
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Save(session *Session) error {
	session.UpdatedAt = time.Now().UTC()
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(session); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir, session.ID+".json"), bytes.TrimRight(buf.Bytes(), "\n"), 0o644)
}

func (s *Store) Load(id string) (*Session, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, id+".json"))
	if err != nil {
		return nil, err
	}
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	if session.Plan == nil {
		session.Plan = make([]PlanItem, 0, 8)
	}
	return &session, nil
}

func (s *Store) List(limit int) ([]Summary, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	summaries := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			return nil, err
		}

		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return nil, err
		}

		summaries = append(summaries, Summary{
			ID:              sess.ID,
			Workspace:       sess.Workspace,
			CreatedAt:       sess.CreatedAt,
			UpdatedAt:       sess.UpdatedAt,
			LastUserMessage: summarizeMessage(lastUserMessage(sess.Messages), 72),
			MessageCount:    len(sess.Messages),
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, nil
}

func newID() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102-150405")
	}
	return time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(buf)
}

func lastUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func summarizeMessage(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
}
