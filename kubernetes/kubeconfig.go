package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

// terminalEnv is the environment of a terminal working in a context (and
// namespace) without touching the global current-context: KUBECONFIG starts
// with a small file that sets only current-context and that context's
// namespace (kubectl takes the first file to set a key), followed by the
// user's own kubeconfig. No credentials are copied. The small files live in
// the extension's data folder (FLINT_EXTENSION_DATA/kube).
func terminalEnv(ctx context.Context, k cluster, dataDir, name, namespace string) (map[string]string, error) {
	if namespace != "" && !kube.ValidName(namespace) {
		return nil, fmt.Errorf("invalid namespace %q", namespace)
	}
	contexts, err := k.Contexts(ctx)
	if err != nil {
		return nil, err
	}
	var found *kube.Context
	for i := range contexts {
		if contexts[i].Name == name {
			found = &contexts[i]
		}
	}
	if found == nil {
		return nil, fmt.Errorf("no context %q in the kubeconfig", name)
	}
	if namespace == "" {
		namespace = found.Namespace
	}
	dir := filepath.Join(dataDir, "kube")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(found.Name + "\x00" + namespace))
	path := filepath.Join(dir, hex.EncodeToString(sum[:8])+".yaml")
	if err := writeFileAtomic(path, overrideKubeconfig(*found, namespace), 0o600); err != nil {
		return nil, err
	}
	base := os.Getenv("KUBECONFIG")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		base = filepath.Join(home, ".kube", "config")
	}
	return map[string]string{"KUBECONFIG": path + string(os.PathListSeparator) + base}, nil
}

// overrideKubeconfig is YAML (JSON strings are valid YAML scalars, so names
// with any characters stay one value).
func overrideKubeconfig(c kube.Context, namespace string) []byte {
	q := func(s string) string {
		b, _ := json.Marshal(s)
		return string(b)
	}
	out := "apiVersion: v1\nkind: Config\ncurrent-context: " + q(c.Name) + "\ncontexts:\n" +
		"- name: " + q(c.Name) + "\n  context:\n    cluster: " + q(c.Cluster) + "\n    user: " + q(c.User) + "\n"
	if namespace != "" {
		out += "    namespace: " + q(namespace) + "\n"
	}
	return []byte(out)
}

// writeFileAtomic writes data to a temporary file next to path and renames
// it over path, so a reader never sees half a file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // a no-op after the rename
	if err := f.Chmod(perm); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// dataDir is the extension's own folder (FLINT_EXTENSION_DATA, which Flint
// creates); outside Flint, a folder in the user's cache directory.
func dataDir() (string, error) {
	if d := os.Getenv("FLINT_EXTENSION_DATA"); d != "" {
		return d, nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(cache, "flint.kubernetes")
	return d, os.MkdirAll(d, 0o700)
}
