package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Context is one kubeconfig context, without credentials.
type Context struct {
	Name      string `json:"name"`
	Cluster   string `json:"cluster"`
	User      string `json:"user"`
	Namespace string `json:"namespace,omitempty"`
	Current   bool   `json:"current"`
	// Auth is how kubectl authenticates: "exec:<command>", "certificate",
	// "token", "auth-provider" or "".
	Auth string `json:"auth"`
	// AWSProfile is the AWS profile an EKS login (aws eks get-token) uses.
	AWSProfile string `json:"awsProfile,omitempty"`
}

// kubeconfig is the part of `kubectl config view -o json` Flint reads
// (without --raw, kubectl redacts certificate and token data anyway).
type kubeconfig struct {
	CurrentContext string `json:"current-context"`
	Contexts       []struct {
		Name    string `json:"name"`
		Context struct {
			Cluster   string `json:"cluster"`
			User      string `json:"user"`
			Namespace string `json:"namespace"`
		} `json:"context"`
	} `json:"contexts"`
	Users []struct {
		Name string `json:"name"`
		User struct {
			Exec *struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
				Env     []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"env"`
			} `json:"exec"`
			Token        string          `json:"token"`
			TokenFile    string          `json:"tokenFile"`
			ClientCert   string          `json:"client-certificate"`
			ClientCertD  string          `json:"client-certificate-data"`
			AuthProvider json.RawMessage `json:"auth-provider"`
		} `json:"user"`
	} `json:"users"`
}

// Contexts lists the contexts of the kubeconfig (KUBECONFIG is honoured by
// kubectl itself), the current one first.
func (k *Kubectl) Contexts(ctx context.Context) ([]Context, error) {
	out, err := k.run(ctx, readTimeout, "config", "view", "-o", "json")
	if err != nil {
		return nil, err
	}
	return parseContexts(out)
}

func parseContexts(data []byte) ([]Context, error) {
	var cfg kubeconfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unexpected kubectl config output: %w", err)
	}
	users := map[string]Context{}
	for _, u := range cfg.Users {
		var c Context
		switch {
		case u.User.Exec != nil:
			c.Auth = "exec:" + baseName(u.User.Exec.Command)
			for _, e := range u.User.Exec.Env {
				if e.Name == "AWS_PROFILE" {
					c.AWSProfile = e.Value
				}
			}
			if i := slices.Index(u.User.Exec.Args, "--profile"); i >= 0 && i+1 < len(u.User.Exec.Args) {
				c.AWSProfile = u.User.Exec.Args[i+1]
			}
		case u.User.ClientCert != "" || u.User.ClientCertD != "":
			c.Auth = "certificate"
		case u.User.Token != "" || u.User.TokenFile != "":
			c.Auth = "token"
		case len(u.User.AuthProvider) > 0 && string(u.User.AuthProvider) != "null":
			c.Auth = "auth-provider"
		}
		users[u.Name] = c
	}
	out := make([]Context, 0, len(cfg.Contexts))
	for _, c := range cfg.Contexts {
		auth := users[c.Context.User]
		out = append(out, Context{
			Name: c.Name, Cluster: c.Context.Cluster, User: c.Context.User, Namespace: c.Context.Namespace,
			Current: c.Name == cfg.CurrentContext, Auth: auth.Auth, AWSProfile: auth.AWSProfile,
		})
	}
	slices.SortStableFunc(out, func(a, b Context) int {
		if a.Current != b.Current {
			if a.Current {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out, nil
}

func baseName(cmd string) string {
	if i := strings.LastIndexByte(cmd, '/'); i >= 0 {
		return cmd[i+1:]
	}
	return cmd
}

// UseContext makes name the kubeconfig's current context (for every terminal
// and tool, not only Flint).
func (k *Kubectl) UseContext(ctx context.Context, name string) error {
	if !validContext(name) {
		return fmt.Errorf("invalid context %q", name)
	}
	_, err := k.run(ctx, actionTimeout, "config", "use-context", name)
	return err
}

// Namespaces lists the cluster's namespaces (an error when not allowed).
func (k *Kubectl) Namespaces(ctx context.Context, kubeContext string) ([]string, error) {
	args, err := scope(kubeContext, "")
	if err != nil {
		return nil, err
	}
	out, err := k.run(ctx, readTimeout, append(args, "get", "namespaces", "-o", "json")...)
	if err != nil {
		return nil, err
	}
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("unexpected kubectl output: %w", err)
	}
	names := make([]string, 0, len(list.Items))
	for _, it := range list.Items {
		names = append(names, it.Metadata.Name)
	}
	slices.Sort(names)
	return names, nil
}
