package redact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	home, _ := os.UserHomeDir()
	in := strings.Join([]string{
		"open " + filepath.Join(home, "projects", "acme") + " failed",
		"key AKIAIOSFODNN7EXAMPLE and ASIAABCDEFGHIJKLMNOP",
		"token ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"Authorization: Bearer abc.def.ghi-123456",
		`{"password": "hunter2", "api_key":"k-123"}`,
		"postgres://admin:s3cret@db.internal:5432/app",
		"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.SflKxwRJSMeKKF2QT4fwpM",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIEow\n-----END RSA PRIVATE KEY-----",
	}, "\n")
	out := String(in)
	for _, leak := range []string{home, "AKIAIOSFODNN7EXAMPLE", "ASIAABCDEFGHIJKLMNOP", "ghp_abc", "abc.def.ghi", "hunter2", "k-123", "s3cret", "eyJzdWIi", "MIIEow"} {
		if strings.Contains(out, leak) {
			t.Errorf("%q survived:\n%s", leak, out)
		}
	}
	if got := String("token=ghp_abcdefghijklmnopqrstuvwxyz0123456789"); got != "token=[github token]" {
		t.Errorf("double redaction: %q", got)
	}
	for _, kept := range []string{"~/projects/acme", "db.internal:5432/app", "[aws key id]", "[private key]"} {
		if !strings.Contains(out, kept) {
			t.Errorf("%q missing:\n%s", kept, out)
		}
	}
}

func TestSecretsKeepsPaths(t *testing.T) {
	home, _ := os.UserHomeDir()
	in := filepath.Join(home, "projects") + " token=abc123456"
	if got := Secrets(in); got != filepath.Join(home, "projects")+" token=[redacted]" {
		t.Fatalf("Secrets = %q", got)
	}
}

func TestSecretsKeyNames(t *testing.T) {
	for _, in := range []string{
		"DB_PASSWORD=hunter2",
		"export AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY",
		`const dbPassword = "hunter2"`,
		`apiKey: 'hunter2'`,
		"GITHUB_TOKEN: hunter2",
		`password := "hunter2"`,
		`client_secret = "hunter2"`,
		"mySessionToken=`hunter2`",
		`"password": "my hunter2 phrase"`,
	} {
		if out := Secrets(in); strings.Contains(out, "hunter2") || strings.Contains(out, "wJalrXUtnFEMI") {
			t.Errorf("Secrets(%q) = %q", in, out)
		}
	}
}

func TestSecretsKeepsOrdinaryCode(t *testing.T) {
	for _, in := range []string{
		"token: string",
		"password: str",
		"apiKey: process.env.API_KEY,",
		`password = os.Getenv("PW")`,
		`secret := cfg.Secret()`,
		`if token == "" {`,
		"tokens := strings.Fields(line)",
		"const onToken = (t) => t",
		`password: ""`,
		"API_KEY=${API_KEY}",
	} {
		if out := Secrets(in); out != in {
			t.Errorf("Secrets(%q) = %q, want it unchanged", in, out)
		}
	}
}

func TestSecretsUnterminatedPrivateKey(t *testing.T) {
	head := "x := 1\n-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\nabcdef"
	if out := Secrets(head); strings.Contains(out, "MIIEow") || strings.Contains(out, "abcdef") || !strings.HasPrefix(out, "x := 1\n") {
		t.Errorf("unterminated key: %q", out)
	}
	tail := "MIIEowIBAAKCAQEA\nabcdef\n-----END RSA PRIVATE KEY-----\nnext := 2"
	if out := Secrets(tail); strings.Contains(out, "MIIEow") || strings.Contains(out, "abcdef") || !strings.HasSuffix(out, "next := 2") {
		t.Errorf("key whose BEGIN is cut off: %q", out)
	}
}

func TestHasSecretValue(t *testing.T) {
	for _, in := range []string{
		`password = "hunter2"`,
		"DB_PASSWORD=hunter2",
		"token ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"-----BEGIN PRIVATE KEY-----\nMIIE",
		"postgres://admin:s3cret@db:5432/app",
	} {
		if !HasSecretValue(in) {
			t.Errorf("HasSecretValue(%q) = false", in)
		}
	}
	for _, in := range []string{
		"token: string",
		"apiKey: process.env.API_KEY",
		`password = os.Getenv("PW")`,
		"password = pw",
		"token: Token",
		`password: ""`,
	} {
		if HasSecretValue(in) {
			t.Errorf("HasSecretValue(%q) = true", in)
		}
	}
}

// Every glob names files SecretFile recognises (a sample per glob: * → "x").
func TestSecretFileGlobsMatchSecretFile(t *testing.T) {
	for _, g := range SecretFileGlobs() {
		sample := strings.ReplaceAll(g, "*", "x")
		if !SecretFile(sample) {
			t.Errorf("glob %q: SecretFile(%q) = false", g, sample)
		}
		if strings.HasPrefix(g, "-") || strings.ContainsAny(g, ",() ") {
			t.Errorf("glob %q can't be a rule argument", g)
		}
	}
}

func TestSecretFile(t *testing.T) {
	for _, p := range []string{".env", "/p/.env.local", "certs/server.pem", "tls.key", "/home/u/.ssh/id_ed25519", "/home/u/.aws/credentials", ".npmrc", "/u/.netrc", "/u/.kube/config", "kubeconfig", "prod.kubeconfig", "secrets.p12",
		".envrc", "app/.dev.vars", "/u/.git-credentials", "infra/prod.tfvars", "vars.auto.tfvars.json", "terraform.tfstate",
		"terraform.tfstate.backup", "config/secrets.yaml", "secrets.yml", "secrets.json", "secrets.toml"} {
		if !SecretFile(p) {
			t.Errorf("SecretFile(%q) = false", p)
		}
	}
	for _, p := range []string{"main.go", "env.go", "src/keys.ts", "config.yaml", "id.go", "README.md", "/p/.config/app/config",
		"secrets.go", "secrets_test.go", "pkg/secrets/store.go", "secrets.md", "main.tf", "variables.tf", "envrc.go", "tfstate.go", "credentials.go"} {
		if SecretFile(p) {
			t.Errorf("SecretFile(%q) = true", p)
		}
	}
}
