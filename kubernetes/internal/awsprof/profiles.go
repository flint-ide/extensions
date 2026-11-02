// Package awsprof lists the AWS CLI profiles of ~/.aws (config and
// credentials) with how each gets credentials and whether they are still
// valid. Secrets are read only to tell that they exist; they never leave here.
package awsprof

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Kinds of profiles, by where their credentials come from.
const (
	KindSSO       = "sso"       // IAM Identity Center: sso_start_url / sso_session
	KindTemporary = "temporary" // session keys written by saml2aws and similar tools
	KindKeys      = "keys"      // long-term access keys
	KindRole      = "role"      // role_arn assumed from source_profile
	KindProcess   = "process"   // credential_process
	KindConfig    = "config"    // settings only (region…), no credentials
)

// Statuses of a profile's credentials.
const (
	StatusValid     = "valid"      // ExpiresAt is in the future
	StatusExpired   = "expired"    // log in again
	StatusLoggedOut = "logged-out" // SSO without a cached token
	StatusStatic    = "static"     // long-term keys, no expiry
	StatusUnknown   = "unknown"    // decided when used (credential_process, roles)
)

// Profile is one AWS CLI profile, without secrets.
type Profile struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Region  string `json:"region,omitempty"`
	Account string `json:"account,omitempty"`
	Role    string `json:"role,omitempty"`
	// Source is the profile a role is assumed from.
	Source string `json:"source,omitempty"`
	// ExpiresAt (RFC 3339) of the SSO token or the temporary keys.
	ExpiresAt string `json:"expiresAt,omitempty"`
	// Login is the command that refreshes the credentials, empty when there is none.
	Login []string `json:"login,omitempty"`
	// Refreshable: Refresh can renew the credentials without a terminal.
	Refreshable bool `json:"refreshable"`
}

// Files are where profiles are read from (AWS_CONFIG_FILE and
// AWS_SHARED_CREDENTIALS_FILE override the defaults under ~/.aws).
type Files struct {
	Config      string
	Credentials string
	SSOCache    string
	// SAML2AWS is saml2aws's config (~/.saml2aws, SAML2AWS_CONFIGFILE): its IdP
	// accounts tell which one logs in to a profile.
	SAML2AWS string
}

// DefaultFiles follows the AWS CLI's lookup.
func DefaultFiles() (Files, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Files{}, err
	}
	f := Files{
		Config:      filepath.Join(home, ".aws", "config"),
		Credentials: filepath.Join(home, ".aws", "credentials"),
		SSOCache:    filepath.Join(home, ".aws", "sso", "cache"),
		SAML2AWS:    filepath.Join(home, ".saml2aws"),
	}
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" {
		f.Config = p
	}
	if p := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); p != "" {
		f.Credentials = p
	}
	if p := os.Getenv("SAML2AWS_CONFIGFILE"); p != "" {
		f.SAML2AWS = p
	}
	return f, nil
}

// Load lists the profiles of config and credentials, sorted with default first.
func Load(files Files, now time.Time) ([]Profile, error) {
	config, err := readINI(files.Config)
	if err != nil {
		return nil, err
	}
	creds, err := readINI(files.Credentials)
	if err != nil {
		return nil, err
	}
	sessions := map[string]map[string]string{}
	settings := map[string]map[string]string{}
	var names []string
	for _, s := range config {
		switch {
		case s.name == "default":
			settings["default"] = s.values
			names = append(names, "default")
		case strings.HasPrefix(s.name, "profile "):
			name := strings.TrimSpace(strings.TrimPrefix(s.name, "profile "))
			settings[name] = s.values
			names = append(names, name)
		case strings.HasPrefix(s.name, "sso-session "):
			sessions[strings.TrimSpace(strings.TrimPrefix(s.name, "sso-session "))] = s.values
		}
	}
	keys := map[string]map[string]string{}
	for _, s := range creds {
		keys[s.name] = s.values
		names = append(names, s.name)
	}
	slices.Sort(names)
	names = slices.Compact(names)
	if i := slices.Index(names, "default"); i > 0 {
		names = append([]string{"default"}, slices.Delete(names, i, i+1)...)
	}
	// Only a convenience for the saml2aws login: an unreadable file leaves it as before.
	var accounts map[string]string
	if files.SAML2AWS != "" {
		if idp, err := readINI(files.SAML2AWS); err == nil {
			accounts = idpAccounts(idp)
		}
	}
	out := make([]Profile, 0, len(names))
	for _, name := range names {
		out = append(out, describe(name, settings[name], keys[name], sessions, files.SSOCache, accounts[name], now))
	}
	return out, nil
}

