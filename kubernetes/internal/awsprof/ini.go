package awsprof

import (
	"bufio"
	"bytes"
	"strings"
)

// section is one [header] of an AWS INI file with its keys (lower-cased).
type section struct {
	name   string
	values map[string]string
}

// parseINI reads the AWS config/credentials format: [section] headers,
// key = value lines, # and ; comments. Indented lines continue a nested block
// (s3 = …) and are skipped; later duplicates win.
func parseINI(data []byte) []section {
	var out []section
	var cur *section
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		raw := sc.Text()
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' {
			name, ok := strings.CutSuffix(line[1:], "]")
			if !ok {
				cur = nil
				continue
			}
			out = append(out, section{name: strings.TrimSpace(name), values: map[string]string{}})
			cur = &out[len(out)-1]
			continue
		}
		if cur == nil || raw[0] == ' ' || raw[0] == '\t' {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		cur.values[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
	}
	return out
}
