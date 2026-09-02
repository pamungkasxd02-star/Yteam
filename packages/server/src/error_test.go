package server

import (
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/pamungkasxd02-star/Yteam/packages/core/src/permission"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/question"
	"github.com/pamungkasxd02-star/Yteam/packages/core/src/session"
)

func TestErrorStatusMapsClientErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"missing", os.ErrNotExist, http.StatusNotFound},
		{"message", session.ErrMessageNotFound, http.StatusNotFound},
		{"question", question.ErrNotFound, http.StatusNotFound},
		{"denied", permission.ErrDenied, http.StatusForbidden},
		{"invalid", errors.New("invalid JSON"), http.StatusBadRequest},
		{"internal", errors.New("database unavailable"), http.StatusInternalServerError},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := errorStatus(item.err); got != item.want {
				t.Fatalf("status = %d, want %d", got, item.want)
			}
		})
	}
}