// idpAccounts maps AWS profiles to the saml2aws IdP account that logs in to them:
// the account whose aws_profile is the profile (the first one wins), else an
// account named like the profile. Without --idp-account saml2aws uses "default",
// which fails with "URL empty in idp account" when there is no such account.
func idpAccounts(sections []section) map[string]string {
	out := map[string]string{}
	for _, s := range sections {
		if profile := s.values["aws_profile"]; profile != "" {
			if _, seen := out[profile]; !seen {
				out[profile] = s.name
			}
		}
	}
	for _, s := range sections {
		if _, seen := out[s.name]; !seen {
			out[s.name] = s.name
		}
	}
	return out
}

func readINI(path string) ([]section, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseINI(data), nil
}

// describe works out a profile's kind and status from its settings (config)
// and keys (credentials).
func describe(name string, conf, keys map[string]string, sessions map[string]map[string]string, ssoCache, idpAccount string, now time.Time) Profile {
	get := func(k string) string {
		if v := keys[k]; v != "" {
			return v
		}
		return conf[k]
	}
	p := Profile{Name: name, Region: get("region"), Kind: KindConfig, Status: StatusUnknown}
	switch {
	case conf["sso_session"] != "" || conf["sso_start_url"] != "":
		p.Kind = KindSSO
		p.Account, p.Role = conf["sso_account_id"], conf["sso_role_name"]
		p.Login = []string{"aws", "sso", "login", "--profile", name}
		cacheKey := conf["sso_start_url"]
		if s := conf["sso_session"]; s != "" {
			cacheKey = s
			if p.Region == "" {
				p.Region = sessions[s]["sso_region"]
			}
		}
		p.ExpiresAt, p.Status = ssoToken(ssoCache, cacheKey, now)
	case keys["aws_session_token"] != "" || keys["aws_security_token"] != "":
		p.Kind = KindTemporary
		p.Status = StatusUnknown
		for _, k := range []string{"x_security_token_expires", "aws_expiration", "expiration"} {
			if t, ok := parseTime(keys[k]); ok {
				p.ExpiresAt, p.Status = t.UTC().Format(time.RFC3339), expiry(t, now)
				break
			}
		}
		p.Account, p.Role = fromARN(keys["x_principal_arn"])
		if keys["x_principal_arn"] != "" && p.Account != "" && p.Role != "" {
			// Written by saml2aws: log in again into the same role.
			p.Login = []string{"saml2aws", "login"}
			if idpAccount != "" {
				p.Login = append(p.Login, "--idp-account", idpAccount)
			}
			p.Login = append(p.Login, "--profile", name, "--role", "arn:aws:iam::"+p.Account+":role/"+p.Role)
		}
	case keys["aws_access_key_id"] != "":
		p.Kind, p.Status = KindKeys, StatusStatic
	case conf["role_arn"] != "":
		p.Kind, p.Source = KindRole, conf["source_profile"]
		p.Account, p.Role = fromARN(conf["role_arn"])
	case conf["credential_process"] != "":
		p.Kind = KindProcess
	}
	if p.Account == "" {
		p.Account, _ = fromARN(conf["role_arn"])
	}
	_, _, p.Refreshable = refreshCommand(p)
	return p
}

// ssoToken reads the cached IAM Identity Center token for an sso_session name
// or a legacy sso_start_url (the cache file is named by its SHA-1).
func ssoToken(dir, key string, now time.Time) (string, string) {
	if key == "" {
		return "", StatusLoggedOut
	}
	sum := sha1.Sum([]byte(key))
	data, err := os.ReadFile(filepath.Join(dir, hex.EncodeToString(sum[:])+".json"))
	if err != nil {
		return "", StatusLoggedOut
	}
	var token struct {
		ExpiresAt string `json:"expiresAt"`
	}
	if json.Unmarshal(data, &token) != nil {
		return "", StatusLoggedOut
	}
	t, ok := parseTime(token.ExpiresAt)
	if !ok {
		return "", StatusUnknown
	}
	return t.UTC().Format(time.RFC3339), expiry(t, now)
}

func expiry(t, now time.Time) string {
	if t.After(now) {
		return StatusValid
	}
	return StatusExpired
}

// parseTime accepts RFC 3339 and the "…UTC" form older AWS CLIs wrote.
func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if strings.HasSuffix(s, "UTC") {
		s = strings.TrimSuffix(s, "UTC") + "Z"
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

// fromARN takes the account and role (or user) out of an IAM/STS ARN:
// arn:aws:sts::123456789012:assumed-role/Dev/jane → 123456789012, Dev.
func fromARN(arn string) (account, role string) {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) != 6 || parts[0] != "arn" {
		return "", ""
	}
	resource := strings.Split(parts[5], "/")
	if len(resource) > 1 {
		role = resource[1]
	}
	return parts[4], role
}
