package widget

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pkg/errors"
)

type bootstrapRequest struct {
	OrganizationID string `json:"organization_id"`
	WidgetID       string `json:"widget_id"`
	SessionToken   string `json:"session_token"`
}

// Handler exposes only the public bootstrap operation. Widget conversations
// use the issued session for subsequent authenticated APIs; this endpoint does
// not accept cookies or return Organization-private configuration.
func (s *Service) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		var request bootstrapRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		if err := decoder.Decode(&request); err != nil {
			writeBootstrapError(w, http.StatusBadRequest)
			return
		}
		origin := r.Header.Get("Origin")
		if strings.TrimSpace(origin) == "" {
			writeBootstrapError(w, http.StatusForbidden)
			return
		}
		response, err := s.BootstrapFromOrigin(r.Context(), request.OrganizationID, request.WidgetID, request.SessionToken, origin, r.RemoteAddr)
		if err != nil {
			if errors.Is(err, ErrRateLimited) {
				writeBootstrapError(w, http.StatusTooManyRequests)
				return
			}
			writeBootstrapError(w, http.StatusForbidden)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'self' "+origin)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})
}

func writeBootstrapError(w http.ResponseWriter, status int) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, `{"error":"widget bootstrap failed"}`, status)
}
