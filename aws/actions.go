package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/flint-ide/extensions/aws/internal/awsprof"
)

// action runs a panel action. Long work (a refresh may wait minutes for the
// browser) runs in the background so the answer comes at once; the panel
// follows with view/refresh.
func (e *ext) action(itemID, act string) error {
	if itemID == "" && act == actReload {
		e.reload()
		e.refreshView()
		return nil
	}
	name, ok := profileName(itemID)
	if !ok {
		return &rpcError{Code: -32602, Message: "unknown item " + itemID}
	}
	p, ok := e.profile(name)
	if !ok {
		return &rpcError{Code: -32602, Message: "no AWS profile " + name}
	}
	switch act {
	case actTerminal:
		return e.openProfileTerminal(p)
	case actLogin:
		e.dismiss(name)
		return e.login(p)
	case actRefresh:
		return e.refreshCredentials(p)
	case actStop:
		e.d.refresher.Cancel(name)
		return nil
	case actIdentity:
		e.checkIdentity(name)
		return nil
	case actDismiss:
		e.dismiss(name)
		e.refreshView()
		return nil
	}
	return &rpcError{Code: -32602, Message: "unknown action " + act}
}

// openProfileTerminal opens a terminal working as p (AWS_PROFILE, AWS_REGION).
func (e *ext) openProfileTerminal(p awsprof.Profile) error {
	if !awsprof.ValidName(p.Name) {
		return fmt.Errorf("invalid profile name %s", p.Name)
	}
	return e.c.call("window/openTerminal", map[string]any{
		"env": profileEnv(p),
		"tag": "aws:" + p.Name,
	}, nil)
}

// login runs the profile's login command (aws sso login, saml2aws…) in a
// terminal; the panel picks up the new credentials on its next read.
func (e *ext) login(p awsprof.Profile) error {
	if len(p.Login) == 0 {
		return fmt.Errorf("%s has no login command", p.Name)
	}
	if !awsprof.ValidName(p.Name) {
		return fmt.Errorf("invalid profile name %s", p.Name)
	}
	return e.c.call("window/openTerminal", map[string]any{
		"title":   "aws login: " + p.Name,
		"env":     profileEnv(p),
		"command": p.Login,
	}, nil)
}

// checkIdentity asks STS who the profile is; the answer shows under it.
func (e *ext) checkIdentity(name string) {
	e.mu.Lock()
	e.identities[name] = identityState{checking: true}
	e.mu.Unlock()
	e.refreshView()
	go func() {
		id, err := e.d.identity(e.ctx, name)
		st := identityState{id: id}
		if err != nil {
			st = identityState{err: err.Error()}
		}
		e.mu.Lock()
		e.identities[name] = st
		e.mu.Unlock()
		e.refreshView()
	}()
}

// refreshCredentials renews p's credentials without a terminal, then reads
// the profiles again. A login that needs an answer offers the terminal login.
func (e *ext) refreshCredentials(p awsprof.Profile) error {
	if !p.Refreshable {
		return fmt.Errorf("%s has nothing to refresh", p.Name)
	}
	e.mu.Lock()
	if e.refreshes[p.Name].running {
		e.mu.Unlock()
		return nil
	}
	e.refreshSeq++
	seq := e.refreshSeq
	e.refreshes[p.Name] = refreshState{running: true, seq: seq}
	e.mu.Unlock()
	e.refreshView()
	go e.runRefresh(p, seq)
	return nil
}

func (e *ext) runRefresh(p awsprof.Profile, seq int) {
	output, err := e.d.refresher.Refresh(e.ctx, p)
	st := refreshState{seq: seq}
	if err == nil {
		st.ok, st.message = true, lastLine(output)
		if st.message == "" {
			st.message = "Credentials refreshed"
		}
	} else {
		st.message = err.Error()
		st.needsInput = errors.Is(err, awsprof.ErrNeedsInput)
	}
	e.mu.Lock()
	e.refreshes[p.Name] = st
	e.mu.Unlock()
	e.reload()
	e.refreshView()

	switch {
	case err == nil:
		e.toast("info", "Refreshed "+p.Name)
		time.AfterFunc(doneNoteFor/e.d.scale, func() { e.clearDone(p.Name, seq) })
	case st.needsInput && len(p.Login) > 0:
		var answer struct{ Action string }
		q := map[string]any{"type": "warning", "message": p.Name + ": " + err.Error(), "actions": []string{"Log In"}}
		if e.c.call("window/showMessageRequest", q, &answer) == nil && answer.Action == "Log In" {
			e.dismiss(p.Name)
			if err := e.login(p); err != nil {
				log.Printf("login terminal for %s: %v", p.Name, err)
			}
			e.refreshView()
		}
	default:
		e.toast("error", "Refreshing "+p.Name+" failed: "+err.Error())
	}
}

// clearDone drops the note of a successful refresh, unless another started.
func (e *ext) clearDone(name string, seq int) {
	e.mu.Lock()
	r, ok := e.refreshes[name]
	drop := ok && r.ok && r.seq == seq
	if drop {
		delete(e.refreshes, name)
	}
	e.mu.Unlock()
	if drop {
		e.refreshView()
	}
}

// dismiss drops a finished refresh's note (a running one stays).
func (e *ext) dismiss(name string) {
	e.mu.Lock()
	if r, ok := e.refreshes[name]; ok && !r.running {
		delete(e.refreshes, name)
	}
	e.mu.Unlock()
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return ""
}
