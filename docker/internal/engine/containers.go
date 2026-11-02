package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// apiContainer is an element of GET /containers/json.
type apiContainer struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
	Ports   []struct {
		IP          string `json:"IP"`
		PrivatePort int    `json:"PrivatePort"`
		PublicPort  int    `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
	Labels map[string]string `json:"Labels"`
}

// List returns every container, running or not.
func (d *Docker) List(ctx context.Context) ([]Container, error) {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	var raw []apiContainer
	if err := d.getJSON(ctx, "/containers/json", url.Values{"all": {"1"}}, &raw, true); err != nil {
		return nil, err
	}
	return convertContainers(raw), nil
}

func convertContainers(raw []apiContainer) []Container {
	cs := make([]Container, 0, len(raw))
	for _, r := range raw {
		name := ""
		if len(r.Names) > 0 {
			name = strings.TrimPrefix(r.Names[0], "/")
		}
		ports := []Port{}
		seen := map[int]bool{}
		for _, p := range r.Ports {
			if p.PublicPort == 0 || seen[p.PublicPort] { // unpublished, or the same port on IPv4 and IPv6
				continue
			}
			seen[p.PublicPort] = true
			ports = append(ports, Port{Host: p.IP, HostPort: p.PublicPort, Container: fmt.Sprintf("%d/%s", p.PrivatePort, p.Type)})
		}
		created := ""
		if r.Created > 0 {
			created = time.Unix(r.Created, 0).UTC().Format(time.RFC3339)
		}
		cs = append(cs, Container{
			ID: r.ID, Name: name, Image: r.Image, State: r.State, Status: r.Status, Ports: ports, Created: created,
			Project:     r.Labels["com.docker.compose.project"],
			Service:     r.Labels["com.docker.compose.service"],
			ConfigFiles: r.Labels["com.docker.compose.project.config_files"],
			WorkingDir:  r.Labels["com.docker.compose.project.working_dir"],
		})
	}
	return cs
}

// Details is what Flint needs to know about one container.
type Details struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Running bool   `json:"running"`
	// TTY: the container has a terminal, so its logs are not multiplexed.
	TTY bool `json:"tty"`
}

// Inspect reads one container (by id or name).
func (d *Docker) Inspect(ctx context.Context, id string) (Details, error) {
	if !validID(id) {
		return Details{}, fmt.Errorf("invalid container id %q", id)
	}
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	var raw struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		State  struct{ Running bool }
		Config struct {
			Tty   bool
			Image string
		}
	}
	if err := d.getJSON(ctx, "/containers/"+id+"/json", nil, &raw, true); err != nil {
		return Details{}, err
	}
	return Details{ID: raw.ID, Name: strings.TrimPrefix(raw.Name, "/"), Image: raw.Config.Image, Running: raw.State.Running, TTY: raw.Config.Tty}, nil
}

// containerActions are what Act accepts.
var containerActions = []string{"start", "stop", "restart", "rm"}

// Act runs start / stop / restart / rm on a container (by id or name).
func (d *Docker) Act(ctx context.Context, id, action string) error {
	if !slices.Contains(containerActions, action) {
		return fmt.Errorf("unknown container action %q", action)
	}
	if !validID(id) {
		return fmt.Errorf("invalid container id %q", id)
	}
	ctx, cancel := context.WithTimeout(ctx, actionTimeout)
	defer cancel()
	method, path := http.MethodPost, "/containers/"+id+"/"+action
	if action == "rm" {
		method, path = http.MethodDelete, "/containers/"+id
	}
	resp, err := d.request(ctx, method, path, nil, nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
