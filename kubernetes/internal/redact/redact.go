// Package redact takes credentials and the home directory out of text that
// leaves Flint's control: bug reports, logs, and context sent to AI agents.
package redact

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// secretPatterns match credentials by their shape; each is replaced by its
// label. Private keys come first: a BEGIN/END pair, then a key cut off by the
// end of the text, then one whose BEGIN line was cut off at the start.
var secretPatterns = []struct {
	re    *regexp.Regexp
	label string
}{
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), "[private key]"},
	{regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*$`), "[private key]"},
	{regexp.MustCompile(`\A[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`), "[private key]"},
	{regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`), "[aws key id]"},
	{regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr)_[A-Za-z0-9]{20,}\b|\bgithub_pat_[A-Za-z0-9_]{20,}\b`), "[github token]"},
	{regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`), "[slack token]"},
	{regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`), "[api key]"},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`), "[jwt]"},
	{regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`), "$1 [redacted]"},
	{regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^\s/:@]+:[^\s/@]+@`), "$1[credentials]@"},
}

// keyValue matches an assignment to a name that says it holds a secret:
// DB_PASSWORD=…, dbPassword = "…", "api_key": "…", token: …. The name may
// carry any identifier prefix or suffix (AWS_SECRET_ACCESS_KEY, clientSecret).
// Groups: 1 line start (export), 2 name, 3 separator, then the value: 4 "…",
// 5 '…', 6 `…`, 7 unquoted.
var keyValue = regexp.MustCompile(`(?im)(^[ \t]*(?:export[ \t]+)?)?\b([a-z0-9_.-]*(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|authorization|credentials?)[a-z0-9_]*)` +
	`(["']?[ \t]*(?::=|[:=])[ \t]*)` +
	`(?:"([^"\n]*)"|'([^'\n]*)'|` + "`([^`\\n]*)`" + `|([^\s"'` + "`" + `,;\[][^\s"'` + "`" + `,;]*))`)

// codeValue: an unquoted value that is code, not a secret — a call, a member
// access (process.env.X, cfg.Secret), a variable reference, an operator.
var codeValue = regexp.MustCompile(`^(?:[=>(){}\[&*!<|$#@]|[A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)+|.*\()`)

// typeNames are unquoted values that name a type or a constant (token: string).
var typeNames = map[string]bool{
	"string": true, "str": true, "bool": true, "boolean": true, "int": true, "number": true, "any": true,
	"bytes": true, "byte": true, "none": true, "null": true, "nil": true, "undefined": true, "true": true,
	"false": true, "new": true, "const": true, "let": true, "var": true, "func": true, "function": true,
	"lambda": true, "await": true, "optional": true, "[]byte": true,
}

// labels are what an earlier pattern left behind ("[github token]").
var labelValue = regexp.MustCompile(`^\[[a-z ]+\]$`)

// String removes credentials and replaces the home directory with ~ (user
// names and project paths are personal).
func String(s string) string {
	s = Secrets(s)
	if home, err := os.UserHomeDir(); err == nil && len(home) > 1 {
		s = strings.ReplaceAll(s, home, "~")
	}
	return s
}

// Secrets removes credentials only (paths stay usable, e.g. in saved sessions).
// It errs on the side of redacting: an unquoted value of a secret-named key is
// redacted unless it is plainly code.
func Secrets(s string) string {
	for _, p := range secretPatterns {
		s = p.re.ReplaceAllString(s, p.label)
	}
	return keyValue.ReplaceAllStringFunc(s, func(m string) string {
		kv := parseKeyValue(m, keyValue.FindStringSubmatchIndex(m))
		if !secretValue(kv.value, kv.quote) {
			return m
		}
		return kv.lead + kv.name + kv.sep + kv.quote + "[redacted]" + kv.quote
	})
}

