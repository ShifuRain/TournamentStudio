package server

import (
	"context"
	"net/http"
	"strings"

	"tournamentstudio/internal/auth"
)

type contextKey string

const sessionContextKey contextKey = "session"

func (s *Server) requireRole(roles ...auth.Role) func(http.Handler) http.Handler {
	allowed := make(map[auth.Role]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			sess, err := s.sessions.Find(token)
			if err != nil {
				http.Error(w, "invalid session", http.StatusUnauthorized)
				return
			}

			user, err := s.users.FindByID(sess.UserID)
			if err != nil {
				http.Error(w, "invalid session", http.StatusUnauthorized)
				return
			}
			if !allowed[user.Role] {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			// Reflect the current role (not the one snapshotted at login) to
			// any handler that reads the session from the request context.
			sess.Role = user.Role

			ctx := context.WithValue(r.Context(), sessionContextKey, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func sessionFromContext(r *http.Request) *auth.Session {
	sess, _ := r.Context().Value(sessionContextKey).(*auth.Session)
	return sess
}
