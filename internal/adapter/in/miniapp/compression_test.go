package miniapp

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/andranikasd/marumbot/internal/design"
)

func TestCompressedAssets(t *testing.T) {
	sub := fstest.MapFS{"js/main.js": {Data: []byte("export const test = true;")}, "styles.css": {Data: []byte("body{}")}}
	h := compressedAssets(sub, withTokens(sub, http.FileServerFS(sub)))
	for _, path := range []string{"/js/main.js", "/styles.css"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		r.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Header().Get("Content-Encoding") != "gzip" || w.Header().Get("Vary") != "Accept-Encoding" {
			t.Fatal(w.Header())
		}
		zr, err := gzip.NewReader(w.Body)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(zr)
		if err != nil {
			t.Fatal(err)
		}
		_ = zr.Close()
		expected := string(sub[strings.TrimPrefix(path, "/")].Data)
		if path == "/styles.css" {
			expected = string(design.Tokens) + expected
		}
		if string(body) != expected {
			t.Fatal("compressed asset differs from original")
		}
		r = httptest.NewRequestWithContext(t.Context(), http.MethodHead, path, nil)
		r.Header.Set("Accept-Encoding", "gzip")
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Body.Len() != 0 || w.Header().Get("Content-Encoding") != "gzip" {
			t.Fatal("HEAD must retain encoding without body")
		}
	}
}

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{"": false, "br": false, "gzip": true, "br, gzip;q=0.5": true, "gzip;q=0": false, "*;q=1,gzip;q=0": false, "gzip;q=invalid": false, "*": true} {
		if got := acceptsGzip(header); got != want {
			t.Errorf("%q: got %v, want %v", header, got, want)
		}
	}
}
