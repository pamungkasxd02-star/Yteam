package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/event"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/runtime"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
	"github.com/pamungkasxd02-star/Yteam/packages/schema/src"
)

type Server struct {
	Runtime *runtime.Runtime
	Events  *event.Journal
	Token   string
}

func New(app *runtime.Runtime, journal *event.Journal, token string) *Server {
	return &Server{Runtime: app, Events: journal, Token: token}
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			writeJSON(w, http.StatusOK, map[string]any{"healthy": true})
			return
		}
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="YTEAM"`)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		s.route(w, r)
	})
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/status":
		writeJSON(w, http.StatusOK, map[string]any{
			"project": s.Runtime.Root,
			"model":   s.Runtime.Config.Model,
			"session": s.Runtime.CurrentSession(),
		})
	case r.Method == http.MethodGet && r.URL.Path == "/api/session":
		items, err := s.Runtime.Store.List()
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, items)
	case r.Method == http.MethodGet && r.URL.Path == "/api/models":
		models, err := s.Runtime.Provider.Models(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, models)
	case r.Method == http.MethodGet && r.URL.Path == "/api/tools":
		writeJSON(w, http.StatusOK, s.Runtime.RunnerTools())
	case r.Method == http.MethodGet && r.URL.Path == "/api/agent":
		writeJSON(w, http.StatusOK, map[string]any{"current": s.Runtime.AgentName(), "agents": s.Runtime.Agents()})
	case r.Method == http.MethodPost && r.URL.Path == "/api/agent":
		var input struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if err := s.Runtime.SetAgent(input.Name); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"current": s.Runtime.AgentName()})
	case r.Method == http.MethodGet && r.URL.Path == "/api/model":
		writeJSON(w, http.StatusOK, map[string]string{"current": s.Runtime.ModelName()})
	case r.Method == http.MethodPost && r.URL.Path == "/api/model":
		var input struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if err := s.Runtime.SetModel(input.Model); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"current": s.Runtime.ModelName()})
	case r.Method == http.MethodGet && r.URL.Path == "/api/git":
		status, err := s.Runtime.GitStatus(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	case r.Method == http.MethodGet && r.URL.Path == "/api/git/diff":
		diff, err := s.Runtime.GitDiff(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(diff))
	case r.Method == http.MethodGet && r.URL.Path == "/api/git/log":
		count := 10
		if raw := r.URL.Query().Get("count"); raw != "" {
			if parsed, parseErr := strconv.Atoi(raw); parseErr == nil {
				count = parsed
			}
		}
		log, err := s.Runtime.GitLog(r.Context(), count)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(log))
	case r.Method == http.MethodGet && r.URL.Path == "/api/skills":
		skills, err := s.Runtime.Skills()
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skills)
	case r.Method == http.MethodGet && r.URL.Path == "/api/mcp":
		writeJSON(w, http.StatusOK, s.Runtime.MCP())
	case r.Method == http.MethodGet && r.URL.Path == "/api/lsp":
		writeJSON(w, http.StatusOK, s.Runtime.LSP())
	case r.Method == http.MethodGet && r.URL.Path == "/api/permission/request":
		writeJSON(w, http.StatusOK, s.Runtime.PendingPermissions())
	case r.Method == http.MethodGet && r.URL.Path == "/api/question":
		writeJSON(w, http.StatusOK, s.Runtime.PendingQuestions(""))
	case r.Method == http.MethodPost && r.URL.Path == "/api/session":
		next, err := s.Runtime.Store.New()
		if err != nil {
			writeError(w, err)
			return
		}
		s.Runtime.SwitchSession(next)
		writeJSON(w, http.StatusCreated, next)
	case r.Method == http.MethodGet && r.URL.Path == "/api/event":
		s.streamEvents(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/session/"):
		s.sessionRoute(w, r)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	}
}

func (s *Server) sessionRoute(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/session/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, nil)
		return
	}
	current, err := s.Runtime.Store.Load(parts[0])
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		} else {
			writeError(w, err)
		}
		return
	}
	if len(parts) == 1 && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, current)
		return
	}
	if len(parts) == 2 && parts[1] == "message" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, current.Messages)
		return
	}
	if len(parts) == 2 && parts[1] == "context" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"session_id": current.ID, "messages": current.Messages})
		return
	}
	if len(parts) == 2 && parts[1] == "compact" && r.Method == http.MethodPost {
		var input struct {
			Summary string `json:"summary"`
			Keep    int    `json:"keep"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		result, err := s.Runtime.CompactSession(input.Summary, input.Keep)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if len(parts) == 2 && parts[1] == "revert" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, current.Revert)
		return
	}
	if len(parts) == 3 && parts[1] == "revert" && parts[2] == "stage" && r.Method == http.MethodPost {
		var input struct {
			MessageID string `json:"message_id"`
			Diff      string `json:"diff"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		next, err := s.Runtime.StageRevert(input.MessageID, input.Diff)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, next.Revert)
		return
	}
	if len(parts) == 3 && parts[1] == "revert" && parts[2] == "clear" && r.Method == http.MethodPost {
		next, err := s.Runtime.ClearRevert()
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, next)
		return
	}
	if len(parts) == 3 && parts[1] == "revert" && parts[2] == "commit" && r.Method == http.MethodPost {
		next, err := s.Runtime.CommitRevert()
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, next)
		return
	}
	if len(parts) == 2 && parts[1] == "history" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, map[string]any{"session_id": current.ID, "messages": current.Messages})
		return
	}
	if len(parts) == 2 && parts[1] == "event" && r.Method == http.MethodGet {
		s.streamSessionEvents(w, r, current.ID)
		return
	}
	if len(parts) == 2 && parts[1] == "permission" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.Runtime.PendingPermissionsForSession(current.ID))
		return
	}
	if len(parts) == 2 && parts[1] == "question" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.Runtime.PendingQuestions(current.ID))
		return
	}
	if len(parts) == 4 && parts[1] == "question" && parts[3] == "reply" && r.Method == http.MethodPost {
		var input schema.QuestionReply
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if err := s.Runtime.ReplyQuestion(r.Context(), current.ID, parts[2], input.Answers); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if len(parts) == 4 && parts[1] == "question" && parts[3] == "reject" && r.Method == http.MethodPost {
		if err := s.Runtime.RejectQuestion(r.Context(), current.ID, parts[2]); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if len(parts) == 2 && parts[1] == "input" && r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, s.Runtime.PendingInputs(current.ID))
		return
	}
	if len(parts) == 2 && parts[1] == "input" && r.Method == http.MethodPost {
		var input struct {
			Content  string           `json:"content"`
			Delivery session.Delivery `json:"delivery"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Content) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content is required"})
			return
		}
		if input.Delivery == "" {
			input.Delivery = session.DeliveryQueue
		}
		item, err := s.Runtime.AdmitInput(current.ID, input.Content, input.Delivery)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, item)
		return
	}
	if len(parts) == 3 && parts[1] == "input" && parts[2] == "promote" && r.Method == http.MethodPost {
		items := s.Runtime.PromoteInputs(current.ID)
		writeJSON(w, http.StatusOK, items)
		return
	}
	if len(parts) == 2 && parts[1] == "interrupt" && r.Method == http.MethodPost {
		s.Runtime.InterruptSession(current.ID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "interrupted"})
		return
	}
	if len(parts) == 3 && parts[1] == "permission" && r.Method == http.MethodPost {
		var input struct {
			Reply permission.Reply `json:"reply"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		if err := s.Runtime.ReplyPermissionForSession(current.ID, parts[2], input.Reply); err != nil {
			if strings.Contains(err.Error(), "not found") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			} else {
				writeError(w, err)
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	if len(parts) == 2 && parts[1] == "rename" && r.Method == http.MethodPost {
		var input struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
		next, err := s.Runtime.Store.Rename(current.ID, input.Title)
		if err != nil {
			writeError(w, err)
			return
		}
		s.Runtime.SwitchSession(next)
		writeJSON(w, http.StatusOK, next)
		return
	}
	if len(parts) == 2 && parts[1] == "fork" && r.Method == http.MethodPost {
		next, err := s.Runtime.Store.Fork(current.ID)
		if err != nil {
			writeError(w, err)
			return
		}
		s.Runtime.SwitchSession(next)
		writeJSON(w, http.StatusCreated, next)
		return
	}
	if len(parts) == 2 && parts[1] == "export" && r.Method == http.MethodGet {
		if r.URL.Query().Get("format") == "json" {
			data, err := s.Runtime.Store.ExportJSON(current.ID)
			if err != nil {
				writeError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(data)
			return
		}
		data, err := s.Runtime.Store.ExportMarkdown(current.ID)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(data))
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := s.Runtime.Store.Delete(current.ID); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) == 2 && parts[1] == "prompt" && r.Method == http.MethodPost {
		var input struct {
			Content  string           `json:"content"`
			Delivery session.Delivery `json:"delivery"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Content) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content is required"})
			return
		}
		s.Runtime.SwitchSession(current)
		if input.Delivery == "" {
			input.Delivery = session.DeliveryQueue
		}
		if err := s.Runtime.PromptDelivery(r.Context(), input.Content, input.Delivery, w); err != nil {
			writeError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request) {
	if s.Events == nil {
		writeError(w, errors.New("event journal is not configured"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, errors.New("streaming is not supported"))
		return
	}
	fmt.Fprint(w, "event: server.connected\ndata: {\"healthy\":true}\n\n")
	flusher.Flush()
	updates := s.Events.Subscribe(r.Context())
	for {
		select {
		case <-r.Context().Done():
			return
		case item, ok := <-updates:
			if !ok {
				return
			}
			data, _ := json.Marshal(item)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", item.Type, data)
			flusher.Flush()
		}
	}
}

func (s *Server) streamSessionEvents(w http.ResponseWriter, r *http.Request, sessionID string) {
	if s.Events == nil {
		writeError(w, errors.New("event journal is not configured"))
		return
	}
	after, err := parseAfter(r.URL.Query().Get("after"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	history, err := s.Events.Since(sessionID, after)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, errors.New("streaming is not supported"))
		return
	}
	updates := s.Events.Subscribe(r.Context())
	writeEvent := func(item schema.Event) {
		data, _ := json.Marshal(item)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", item.Type, data)
		flusher.Flush()
	}
	for _, item := range history {
		writeEvent(item)
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case item, ok := <-updates:
			if !ok {
				return
			}
			if item.Aggregate == sessionID && item.Sequence > after {
				after = item.Sequence
				writeEvent(item)
			}
		}
	}
}

func parseAfter(value string) (uint64, error) {
	if value == "" {
		return 0, nil
	}
	number, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, errors.New("after must be a non-negative integer")
	}
	return number, nil
}

func (s *Server) authorized(r *http.Request) bool {
	if s.Token == "" {
		return true
	}
	if strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ") == s.Token {
		return true
	}
	if user, pass, ok := r.BasicAuth(); ok && user == "yteam" && pass == s.Token {
		return true
	}
	value := r.URL.Query().Get("auth_token")
	if value == "" {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	return err == nil && strings.HasSuffix(string(decoded), ":"+s.Token)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
}
