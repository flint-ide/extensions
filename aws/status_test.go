package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/flint-ide/extensions/aws/internal/awsprof"
)

func TestTimeLeft(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:                "0s",
		45 * time.Second:            "45s",
		12 * time.Minute:            "12m",
		2*time.Hour + 5*time.Minute: "2h 05m",
		47 * time.Hour:              "47h 00m",
		72 * time.Hour:              "3d",
	} {
		if got := timeLeft(d); got != want {
			t.Errorf("timeLeft(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestProfileStatus(t *testing.T) {
	now := testNow
	at := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	cases := []struct {
		p          awsprof.Profile
		text, tone string
	}{
		{awsprof.Profile{ExpiresAt: at(3 * time.Hour)}, "3h 00m", "ok"},
		{awsprof.Profile{ExpiresAt: at(10 * time.Minute)}, "10m", "warn"},
		{awsprof.Profile{ExpiresAt: at(-time.Second), Status: awsprof.StatusExpired}, "expired", "bad"},
		{awsprof.Profile{Status: awsprof.StatusLoggedOut}, "logged out", "bad"},
		{awsprof.Profile{Status: awsprof.StatusStatic}, "keys", "muted"},
		{awsprof.Profile{Status: awsprof.StatusUnknown, Source: "base"}, "—", "muted"},
	}
	for _, c := range cases {
		if got := profileStatus(c.p, now); got.text != c.text || got.tone != c.tone {
			t.Errorf("%+v: %+v, want %s/%s", c.p, got, c.text, c.tone)
		}
	}
	if got := profileStatus(awsprof.Profile{Source: "base"}, now).title; got != "Uses the credentials of base" {
		t.Errorf("title = %q", got)
	}
}

func TestProfileEnvAndShellCommand(t *testing.T) {
	if env := profileEnv(awsprof.Profile{Name: "dev"}); len(env) != 1 || env["AWS_PROFILE"] != "dev" {
		t.Errorf("env = %v", env)
	}
	if got := shellCommand([]string{"saml2aws", "login", "--role", "arn:aws:iam::1:role/Dev", "it's"}); got != `saml2aws login --role arn:aws:iam::1:role/Dev 'it'\''s'` {
		t.Errorf("shellCommand = %s", got)
	}
}

// TestManifest keeps the manifest in step with the code and with the rules
// Flint enforces (internal/extensions/manifest/check.go there).
func TestManifest(t *testing.T) {
	data, err := os.ReadFile("flint-extension.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Contract    int
		ID          string
		Run         struct{ Command string }
		Activation  []string
		Permissions []string
		Contributes struct {
			Commands []struct{ ID, Title string }
			Panels   []struct{ ID, Icon, View string }
			Settings struct {
				Properties map[string]struct {
					Type             string
					Default          int
					Minimum, Maximum int
				}
			}
		}
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Contract != 1 || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}\.[a-z0-9][a-z0-9-]{0,30}$`).MatchString(m.ID) {
		t.Errorf("contract %d, id %q", m.Contract, m.ID)
	}
	ignore, _ := os.ReadFile(".gitignore")
	if !strings.Contains(string(ignore), "/"+m.Run.Command) {
		t.Errorf("%s is not git-ignored", m.Run.Command)
	}
	if len(m.Contributes.Panels) != 1 || m.Contributes.Panels[0].ID != panelID || m.Contributes.Panels[0].View != "tree" || m.Contributes.Panels[0].Icon != "cloud" {
		t.Errorf("panels %+v", m.Contributes.Panels)
	}
	if len(m.Contributes.Commands) != 1 || m.Contributes.Commands[0].ID != refreshID {
		t.Errorf("commands %+v", m.Contributes.Commands)
	}
	for _, ev := range m.Activation {
		if ev != "onView:"+panelID && ev != "onCommand:"+refreshID {
			t.Errorf("activation %q", ev)
		}
	}
	if len(m.Permissions) != 1 || m.Permissions[0] != "terminal" {
		t.Errorf("permissions %v", m.Permissions)
	}
	s, ok := m.Contributes.Settings.Properties["refreshSeconds"]
	if !ok || s.Type != "integer" || s.Default != defaultRefreshSeconds || s.Minimum != minRefreshSeconds || s.Maximum != maxRefreshSeconds {
		t.Errorf("refreshSeconds = %+v", s)
	}
}
