package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) Rename(id, title string) (*Session, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) == "" {
		return nil, errors.New("session title is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, err := s.loadLocked(id)
	if err != nil {
		return nil, err
	}
	sess.Title = strings.TrimSpace(title)
	sess.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writeMetaLocked(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) Delete(id string) error {
	if err := validID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, suffix := range []string{".json", ".jsonl"} {
		if err := os.Remove(filepath.Join(s.sessions, id+suffix)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *Store) Fork(id string) (*Session, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	source, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	fork, err := s.New()
	if err != nil {
		return nil, err
	}
	fork.Title = "Fork: " + source.Title
	fork.Directory = source.Directory
	if err := s.writeMeta(fork); err != nil {
		return nil, err
	}
	for _, message := range source.Messages {
		if err := s.Append(fork.ID, message); err != nil {
			return nil, err
		}
	}
	return s.Load(fork.ID)
}

func (s *Store) ExportJSON(id string) ([]byte, error) {
	sess, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	type export struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		Directory string    `json:"directory"`
		CreatedAt string    `json:"created_at"`
		UpdatedAt string    `json:"updated_at"`
		Messages  []Message `json:"messages"`
	}
	return json.MarshalIndent(export{
		ID: sess.ID, Title: sess.Title, Directory: sess.Directory,
		CreatedAt: sess.CreatedAt, UpdatedAt: sess.UpdatedAt,
		Messages: sess.Messages,
	}, "", "  ")
}

func (s *Store) ExportMarkdown(id string) (string, error) {
	sess, err := s.Load(id)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\n", sess.Title)
	fmt.Fprintf(&out, "- Session: `%s`\n- Directory: `%s`\n\n", sess.ID, sess.Directory)
	for _, message := range sess.Messages {
		fmt.Fprintf(&out, "## %s\n\n%s\n\n", strings.Title(message.Role), message.Content)
	}
	return out.String(), nil
}

func (s *Store) Compact(id, summary string) error {
	if err := validID(id); err != nil {
		return err
	}
	if strings.TrimSpace(summary) == "" {
		return errors.New("summary is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.loadLocked(id); err != nil {
		return err
	}
	path := filepath.Join(s.sessions, id+".jsonl")
	message := Message{Role: "system", Content: "[context compacted]\n" + strings.TrimSpace(summary), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(message)
}

func (s *Store) loadLocked(id string) (*Session, error) {
	data, err := os.ReadFile(filepath.Join(s.sessions, id+".json"))
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	sess.Messages, err = s.readMessages(id)
	return &sess, err
}
