package main

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/flint-ide/extensions/docker/internal/engine"
)

// shellScript runs bash when the container has it, else sh (as Flint's
// built-in Docker shell did).
const shellScript = "command -v bash >/dev/null && exec bash || exec sh"

// refName is what a container id or name and a project name may look like
// before they reach a command line.
var refName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,254}$`)

// Verbs of the container actions: the toast's past tense, the row's
// description while it runs.
var containerVerbs = map[string][2]string{
	"start":   {"Started", "Starting…"},
	"stop":    {"Stopped", "Stopping…"},
	"restart": {"Restarted", "Restarting…"},
	"rm":      {"Removed", "Removing…"},
}

var composeVerbs = map[string][2]string{
	"up":      {"Compose up finished for", "Starting…"},
	"down":    {"Compose down finished for", "Stopping…"},
	"restart": {"Restarted", "Restarting…"},
	"stop":    {"Stopped", "Stopping…"},
	"pull":    {"Pulled the images of", "Pulling…"},
}

// action answers view/action. Anything that may take longer than Flint's
// 30-second request timeout (a confirmation, compose up) runs in the
// background; its result is a toast and a refreshed panel.
func (e *ext) action(item, act string) error {
	if item == "" { // the message's button
		if act == "refresh" {
			e.invalidate()
			e.refreshView()
		}
		return nil
	}
	if id, ok := strings.CutPrefix(item, prefixContainer); ok {
		return e.containerAction(item, id, act)
	}
	if strings.HasPrefix(item, prefixProject) {
		p, n, ok := e.findProject(item)
		if !ok {
			e.refreshView()
			return &rpcError{Code: -32602, Message: "the compose project is gone"}
		}
		return e.composeAction(item, p, n, act)
	}
	return nil // the warning and hint rows have no actions
}

// findContainer looks a container up in the last listing.
func (e *ext) findContainer(id string) (engine.Container, bool) {
	for _, c := range e.overview().containers {
		if c.ID == id {
			return c, true
		}
	}
	return engine.Container{}, false
}

// findProject looks the project of an item up, with its number of containers.
func (e *ext) findProject(item string) (engine.ComposeProject, int, bool) {
	s := e.overview()
	byDir, _ := tree(s)
	for _, p := range s.projects {
		if cut(prefixProject+p.Dir, 256) == item {
			return p, len(byDir[p.Dir]), true
		}
	}
	return engine.ComposeProject{}, 0, false
}

func (e *ext) containerAction(item, id, act string) error {
	if !refName.MatchString(id) {
		return &rpcError{Code: -32602, Message: "invalid container id"}
	}
	c, ok := e.findContainer(id)
	if !ok {
		c = engine.Container{ID: id, Name: shortID(id)}
	}
	name := cmpName(c)
	switch {
	case act == "logs":
		return e.openTerminal("Logs "+name, []string{"docker", "logs", "-f", "--tail", strconv.Itoa(e.tail()), id})
	case act == "shell":
		return e.openTerminal("Shell "+name, []string{"docker", "exec", "-it", id, "sh", "-c", shellScript})
	case strings.HasPrefix(act, "port:"):
		port, err := strconv.Atoi(strings.TrimPrefix(act, "port:"))
		if err != nil || port <= 0 || port > 65535 {
			return &rpcError{Code: -32602, Message: "invalid port"}
		}
		return e.fl.call("window/openUrl", map[string]string{"url": fmt.Sprintf("http://localhost:%d", port)}, nil)
	}
	verbs, ok := containerVerbs[act]
	if !ok {
		return &rpcError{Code: -32602, Message: "unknown action " + act}
	}
	e.background(func() {
		if act == "rm" && !e.confirm(fmt.Sprintf("Remove container %s?", name), "Remove") {
			return
		}
		e.run(item, verbs[1], func() error {
			dk, _, err := e.docker()
			if err != nil {
				return err
			}
			return dk.Act(e.ctx, id, act)
		}, fmt.Sprintf("%s %s.", verbs[0], name), fmt.Sprintf("Couldn't %s %s", verbName(act), name))
	})
	return nil
}

func (e *ext) composeAction(item string, p engine.ComposeProject, containers int, act string) error {
	name := projectName(p)
	if act == "logs" {
		if p.Name == "" || !refName.MatchString(p.Name) {
			return &rpcError{Code: -32602, Message: "the project has no containers"}
		}
		return e.openTerminal("Logs "+name, []string{"docker", "compose", "-p", p.Name, "logs", "-f", "--tail", strconv.Itoa(e.tail())})
	}
	verbs, ok := composeVerbs[act]
	if !ok {
		return &rpcError{Code: -32602, Message: "unknown action " + act}
	}
	e.background(func() {
		if act == "down" && !e.confirm(fmt.Sprintf("Stop and remove the containers of %s?", name), "Down") {
			return
		}
		e.run(item, verbs[1], func() error {
			dk, _, err := e.docker()
			if err != nil {
				return err
			}
			return dk.Compose(e.ctx, p, act)
		}, fmt.Sprintf("%s %s.", verbs[0], name), fmt.Sprintf("Compose %s failed for %s", act, name))
	})
	return nil
}

func cmpName(c engine.Container) string {
	if c.Name != "" {
		return c.Name
	}
	return shortID(c.ID)
}

func verbName(act string) string {
	if act == "rm" {
		return "remove"
	}
	return act
}

// background runs fn after the request is answered.
func (e *ext) background(fn func()) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		fn()
	}()
}

// run marks the item busy, runs fn, and reports the result as a toast.
func (e *ext) run(item, busy string, fn func() error, done, failed string) {
	e.mu.Lock()
	if e.busy[item] != "" {
		e.mu.Unlock()
		e.toast("warning", "Wait for the running action to finish.")
		return
	}
	e.busy[item] = busy
	e.mu.Unlock()
	e.refreshView()

	err := fn()

	e.mu.Lock()
	delete(e.busy, item)
	e.snap = nil
	e.mu.Unlock()
	e.refreshView()
	if err != nil {
		log.Printf("%s: %v", failed, err)
		e.toast("error", failed+": "+err.Error())
		return
	}
	e.toast("info", done)
}

// confirm asks the user with a native question; false when they cancel.
func (e *ext) confirm(question, yes string) bool {
	var r struct {
		Action string `json:"action"`
	}
	err := e.fl.call("window/showMessageRequest", map[string]any{"type": "warning", "message": question, "actions": []string{yes}}, &r)
	if err != nil {
		log.Printf("confirmation: %v", err)
		return false
	}
	return r.Action == yes
}

func (e *ext) toast(kind, msg string) {
	e.fl.notify("window/showMessage", map[string]string{"type": kind, "message": cut(msg, 1000)})
}

func (e *ext) tail() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.settings.LogTailLines
}

// openTerminal opens a terminal tab typing command, with the environment
// the docker CLI needs to reach the configured engine.
func (e *ext) openTerminal(title string, command []string) error {
	e.mu.Lock()
	host := e.settings.Host
	e.mu.Unlock()
	params := map[string]any{"title": cut(title, 100), "command": command, "tag": "Docker"}
	if host != "" {
		if err := (settings{Host: host}).check(); err != nil {
			return err
		}
		ep, err := e.deps.resolve(host)
		if err != nil {
			return err
		}
		if env := ep.CLIEnv(); len(env) > 0 {
			params["env"] = env
		}
	}
	err := e.fl.call("window/openTerminal", params, nil)
	var re *rpcError
	if errors.As(err, &re) && re.Code == -32601 {
		return &rpcError{Code: -32603, Message: "this Flint can't open terminal tabs for extensions yet; update Flint"}
	}
	return err
}
