package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

// What the panel remembers between launches: the context shown, each
// context's namespace and the view (Flint's built-in panel keeps the same in
// localStorage, which a sandboxed frame doesn't have). It is a file in the
// extension's data folder.

const (
	stateFile      = "state.json"
	maxStateBytes  = 64 << 10
	maxNamespaces  = 200
	viewApps       = "apps"
	viewResources  = "resources"
	stateFileMode  = 0o600
	stateFolderMod = 0o700
)

type uiState struct {
	Context    string            `json:"context"`
	Namespaces map[string]string `json:"namespaces"`
	View       string            `json:"view"`
}

// clean drops what doesn't fit: unknown views, invalid names, too many entries.
func (s uiState) clean() uiState {
	out := uiState{Namespaces: map[string]string{}, View: viewApps}
	if validContext(s.Context) {
		out.Context = s.Context
	}
	if s.View == viewResources {
		out.View = viewResources
	}
	for c, ns := range s.Namespaces {
		if len(out.Namespaces) == maxNamespaces {
			break
		}
		if validContext(c) && kube.ValidName(ns) {
			out.Namespaces[c] = ns
		}
	}
	return out
}

func loadState(dir string) uiState {
	data, err := os.ReadFile(filepath.Join(dir, stateFile))
	var s uiState
	if err == nil && len(data) <= maxStateBytes {
		if err := json.Unmarshal(data, &s); err != nil {
			log.Printf("ignoring %s: %v", stateFile, err)
		}
	}
	return s.clean()
}

func saveState(dir string, raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > maxStateBytes {
		return errors.New("saveState needs a state of at most 64 KiB")
	}
	var s uiState
	if err := json.Unmarshal(raw, &s); err != nil {
		return &rpcError{Code: -32602, Message: "invalid state: " + err.Error()}
	}
	data, err := json.Marshal(s.clean())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, stateFolderMod); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(dir, stateFile), data, stateFileMode)
}
