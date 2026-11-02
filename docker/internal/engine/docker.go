// Package engine talks to the Docker Engine API (Docker Desktop, Colima,
// OrbStack, a plain daemon) over its unix socket or TLS for the Docker panel:
// containers grouped by Compose project, their actions, logs, a shell, the
// event stream, and the project's compose files. Only Compose's up / down /
// pull need the docker CLI.
package engine

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Container is one row of the panel.
type Container struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	State   string `json:"state"`  // running, exited, paused, restarting, created, dead
	Status  string `json:"status"` // "Up 3 hours (healthy)"
	Ports   []Port `json:"ports"`
	Created string `json:"created"`
	// Compose labels, empty for containers not started by Compose.
	Project     string `json:"project"`
	Service     string `json:"service"`
	ConfigFiles string `json:"configFiles"` // comma-separated, as Compose records them
	WorkingDir  string `json:"workingDir"`
}

// Port is a published port.
type Port struct {
	Host      string `json:"host"`
	HostPort  int    `json:"hostPort"`
	Container string `json:"container"` // "80/tcp"
}

// ComposeProject is a Compose application: running (from labels) or only a
// file in the workspace.
type ComposeProject struct {
	Name  string   `json:"name"`
	Dir   string   `json:"dir"`
	Files []string `json:"files"`
}

var idRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// validID accepts container ids and names; never anything that could alter a
// request path or read as a flag.
func validID(s string) bool { return len(s) <= 255 && idRe.MatchString(s) }

// composeNames are the file names `docker compose` looks for.
var composeNames = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}

// FindComposeFiles lists compose files in each root and its direct subfolders.
func FindComposeFiles(roots []string) []ComposeProject {
	var out []ComposeProject
	seen := map[string]bool{}
	add := func(dir string) {
		for _, n := range composeNames {
			p := filepath.Join(dir, n)
			if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && !seen[dir] {
				seen[dir] = true
				out = append(out, ComposeProject{Dir: dir, Files: []string{p}})
			}
		}
	}
	for _, root := range roots {
		add(root)
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && e.Name() != "node_modules" && e.Name() != "vendor" {
				add(filepath.Join(root, e.Name()))
			}
		}
	}
	return out
}
