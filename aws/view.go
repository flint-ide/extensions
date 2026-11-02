package main

import (
	"strings"
	"time"

	"github.com/flint-ide/extensions/aws/internal/awsprof"
)

// The panel: one node per profile, and under it the profile's details, the
// last identity check and the last credential refresh.

// Item ids: a prefix, then the profile's name (a detail row has its field
// first: "detail:region:<name>").
const (
	profilePrefix  = "profile:"
	identityPrefix = "identity:"
	refreshPrefix  = "refresh:"
	detailPrefix   = "detail:"
)

// Action ids.
const (
	actTerminal = "terminal"
	actLogin    = "login"
	actRefresh  = "refresh"
	actStop     = "stop"
	actIdentity = "identity"
	actDismiss  = "dismiss"
	actReload   = "reload"
)

// maxProfiles keeps an answer under Flint's 5,000 items.
const maxProfiles = 5000

type action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type item struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Tooltip     string   `json:"tooltip,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	HasChildren bool     `json:"hasChildren,omitempty"`
	Actions     []action `json:"actions,omitempty"`
}

type view struct {
	Kind    string   `json:"kind"`
	Items   []item   `json:"items,omitempty"`
	Text    string   `json:"text,omitempty"`
	Actions []action `json:"actions,omitempty"`
}

func messageView(text string, actions ...action) view {
	return view{Kind: "message", Text: text, Actions: actions}
}

// root is the list of profiles, or a message when there are none.
func (e *ext) root() view {
	e.mu.Lock()
	profiles, loadErr := e.profiles, e.loadErr
	identities, refreshes := e.identities, e.refreshes
	items := make([]item, 0, min(len(profiles), maxProfiles))
	now := e.d.now()
	for i, p := range profiles {
		if i == maxProfiles {
			break
		}
		items = append(items, profileItem(p, identities[p.Name], refreshes[p.Name], now))
	}
	e.mu.Unlock()

	reload := action{actReload, "Refresh"}
	switch {
	case loadErr != nil:
		return messageView("Couldn't read the AWS profiles: "+loadErr.Error(), reload)
	case len(items) == 0:
		text := "No profiles in ~/.aws/config or ~/.aws/credentials."
		if !e.d.hasCLI() {
			text += " The AWS CLI (aws) isn't installed either: install it, then run aws configure or aws configure sso."
		}
		return messageView(text, reload)
	}
	return view{Kind: "tree", Items: items}
}

func profileItem(p awsprof.Profile, id identityState, r refreshState, now time.Time) item {
	st := profileStatus(p, now)
	badge := st.text
	if r.running {
		badge = "refreshing…"
		if p.Kind == awsprof.KindSSO {
			badge = "browser…"
		}
	}
	tooltip := []string{p.Name, kindLabel(p.Kind)}
	if p.Account != "" {
		tooltip = append(tooltip, "account "+p.Account)
	}
	if p.Role != "" {
		tooltip = append(tooltip, "role "+p.Role)
	}
	if p.Region != "" {
		tooltip = append(tooltip, "region "+p.Region)
	}
	tooltip = append(tooltip, st.title)
	if id.id.Arn != "" && !id.checking && id.err == "" {
		tooltip = append(tooltip, "signed in as "+id.id.Arn)
	}
	return item{
		ID:          profilePrefix + p.Name,
		Label:       p.Name,
		Description: joinNonEmpty(" · ", badge, kindLabel(p.Kind), p.Account, p.Region),
		Tooltip:     strings.Join(tooltip, " · "),
		Icon:        "cloud",
		HasChildren: true,
		Actions:     profileActions(p, r),
	}
}

// profileActions is a profile's context menu, in the built-in panel's order.
func profileActions(p awsprof.Profile, r refreshState) []action {
	out := []action{{actTerminal, "Open Terminal"}}
	switch {
	case r.running:
		out = append(out, action{actStop, "Stop Refreshing"})
	case p.Refreshable:
		out = append(out, action{actRefresh, "Refresh Credentials"})
	}
	if len(p.Login) > 0 {
		out = append(out, action{actLogin, "Log In"})
	}
	return append(out, action{actIdentity, "Check Identity"})
}

// children are the rows under a profile.
func (e *ext) children(parent string) view {
	name, ok := strings.CutPrefix(parent, profilePrefix)
	if !ok {
		return view{Kind: "tree", Items: []item{}}
	}
	p, found := e.profile(name)
	if !found {
		return view{Kind: "tree", Items: []item{}}
	}
	e.mu.Lock()
	id, idOK := e.identities[name]
	r, rOK := e.refreshes[name]
	now := e.d.now()
	e.mu.Unlock()

	items := []item{}
	if rOK {
		items = append(items, refreshItem(p, r))
	}
	if idOK {
		items = append(items, identityItem(name, id))
	}
	st := profileStatus(p, now)
	detail := func(key, label, value string) {
		if value != "" {
			items = append(items, item{ID: detailPrefix + key + ":" + name, Label: label, Description: value, Tooltip: label + ": " + value})
		}
	}
	detail("kind", "Kind", kindLabel(p.Kind))
	detail("status", "Credentials", st.title)
	detail("account", "Account", p.Account)
	detail("role", "Role", p.Role)
	detail("source", "Source profile", p.Source)
	detail("region", "Region", p.Region)
	if len(p.Login) > 0 {
		detail("login", "Log in with", shellCommand(p.Login))
	}
	detail("env", "Terminal", "AWS_PROFILE="+p.Name)
	return view{Kind: "tree", Items: items}
}

func refreshItem(p awsprof.Profile, r refreshState) item {
	it := item{ID: refreshPrefix + p.Name}
	switch {
	case r.running && p.Kind == awsprof.KindSSO:
		it.Label, it.Icon = "Approve the login in your browser…", "updown"
		it.Actions = []action{{actStop, "Stop Refreshing"}}
	case r.running:
		it.Label, it.Icon = "Refreshing credentials…", "updown"
		it.Actions = []action{{actStop, "Stop Refreshing"}}
	case r.ok:
		it.Label, it.Icon = r.message, "checkbox"
	default:
		it.Label, it.Tooltip = r.message, r.message
		if r.needsInput && len(p.Login) > 0 {
			it.Actions = append(it.Actions, action{actLogin, "Log In"})
		}
		it.Actions = append(it.Actions, action{actDismiss, "Dismiss"})
	}
	return it
}

func identityItem(name string, id identityState) item {
	it := item{ID: identityPrefix + name, Actions: []action{{actIdentity, "Check Again"}}}
	switch {
	case id.checking:
		it.Label, it.Actions = "Checking…", nil
	case id.err != "":
		it.Label, it.Tooltip = id.err, id.err
	default:
		it.Label = id.id.Account + " · " + id.id.Arn
		it.Tooltip = "Signed in as " + id.id.Arn + " (user id " + id.id.UserID + ")"
	}
	return it
}

// profileName is the profile an item belongs to.
func profileName(itemID string) (string, bool) {
	for _, prefix := range []string{profilePrefix, identityPrefix, refreshPrefix} {
		if name, ok := strings.CutPrefix(itemID, prefix); ok {
			return name, true
		}
	}
	if rest, ok := strings.CutPrefix(itemID, detailPrefix); ok {
		if _, name, ok := strings.Cut(rest, ":"); ok {
			return name, true
		}
	}
	return "", false
}