// HasSecretValue reports whether s holds an actual credential: a known token
// shape, a private key, credentials in a URL, a quoted literal assigned to a
// secret-named key, or an env-file line (DB_PASSWORD=hunter2). Unlike Secrets
// it ignores unquoted values in code (password = pw, token: Token).
func HasSecretValue(s string) bool {
	for _, p := range secretPatterns {
		if p.re.MatchString(s) {
			return true
		}
	}
	for _, idx := range keyValue.FindAllStringSubmatchIndex(s, -1) {
		kv := parseKeyValue(s, idx)
		if !secretValue(kv.value, kv.quote) {
			continue
		}
		if kv.quote != "" || (kv.lineStart && kv.sep == "=") {
			return true
		}
	}
	return false
}

// keyValueMatch is one keyValue match taken apart.
type keyValueMatch struct {
	lead, name, sep, value, quote string
	lineStart                     bool // the name starts its line (an env file)
}

func parseKeyValue(s string, idx []int) keyValueMatch {
	group := func(i int) string {
		if idx[2*i] < 0 {
			return ""
		}
		return s[idx[2*i]:idx[2*i+1]]
	}
	kv := keyValueMatch{lead: group(1), name: group(2), sep: group(3), lineStart: idx[2] >= 0}
	for i, q := range []string{`"`, `'`, "`"} {
		if idx[2*(4+i)] >= 0 {
			kv.value, kv.quote = group(4+i), q
			return kv
		}
	}
	kv.value = group(7)
	return kv
}

func secretValue(value, quote string) bool {
	switch {
	case value == "" || labelValue.MatchString(value):
		return false
	case quote != "":
		return !strings.HasPrefix(value, "${") && !strings.HasPrefix(value, "{{")
	case strings.HasPrefix(value, "${") || codeValue.MatchString(value):
		return false
	}
	return !typeNames[strings.ToLower(value)]
}

// SecretFileGlobs are gitignore-style name patterns for the files SecretFile
// recognises, for tools that take patterns (an agent's deny rules). SSH keys
// are listed by their usual names: "id_*" would also deny id_test.go.
func SecretFileGlobs() []string {
	return []string{
		".env", ".env.*", ".envrc", ".dev.vars", ".npmrc", ".netrc", "_netrc", ".pypirc", ".pgpass", ".git-credentials",
		"credentials", "kubeconfig", "*.kubeconfig", ".kube/config",
		"id_rsa", "id_dsa", "id_ecdsa", "id_ed25519", "id_ecdsa_sk", "id_ed25519_sk",
		"*.tfvars", "*.tfvars.json", "*.tfstate", "*.tfstate.backup",
		"secrets.yaml", "secrets.yml", "secrets.json", "secrets.toml", "secrets.env",
		"*.pem", "*.key", "*.p12", "*.pfx", "*.jks", "*.keystore", "*.gpg", "*.asc",
	}
}

// secretsExt are the data formats of a "secrets.<ext>" file (secrets.go is code).
var secretsExt = map[string]bool{"yaml": true, "yml": true, "json": true, "toml": true, "env": true}

// SecretFile reports whether path names a file that holds credentials as a
// whole (.env files, keys, certificates, SSH keys, cloud and package-manager
// credentials, kubeconfig): its content is never sent to a model.
func SecretFile(path string) bool {
	slashed := filepath.ToSlash(path)
	base := strings.ToLower(filepath.Base(path))
	switch {
	case base == ".env" || strings.HasPrefix(base, ".env."), base == ".envrc", base == ".dev.vars",
		base == ".npmrc", base == ".netrc", base == "_netrc", base == ".pypirc", base == ".pgpass",
		base == ".git-credentials", base == "credentials", base == "kubeconfig", strings.HasSuffix(base, ".kubeconfig"),
		strings.HasPrefix(base, "id_") && !strings.Contains(base, "."):
		return true
	case strings.HasSuffix(base, ".tfvars"), strings.HasSuffix(base, ".tfvars.json"),
		strings.HasSuffix(base, ".tfstate"), strings.HasSuffix(base, ".tfstate.backup"):
		return true // Terraform variables and state hold provider keys and resource secrets
	case strings.HasPrefix(base, "secrets.") && secretsExt[strings.TrimPrefix(base, "secrets.")]:
		return true
	case strings.HasSuffix(slashed, "/.kube/config") || slashed == ".kube/config":
		return true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore", ".gpg", ".asc":
		return true
	}
	return false
}
