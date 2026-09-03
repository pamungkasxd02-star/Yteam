package identity

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Token represents an authenticated user or service session.
type Token struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email,omitempty"`
	Secret    string    `json:"secret"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Store manages secure local persistence for tokens and credentials.
type Store struct {
	mu       sync.RWMutex
	filePath string
	tokens   map[string]Token
}

func Open(homeDir string) (*Store, error) {
	if homeDir == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		homeDir = filepath.Join(h, ".config", "yteam")
	}
	_ = os.MkdirAll(homeDir, 0o700)
	p := filepath.Join(homeDir, "auth.json")

	s := &Store{filePath: p, tokens: make(map[string]Token)}
	data, err := os.ReadFile(p)
	if err == nil {
		_ = json.Unmarshal(data, &s.tokens)
	}
	return s, nil
}

func (s *Store) CreateToken(userID, email string, ttl time.Duration) (Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idBytes := make([]byte, 16)
	secBytes := make([]byte, 32)
	_, _ = rand.Read(idBytes)
	_, _ = rand.Read(secBytes)

	now := time.Now().UTC()
	tok := Token{
		ID:        "tok_" + hex.EncodeToString(idBytes),
		UserID:    userID,
		Email:     email,
		Secret:    "sec_" + hex.EncodeToString(secBytes),
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	s.tokens[tok.ID] = tok
	return tok, s.saveLocked()
}

func (s *Store) Validate(tokenID string) (Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tok, ok := s.tokens[tokenID]
	if !ok {
		return Token{}, errors.New("token not found")
	}
	if time.Now().UTC().After(tok.ExpiresAt) {
		return Token{}, errors.New("token expired")
	}
	return tok, nil
}

func (s *Store) Revoke(tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tokens, tokenID)
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.tokens, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath, data, 0o600)
}
