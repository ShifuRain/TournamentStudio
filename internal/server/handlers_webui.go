package server

import (
	"io/fs"
	"net/http"
	"strings"
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

// apiCatchAllPattern is the pattern under which apiNotFoundHandler is
// registered -- kept as a constant so apiNotFoundHandler can recognize
// (and ignore) its own registration when probing the mux below.
const apiCatchAllPattern = "/api/"

// apiMethods lists every HTTP method used by any /api/... route
// registered in routes(). It only needs to be broad enough to catch
// every method actually in use; probing with extra methods that are
// never registered is harmless.
var apiMethods = []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete}

// apiNotFoundHandler is registered for "/api/" so that unmatched
// /api/... paths 404 instead of falling through to the SPA catch-all
// (which would otherwise serve 200+HTML for both a typo'd path and a
// wrong-method request to a real endpoint). Go's ServeMux resolves the
// 404-vs-405 distinction for us automatically *unless* a method-agnostic
// pattern like this one already matches the path -- which it always
// does, since that's exactly what makes it useful as a catch-all -- so
// this handler re-derives that distinction itself: it probes the mux
// with the same path under every other method actually used by this
// server, and if a more specific (non-catch-all) pattern matches for
// some other method, responds 405 with an Allow header instead of 404.
func (s *Server) apiNotFoundHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var allowed []string
		for _, method := range apiMethods {
			if method == r.Method {
				continue
			}
			probe := r.Clone(r.Context())
			probe.Method = method
			if _, pattern := s.mux.Handler(probe); pattern != "" && pattern != apiCatchAllPattern {
				allowed = append(allowed, method)
			}
		}
		if len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		http.NotFound(w, r)
	})
}
