package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// composeTimeout covers `compose up`/`pull`, which may download images.
const composeTimeout = 10 * time.Minute

// ErrComposeNeedsCLI: up / down / pull / build are implemented by the docker
// CLI's Compose plugin, not by the Engine API.
var ErrComposeNeedsCLI = errors.New("Compose up, down and pull need the docker CLI with its Compose plugin (install Docker Desktop, or docker-ce-cli and docker-compose-plugin); listing, logs, shell, start, stop and restart work without it")

// composeActions map to `docker compose` arguments.
var composeActions = map[string][]string{
	"up":      {"up", "-d"},
	"down":    {"down"},
	"restart": {"restart"},
	"pull":    {"pull"},
	"stop":    {"stop"},
}

// needsCLI are the actions the Engine API can't do.
var needsCLI = map[string]bool{"up": true, "down": true, "pull": true}

// CLI finds the docker binary on PATH ("" when there is none).
func CLI() string {
	bin, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}
	return bin
}

// ComposeAvailable reports whether the docker CLI is installed, which Compose
// up / down / pull need.
func ComposeAvailable() bool { return CLI() != "" }

// Compose runs an action on a Compose project. up / down / pull run
// `docker compose` (the CLI is required); restart and stop use it when it
// exists and otherwise act on the project's containers through the API.
func (d *Docker) Compose(ctx context.Context, project ComposeProject, action string) error {
	verb, ok := composeActions[action]
	if !ok {
		return fmt.Errorf("unknown compose action %q", action)
	}
	if project.Name != "" && !validID(project.Name) {
		return fmt.Errorf("invalid project name %q", project.Name)
	}
	bin := CLI()
	if bin == "" {
		if needsCLI[action] {
			return ErrComposeNeedsCLI
		}
		return d.composeViaAPI(ctx, project, action)
	}
	if len(project.Files) == 0 || project.Dir == "" {
		return errors.New("the compose project has no files")
	}
	args := []string{"compose"}
	if project.Name != "" {
		args = append(args, "-p", project.Name)
	}
	for _, f := range project.Files {
		if !filepath.IsAbs(f) {
			return fmt.Errorf("compose file %q is not absolute", f)
		}
		args = append(args, "-f", f)
	}
	ctx, cancel := context.WithTimeout(ctx, composeTimeout)
	defer cancel()
	return d.runCLI(ctx, bin, project.Dir, append(args, verb...))
}

// runCLI executes docker; stderr becomes the error text.
func (d *Docker) runCLI(ctx context.Context, bin, dir string, args []string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	if env := d.ep.CLIEnv(); len(env) > 0 {
		cmd.Env = os.Environ()
		for _, k := range slices.Sorted(maps.Keys(env)) {
			cmd.Env = append(cmd.Env, k+"="+env[k])
		}
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if strings.Contains(msg, "Cannot connect to the Docker daemon") || strings.Contains(msg, "docker daemon is not running") {
			return ErrNotRunning
		}
		return fmt.Errorf("docker %s: %s", args[0], msg)
	}
	return nil
}

// composeViaAPI stops or restarts the running containers of a project.
func (d *Docker) composeViaAPI(ctx context.Context, project ComposeProject, action string) error {
	if project.Name == "" {
		return errors.New("the project isn't running")
	}
	cs, err := d.List(ctx)
	if err != nil {
		return err
	}
	var errs []error
	for _, c := range cs {
		if c.Project != project.Name || c.State != "running" {
			continue
		}
		if err := d.Act(ctx, c.ID, action); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.Name, err))
		}
	}
	return errors.Join(errs...)
}
