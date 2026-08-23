package server

import (
	"io/fs"
	"net/http"
)

// webUIHandler serves the embedded frontend build: a real file at the
// requested path if one exists, else index.html (so React Router's
// client-side routes -- e.g. /tournaments/123 -- survive a hard
// refresh, since only "/" and real asset paths exist as actual files
// in the embedded FS).
func (s *Server) webUIHandler() http.Handler {
	fileServer := http.FileServerFS(s.webFS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(s.webFS, r.URL.Path[1:]); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
