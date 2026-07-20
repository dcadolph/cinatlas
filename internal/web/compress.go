package web

import (
	"compress/gzip"
	"net/http"
	"strings"
)

// staticMaxAge is how long clients may cache static assets. Embedded files
// carry no modtime, so without this header every page view refetches them.
const staticMaxAge = "public, max-age=86400"

// robotsTxt keeps crawlers out of the query-shaped endpoints, which are an
// unbounded URL space, and the globe fan-out, which is the most expensive
// page to serve. Movie and person pages stay crawlable.
const robotsTxt = `User-agent: *
Disallow: /search
Disallow: /find
Disallow: /sin
Disallow: /fit
Disallow: /globe
Disallow: /place
`

// handleRobots serves the crawl policy.
func (s *Server) handleRobots(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", staticMaxAge)
	_, _ = w.Write([]byte(robotsTxt))
}

// cacheStatic adds client caching to the static file server.
func cacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", staticMaxAge)
		next.ServeHTTP(w, r)
	})
}

// gzipWriter compresses the response body and drops the Content-Length the
// inner handler computed for the uncompressed form.
type gzipWriter struct {
	http.ResponseWriter
	// gz is the compressor wrapping the underlying writer.
	gz *gzip.Writer
}

// WriteHeader strips the stale Content-Length before the status goes out.
func (w *gzipWriter) WriteHeader(status int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
}

// Write sends the bytes through the compressor.
func (w *gzipWriter) Write(b []byte) (int, error) {
	return w.gz.Write(b)
}

// compress gzips responses for clients that accept it. Everything cinatlas
// serves itself is text, since posters and backdrops ride TMDB's CDN, so the
// whole surface compresses well. Range requests are flattened to full
// responses, which no text client cares about and keeps the encoding simple.
func compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		r.Header.Del("Range")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		gz := gzip.NewWriter(w)
		defer func() { _ = gz.Close() }()
		next.ServeHTTP(&gzipWriter{ResponseWriter: w, gz: gz}, r)
	})
}
