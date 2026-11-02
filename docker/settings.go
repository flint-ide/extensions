package main

import (
	"fmt"
	"strings"
)

// settings are the extension's settings (Settings ▸ Docker), stored by Flint
// under extensions.flint.docker; the schema is in flint-extension.json.
type settings struct {
	// Host overrides where the Docker Engine is: "unix:///path/docker.sock",
	// "tcp://host:2376" or "https://host:2376". Empty finds the engine like
	// the docker CLI: the current docker context, then the default sockets.
	Host string `json:"host"`
	// LogTailLines is how many past lines a container's log tab starts with.
	LogTailLines int `json:"logTailLines"`
}

// Bounds of the settings (the manifest's schema says the same).
const (
	minLogTailLines     = 1
	maxLogTailLines     = 10000
	defaultLogTailLines = 500
	maxHostLength       = 512
)

func defaultSettings() settings { return settings{LogTailLines: defaultLogTailLines} }

// normalize trims the host and clamps the tail (0 or missing is the default).
// Flint already replaces values that don't fit the schema; this is the same
// rule on this side, so a hand-edited settings.json can't surprise the code.
func (s settings) normalize() settings {
	s.Host = strings.TrimSpace(s.Host)
	if s.LogTailLines == 0 {
		s.LogTailLines = defaultLogTailLines
	}
	s.LogTailLines = max(minLogTailLines, min(maxLogTailLines, s.LogTailLines))
	return s
}

// check reports why the host isn't a valid engine address, as Flint's
// built-in Docker settings did.
func (s settings) check() error {
	h := s.Host
	if len(h) > maxHostLength || strings.ContainsAny(h, "\x00\n\r\t ") {
		return fmt.Errorf("the engine address must be a single address such as unix:///var/run/docker.sock or tcp://host:2376")
	}
	if h == "" || strings.HasPrefix(h, "/") {
		return nil
	}
	for _, p := range []string{"unix://", "tcp://", "http://", "https://"} {
		if strings.HasPrefix(h, p) {
			return nil
		}
	}
	return fmt.Errorf("engine address %q: use unix:///path, tcp://host:port or https://host:port", h)
}
