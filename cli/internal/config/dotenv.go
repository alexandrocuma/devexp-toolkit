package config

import (
	"bufio"
	"os"
	"strings"
)

// LoadDotenv reads mcps/.env into a map for MCP ${VAR} substitution. It is a
// deliberately minimal reader: KEY=VALUE, blank lines and # comments, with
// whitespace trimmed and everything after the first = kept as the value.
// Quotes are not stripped and \n is not unescaped, so a quoted value arrives
// with its quotes — the file holds tokens, not shell.
//
// A line without = is skipped rather than rejected, so one malformed entry
// cannot cost the user every other secret in the file.
func LoadDotenv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		env[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return env, scanner.Err()
}
