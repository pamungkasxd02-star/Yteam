package session

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

type Message struct {
	ID           string               `json:"id"`
	Role         string               `json:"role"`
	Content      string               `json:"content"`
	Reasoning    string               `json:"reasoning,omitempty"`
	Model        string               `json:"model,omitempty"`
	FinishReason string               `json:"finish_reason,omitempty"`
	Usage        *schema.Usage        `json:"usage,omitempty"`
	Parts        []schema.MessagePart `json:"parts,omitempty"`
	Name         string               `json:"name,omitempty"`
	ToolCallID   string               `json:"tool_call_id,omitempty"`
	ToolCalls    []schema.ToolCall    `json:"tool_calls,omitempty"`
	SnapshotID   string               `json:"snapshot_id,omitempty"`
	CreatedAt    string               `json:"created_at"`
}
type Session struct {
	ID            string       `json:"id"`
	Title         string       `json:"title"`
	Directory     string       `json:"directory"`
	CreatedAt     string       `json:"created_at"`
	UpdatedAt     string       `json:"updated_at"`
	RunStatus     string       `json:"run_status,omitempty"`
	RunAttempt    int          `json:"run_attempt,omitempty"`
	RunError      string       `json:"run_error,omitempty"`
	RunStartedAt  string       `json:"run_started_at,omitempty"`
	RunFinishedAt string       `json:"run_finished_at,omitempty"`
	Messages      []Message    `json:"-"`
	Revert        *RevertState `json:"revert,omitempty"`
}

type RevertState struct {
	MessageID string `json:"message_id"`
	Diff      string `json:"diff,omitempty"`
	Snapshot  string `json:"snapshot,omitempty"`
}
type Store struct {
	sessions, directory string
	mu                  sync.Mutex
	inputs              *InputQueue
}

func Open(home, directory string) (*Store, error) {
	dir := filepath.Join(home, "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	inputs, err := OpenInputQueue(home)
	if err != nil {
		return nil, err
	}
	return &Store{sessions: dir, directory: directory, inputs: inputs}, nil
}

func (s *Store) Inputs() *InputQueue { return s.inputs }

func (s *Store) New() (*Session, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sess := &Session{ID: "ses_" + hex.EncodeToString(buf), Title: "Sesi baru", Directory: s.directory, CreatedAt: now, UpdatedAt: now}
	return sess, s.writeMeta(sess)
}

func (s *Store) Load(id string) (*Session, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(s.sessions, id+".json"))
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	sess.Messages, err = s.readMessages(id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureMessageIDs(&sess); err != nil {
		return nil, err
	}
	return &sess, err
}

func (s *Store) Latest() (*Session, error) {
	list, err := s.List()
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return s.New()
	}
	return s.Load(list[0].ID)
}

func (s *Store) List() ([]Session, error) {
	entries, err := os.ReadDir(s.sessions)
	if err != nil {
		return nil, err
	}
	var result []Session
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.sessions, entry.Name()))
		if err != nil {
			return nil, err
		}
		var sess Session
		if err := json.Unmarshal(data, &sess); err != nil {
			return nil, err
		}
		result = append(result, sess)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].UpdatedAt > result[j].UpdatedAt })
	return result, nil
}

func (s *Store) Append(id string, message Message) error {
	if err := validID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if message.CreatedAt == "" {
		message.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if message.ID == "" {
		message.ID = newMessageID()
	}
	file, err := os.OpenFile(filepath.Join(s.sessions, id+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(file).Encode(message); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	sess, err := s.loadLocked(id)
	if err != nil {
		return err
	}
	sess.UpdatedAt = message.CreatedAt
	if sess.Title == "Sesi baru" && message.Role == "user" {
		sess.Title = title(message.Content)
	}
	return s.writeMetaLocked(sess)
}

func (s *Store) readMessages(id string) ([]Message, error) {
	file, err := os.Open(filepath.Join(s.sessions, id+".jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var result []Message
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var item Message
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, scanner.Err()
}

func (s *Store) writeMeta(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeMetaLocked(sess)
}
func (s *Store) writeMetaLocked(sess *Session) error {
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.sessions, sess.ID+".tmp")
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.sessions, sess.ID+".json"))
}
func validID(id string) error {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\\`) {
		return errors.New("invalid session id")
	}
	return nil
}
func title(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 80 {
		return value[:80] + "…"
	}
	return value
}

func newMessageID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "msg_unknown"
	}
	return "msg_" + hex.EncodeToString(buf)
}

func NewMessageID() string { return newMessageID() }

func (s *Store) ensureMessageIDs(sess *Session) error {
	changed := false
	for index := range sess.Messages {
		if sess.Messages[index].ID != "" {
			continue
		}
		sess.Messages[index].ID = newMessageID()
		changed = true
	}
	if !changed {
		return nil
	}
	return s.rewriteMessages(sess.ID, sess.Messages)
}

func (s *Store) rewriteMessages(id string, messages []Message) error {
	if err := validID(id); err != nil {
		return err
	}
	path := filepath.Join(s.sessions, id+".jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	for _, message := range messages {
		if err := json.NewEncoder(file).Encode(message); err != nil {
			return err
		}
	}
	return nil
}
