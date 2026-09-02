package session

import "time"

const (
	RunIdle        = "idle"
	RunBusy        = "busy"
	RunRetrying    = "retrying"
	RunInterrupted = "interrupted"
	RunCompleted   = "completed"
	RunFailed      = "failed"
)

func (s *Store) SetRunState(id, status string, attempt int, runErr string) (*Session, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, err := s.loadLocked(id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	sess.RunStatus = status
	sess.RunAttempt = attempt
	sess.RunError = runErr
	if status == RunBusy {
		sess.RunStartedAt = now
		sess.RunFinishedAt = ""
		sess.RunError = ""
	}
	if status == RunCompleted || status == RunFailed || status == RunInterrupted || status == RunIdle {
		sess.RunFinishedAt = now
	}
	sess.UpdatedAt = now
	return sess, s.writeMetaLocked(sess)
}
