package miniapp

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/andranikasd/marumbot/internal/design"
)

// compressedAssets prepares public text assets once, outside the request path.
// API responses and private account data never enter this cache.
func compressedAssets(sub fs.FS, next http.Handler) http.Handler {
	encoded := make(map[string][]byte)
	_ = fs.WalkDir(sub, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || (!strings.HasSuffix(name, ".js") && !strings.HasSuffix(name, ".css")) {
			return nil
		}
		body, err := fs.ReadFile(sub, name)
		if err != nil {
			return err
		}
		if name == "styles.css" {
			body = append(append([]byte{}, design.Tokens...), body...)
		}
		var out bytes.Buffer
		zw := gzip.NewWriter(&out)
		_, _ = zw.Write(body)
		_ = zw.Close()
		encoded["/"+name] = out.Bytes()
		return nil
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := encoded[r.URL.Path]
		if ok {
			w.Header().Add("Vary", "Accept-Encoding")
		}
		if !ok || !acceptsGzip(r.Header.Get("Accept-Encoding")) || r.Header.Get("Range") != "" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		contentType := "text/javascript; charset=utf-8"
		if strings.HasSuffix(r.URL.Path, ".css") {
			contentType = "text/css; charset=utf-8"
		}
		w.Header().Set("Content-Type", contentType)
		http.ServeContent(w, r, r.URL.Path, time.Time{}, bytes.NewReader(body))
	})
}

func acceptsGzip(header string) bool {
	wildcard := false
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		coding := strings.ToLower(strings.TrimSpace(fields[0]))
		if coding != "gzip" && coding != "*" {
			continue
		}
		quality := 1000
		for _, parameter := range fields[1:] {
			key, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if ok && key == "q" {
				whole, fraction, _ := strings.Cut(value, ".")
				if len(fraction) > 3 || (whole != "0" && whole != "1") {
					quality = 0
					continue
				}
				parsed, err := strconv.Atoi(whole + fraction + strings.Repeat("0", 3-len(fraction)))
				if err != nil || parsed < 0 || parsed > 1000 {
					quality = 0
				} else {
					quality = parsed
				}
			}
		}
		if coding == "gzip" {
			return quality > 0
		}
		wildcard = quality > 0
	}
	return wildcard
}
