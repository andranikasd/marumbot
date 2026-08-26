// Package queries holds every SQL statement the application runs.
//
// No SQL string is written in Go source. Statements live in .sql files, are
// embedded at build time, and are looked up by name — the same "-- name:"
// convention sqlc uses, so moving to generated code later is mechanical rather
// than a rewrite.
package queries

import (
	"embed"
	"fmt"
	"strings"
	"sync"
)

//go:embed *.sql
var files embed.FS

var (
	once    sync.Once
	byName  map[string]string
	loadErr error
)

// Get returns the named statement, or panics if it does not exist. A missing
// query is a programming error caught by the first test that runs it, not a
// runtime condition worth threading an error through every call site.
func Get(name string) string {
	q, err := Lookup(name)
	if err != nil {
		panic(err)
	}
	return q
}

// Lookup returns the named statement.
func Lookup(name string) (string, error) {
	once.Do(load)
	if loadErr != nil {
		return "", loadErr
	}
	q, ok := byName[name]
	if !ok {
		return "", fmt.Errorf("no query named %q", name)
	}
	return q, nil
}

// Names lists every embedded statement, for tests that assert coverage.
func Names() []string {
	once.Do(load)
	out := make([]string, 0, len(byName))
	for n := range byName {
		out = append(out, n)
	}
	return out
}

func load() {
	byName = map[string]string{}
	entries, err := files.ReadDir(".")
	if err != nil {
		loadErr = err
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		body, err := files.ReadFile(e.Name())
		if err != nil {
			loadErr = err
			return
		}
		var name string
		var sb strings.Builder
		flush := func() {
			if name != "" && strings.TrimSpace(sb.String()) != "" {
				byName[name] = strings.TrimSpace(sb.String())
			}
			sb.Reset()
		}
		for _, line := range strings.Split(string(body), "\n") {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "-- name:"); ok {
				flush()
				name = strings.TrimSpace(rest)
				continue
			}
			if name != "" {
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
		flush()
	}
}
