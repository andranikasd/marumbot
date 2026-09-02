package miniapp

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func shellTestServer() *Server {
	return &Server{Version: "1.2.3-dev.abc1234", Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func getPath(t *testing.T, server *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	server.Handler().ServeHTTP(w, r)
	return w
}

func TestShellStampsEveryAssetWithTheBuild(t *testing.T) {
	t.Parallel()
	w := getPath(t, shellTestServer(), "/")
	if w.Code != http.StatusOK {
		t.Fatalf("shell answered %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `src="a/1.2.3-dev.abc1234/js/main.js"`) {
		t.Fatalf("entry script is not stamped:\n%s", body)
	}
	if strings.Contains(body, `href="js/`) || strings.Contains(body, `href="styles.css"`) {
		t.Fatalf("an asset escaped the stamp:\n%s", body)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("shell Cache-Control = %q, want no-store", got)
	}
}

func TestVersionEndpointNamesTheBuildAndIsNeverCached(t *testing.T) {
	t.Parallel()
	w := getPath(t, shellTestServer(), "/version")
	if w.Code != http.StatusOK {
		t.Fatalf("version answered %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"version":"1.2.3-dev.abc1234"`) {
		t.Fatalf("version body = %s", w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("version Cache-Control = %q, want no-store", got)
	}
}

func TestStampedAssetsAreImmutable(t *testing.T) {
	t.Parallel()
	w := getPath(t, shellTestServer(), "/a/1.2.3-dev.abc1234/js/main.js")
	if w.Code != http.StatusOK {
		t.Fatalf("stamped asset answered %d", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("stamped asset Cache-Control = %q, want immutable", got)
	}
}

func TestStylesheetCarriesTheSharedTokens(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"/styles.css", "/a/1.2.3-dev.abc1234/styles.css"} {
		w := getPath(t, shellTestServer(), path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s answered %d", path, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "--brass:") || !strings.Contains(body, `[data-theme="dark"]`) {
			t.Fatalf("%s lacks the shared tokens:\n%.300s", path, body)
		}
		if !strings.Contains(body, ".appbar") {
			t.Fatalf("%s lacks the app stylesheet", path)
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/css") {
			t.Fatalf("%s Content-Type = %q", path, got)
		}
	}
}
