package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/flint-ide/extensions/aws/internal/awsprof"
)

// Labels and status of a profile, as Flint's built-in AWS panel showed them
// (frontend/src/lib/aws.ts).

// soon: credentials expiring sooner than this are a warning.
const soon = 15 * time.Minute

var kindLabels = map[string]string{
	awsprof.KindSSO:       "SSO",
	awsprof.KindTemporary: "session keys",
	awsprof.KindKeys:      "access keys",
	awsprof.KindRole:      "assumed role",
	awsprof.KindProcess:   "credential process",
	awsprof.KindConfig:    "settings only",
}

func kindLabel(kind string) string {
	if l, ok := kindLabels[kind]; ok {
		return l
	}
	return kind
}

// timeLeft writes a duration as "2h 05m", "12m", "45s" or "3d".
func timeLeft(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	if s < 0 {
		s = 0
	}
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	h := m / 60
	if h < 48 {
		return fmt.Sprintf("%dh %02dm", h, m%60)
	}
	return fmt.Sprintf("%dd", h/24)
}

// status is a profile's badge: short text, a tone and a longer explanation.
type status struct {
	text, tone, title string
}

// profileStatus is the badge of p now; expiry is recomputed on every read,
// so it counts down while the panel is shown.
func profileStatus(p awsprof.Profile, now time.Time) status {
	if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil {
		at := t.Local().Format("2006-01-02 15:04 MST")
		left := t.Sub(now)
		if left <= 0 {
			return status{"expired", "bad", "Expired " + at}
		}
		tone := "ok"
		if left < soon {
			tone = "warn"
		}
		return status{timeLeft(left), tone, "Valid until " + at}
	}
	switch p.Status {
	case awsprof.StatusLoggedOut:
		return status{"logged out", "bad", "No cached login: log in first"}
	case awsprof.StatusStatic:
		return status{"keys", "muted", "Long-term access keys (no expiry)"}
	}
	if p.Source != "" {
		return status{"—", "muted", "Uses the credentials of " + p.Source}
	}
	return status{"—", "muted", "Checked when used"}
}

// profileEnv is the environment of a terminal working as p.
func profileEnv(p awsprof.Profile) map[string]string {
	env := map[string]string{"AWS_PROFILE": p.Name}
	if p.Region != "" {
		env["AWS_REGION"] = p.Region
	}
	return env
}

// shellCommand writes argv as a shell line (for hints and tooltips only:
// Flint quotes the argv of a terminal itself).
func shellCommand(argv []string) string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if a != "" && strings.Trim(a, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:=@,+%") == "" {
			out[i] = a
			continue
		}
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return strings.Join(out, " ")
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, sep)
}
