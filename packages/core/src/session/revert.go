package session

import (
	"errors"
	"time"
)

var ErrMessageNotFound = errors.New("session message not found")

func (s *Store) StageRevert(id, messageID, diff string) (*Session, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	sess, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if messageID == "" {
		return nil, ErrMessageNotFound
	}
	found := false
	for _, message := range sess.Messages {
		if message.ID == messageID {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrMessageNotFound
	}
	sess.Revert = &RevertState{MessageID: messageID, Diff: diff}
	sess.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writeMeta(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) ClearRevert(id string) (*Session, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	sess, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	sess.Revert = nil
	sess.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writeMeta(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *Store) CommitRevert(id string) (*Session, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	sess, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if sess.Revert == nil {
		return sess, nil
	}
	sess.Revert = nil
	sess.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writeMeta(sess); err != nil {
		return nil, err
	}
	return sess, nil
}
