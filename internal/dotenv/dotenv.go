// Package dotenv parses and formats .env files.
package dotenv

import (
	"fmt"
	"regexp"
	"strings"
)

type Entry struct {
	Key   string
	Value string
}

var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)

// ValidKey reports whether key is a usable environment variable name.
func ValidKey(key string) bool { return keyRe.MatchString(key) }

// Parse reads KEY=VALUE lines. It supports comments, `export` prefixes,
// single quotes (literal), and double quotes (escapes, multi-line). A key
// may appear only once. Errors name line numbers and valid keys but never
// quote other input, which could be a secret value.
func Parse(src string) ([]Entry, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	var out []Entry
	seen := map[string]int{}
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			return nil, fmt.Errorf("line %d: missing '='", i+1)
		}
		key := strings.TrimSpace(line[:eq])
		if !ValidKey(key) {
			return nil, fmt.Errorf("line %d: invalid key", i+1)
		}
		if first, dup := seen[key]; dup {
			return nil, fmt.Errorf("line %d: duplicate key %s (first on line %d)", i+1, key, first)
		}
		seen[key] = i + 1
		raw := strings.TrimLeft(line[eq+1:], " \t")
		var value string
		switch {
		case strings.HasPrefix(raw, `"`):
			// Join following lines until the closing unescaped quote.
			body := raw[1:]
			for {
				if end := closingQuote(body); end >= 0 {
					value = unescape(body[:end])
					break
				}
				i++
				if i >= len(lines) {
					return nil, fmt.Errorf("unterminated double quote for %s", key)
				}
				body += "\n" + lines[i]
			}
		case strings.HasPrefix(raw, "'"):
			end := strings.IndexByte(raw[1:], '\'')
			if end < 0 {
				return nil, fmt.Errorf("line %d: unterminated single quote", i+1)
			}
			value = raw[1 : end+1]
		default:
			if idx := strings.Index(raw, " #"); idx >= 0 {
				raw = raw[:idx]
			}
			value = strings.TrimSpace(raw)
		}
		out = append(out, Entry{Key: key, Value: value})
	}
	return out, nil
}

func closingQuote(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// Format renders entries as a .env file that Parse reads back unchanged.
func Format(entries []Entry) string {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.Key)
		b.WriteByte('=')
		b.WriteString(quote(e.Value))
		b.WriteByte('\n')
	}
	return b.String()
}

func quote(v string) string {
	if v != "" && !strings.ContainsAny(v, " \t\n\r\"'\\#$`=") {
		return v
	}
	// Single quotes are literal in every dotenv loader, so prefer them;
	// they cannot hold a single quote or a line break.
	if !strings.ContainsAny(v, "'\n\r") {
		return "'" + v + "'"
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(v) + `"`
}
