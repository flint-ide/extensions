package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/flint-ide/extensions/kubernetes/internal/awsprof"
)

// EKS logins: a context whose kubeconfig user runs `aws eks get-token` with
// an AWS profile shows that profile's credential status, and offers what
// Flint's AWS panel offers when they expired — renewing them without a
// terminal when the profile allows it, else its login command (aws sso
// login, saml2aws login…) in a terminal tab.

// awsProfile is the named profile of ~/.aws, or nil when there is none.
func (e *ext) awsProfile(name string) (*awsprof.Profile, error) {
	if name == "" {
		return nil, nil
	}
	if !awsprof.ValidName(name) {
		return nil, fmt.Errorf("invalid AWS profile name %q", name)
	}
	profiles, err := e.profiles()
	if err != nil {
		return nil, err
	}
	for _, p := range profiles {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, nil
}

func (e *ext) mustProfile(name string) (awsprof.Profile, error) {
	p, err := e.awsProfile(name)
	if err != nil {
		return awsprof.Profile{}, err
	}
	if p == nil {
		return awsprof.Profile{}, fmt.Errorf("no AWS profile %q in ~/.aws", name)
	}
	return *p, nil
}

// profileEnv is the environment of a terminal working as a profile.
func profileEnv(p awsprof.Profile) map[string]string {
	env := map[string]string{"AWS_PROFILE": p.Name}
	if p.Region != "" {
		env["AWS_REGION"] = p.Region
	}
	return env
}

// awsLogin runs the profile's login command in a terminal tab, as Flint's
// AWS panel does.
func (e *ext) awsLogin(name string) error {
	p, err := e.mustProfile(name)
	if err != nil {
		return err
	}
	if len(p.Login) == 0 {
		return fmt.Errorf("%s has no login command", p.Name)
	}
	return e.openTerminal(map[string]any{"title": "aws login: " + p.Name, "command": p.Login, "env": profileEnv(p)})
}

// awsRefresh renews the profile's credentials without a terminal. It may wait
// minutes for the browser (SSO), longer than a request may take, so it runs
// on its own and tells the panel how it ended with webview/post
// { type: "aws", profile, ok, message | error, needsInput }.
func (e *ext) awsRefresh(name string) error {
	p, err := e.mustProfile(name)
	if err != nil {
		return err
	}
	if !p.Refreshable {
		return fmt.Errorf("%s has nothing to refresh", p.Name)
	}
	if _, running := e.refreshing.LoadOrStore(p.Name, true); running {
		return nil
	}
	go func() {
		defer e.refreshing.Delete(p.Name)
		out, err := e.aws.Refresh(context.Background(), p)
		msg := map[string]any{"type": "aws", "profile": p.Name, "ok": err == nil}
		if err != nil {
			log.Printf("refreshing AWS profile %s: %v", p.Name, err)
			msg["error"] = err.Error()
			msg["needsInput"] = strings.Contains(err.Error(), "needs input")
		} else {
			lines := strings.Split(strings.TrimSpace(out), "\n")
			last := lines[len(lines)-1]
			if last == "" {
				last = "Credentials refreshed"
			}
			msg["message"] = last
		}
		e.post(msg)
	}()
	return nil
}
