package awsprof

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const configFixture = `
[default]
region = eu-central-1

[profile legacy-sso]
sso_start_url = https://acme.awsapps.com/start
sso_region = eu-west-1
sso_account_id = 111111111111
sso_role_name = Admin
region = eu-west-1

[sso-session acme]
sso_start_url = https://acme.awsapps.com/start
sso_region = us-east-1

[profile new-sso]
sso_session = acme
sso_account_id = 222222222222
sso_role_name = ReadOnly

[profile deploy]
role_arn = arn:aws:iam::333333333333:role/Deployer
source_profile = default

[profile tool]
credential_process = /usr/local/bin/creds --json

# Settings of aws-azure-login (deprecated): ignored, the profile only has a region.
[profile azure-dev]
region = eu-north-1
azure_tenant_id = t
azure_app_id_uri = https://signin.aws.amazon.com/saml
s3 =
  max_concurrent_requests = 5
`

const credentialsFixture = `
[default]
aws_access_key_id = AKIAEXAMPLE
aws_secret_access_key = SECRET-DO-NOT-LEAK

[saml-prod]
aws_access_key_id = ASIAEXAMPLE
aws_secret_access_key = SECRET2
aws_session_token = TOKEN
x_principal_arn = arn:aws:sts::444444444444:assumed-role/Ops/jane@acme.com
x_security_token_expires = 2026-10-31T12:00:00+01:00

[azure-old]
aws_access_key_id = ASIA2
aws_secret_access_key = SECRET3
aws_session_token = TOKEN2
aws_expiration = 2026-10-30T08:00:00.000Z
`

// saml2awsFixture: IdP accounts of ~/.saml2aws. "corp" logs in to saml-prod; none is named
// "default", so saml2aws without --idp-account fails with "URL empty in idp account".
const saml2awsFixture = `
[shared]
url                  = https://myapps.microsoft.com/signin/a
password             = SECRET-DO-NOT-LEAK
aws_profile          = default

[corp]
url                  = https://myapps.microsoft.com/signin/b
aws_profile          = saml-prod
`

func writeFixture(t *testing.T) Files {
	t.Helper()
	dir := t.TempDir()
	f := Files{
		Config:      filepath.Join(dir, "config"),
		Credentials: filepath.Join(dir, "credentials"),
		SSOCache:    filepath.Join(dir, "sso"),
		SAML2AWS:    filepath.Join(dir, "saml2aws"),
	}
	if err := os.MkdirAll(f.SSOCache, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{f.Config: configFixture, f.Credentials: credentialsFixture, f.SAML2AWS: saml2awsFixture} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A valid token for the sso-session, an expired one (old CLI format) for the legacy start URL.
	token := func(key, expires string) {
		sum := sha1.Sum([]byte(key))
		body := `{"accessToken":"SECRET-TOKEN","expiresAt":"` + expires + `"}`
		if err := os.WriteFile(filepath.Join(f.SSOCache, hex.EncodeToString(sum[:])+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	token("acme", "2026-10-31T18:00:00Z")
	token("https://acme.awsapps.com/start", "2026-10-31T09:00:00UTC")
	return f
}

func TestLoadDescribesEveryKind(t *testing.T) {
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	got, err := Load(writeFixture(t), now)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Profile{}
	var order []string
	for _, p := range got {
		byName[p.Name] = p
		order = append(order, p.Name)
	}
	if order[0] != "default" || len(order) != 8 {
		t.Fatalf("profiles %q", order)
	}
	check := func(name, kind, status, account, role, expires string) {
		t.Helper()
		p := byName[name]
		if p.Kind != kind || p.Status != status || p.Account != account || p.Role != role || p.ExpiresAt != expires {
			t.Errorf("%s = %+v", name, p)
		}
	}
	check("default", KindKeys, StatusStatic, "", "", "")
	check("legacy-sso", KindSSO, StatusExpired, "111111111111", "Admin", "2026-10-31T09:00:00Z")
	check("new-sso", KindSSO, StatusValid, "222222222222", "ReadOnly", "2026-10-31T18:00:00Z")
	check("deploy", KindRole, StatusUnknown, "333333333333", "Deployer", "")
	check("tool", KindProcess, StatusUnknown, "", "", "")
	check("azure-dev", KindConfig, StatusUnknown, "", "", "")
	check("saml-prod", KindTemporary, StatusValid, "444444444444", "Ops", "2026-10-31T11:00:00Z")
	check("azure-old", KindTemporary, StatusExpired, "", "", "2026-10-30T08:00:00Z")
	if p := byName["new-sso"]; p.Region != "us-east-1" || strings.Join(p.Login, " ") != "aws sso login --profile new-sso" {
		t.Errorf("new-sso region/login %+v", p)
	}
	if got := strings.Join(byName["saml-prod"].Login, " "); got != "saml2aws login --idp-account corp --profile saml-prod --role arn:aws:iam::444444444444:role/Ops" {
		t.Errorf("saml2aws login %q", got)
	}
	if byName["deploy"].Source != "default" || len(byName["azure-dev"].Login) != 0 || len(byName["azure-old"].Login) != 0 {
		t.Errorf("deploy/azure %+v %+v", byName["deploy"], byName["azure-dev"])
	}
}

func TestSAML2AWSLoginWithoutItsConfig(t *testing.T) {
	f := writeFixture(t)
	f.SAML2AWS = filepath.Join(t.TempDir(), "missing")
	got, err := Load(f, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got {
		if p.Name == "saml-prod" && strings.Join(p.Login, " ") != "saml2aws login --profile saml-prod --role arn:aws:iam::444444444444:role/Ops" {
			t.Errorf("login without ~/.saml2aws %q", p.Login)
		}
	}
}

func TestIDPAccounts(t *testing.T) {
	got := idpAccounts(parseINI([]byte(`
[by-name]
aws_profile = other
[first]
aws_profile = prod
[second]
aws_profile = prod
[stage]
aws_profile =
`)))
	want := map[string]string{"other": "by-name", "prod": "first", "by-name": "by-name", "first": "first", "second": "second", "stage": "stage"}
	if len(got) != len(want) {
		t.Fatalf("idpAccounts = %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("idpAccounts[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestLoadNeverReturnsSecrets(t *testing.T) {
	got, err := Load(writeFixture(t), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range got {
		dump := strings.Join(append([]string{p.Name, p.Kind, p.Status, p.Region, p.Account, p.Role, p.Source, p.ExpiresAt}, p.Login...), " ")
		if strings.Contains(dump, "SECRET") || strings.Contains(dump, "TOKEN") || strings.Contains(dump, "AKIA") || strings.Contains(dump, "ASIA") {
			t.Fatalf("secret leaked in %+v", p)
		}
	}
}

func TestMissingFilesAreNoProfiles(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(Files{Config: filepath.Join(dir, "x"), Credentials: filepath.Join(dir, "y")}, time.Now())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestCallerIdentity(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\n[ \"$3\" = \"--profile\" ] || exit 9\nif [ \"$4\" = bad ]; then echo 'The SSO session has expired' >&2; exit 255; fi\n" +
		"echo '{\"UserId\":\"AID:jane\",\"Account\":\"222222222222\",\"Arn\":\"arn:aws:sts::222222222222:assumed-role/ReadOnly/jane\"}'\n"
	if err := os.WriteFile(filepath.Join(bin, "aws"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	id, err := CallerIdentity(t.Context(), "new-sso")
	if err != nil || id.Account != "222222222222" || !strings.HasSuffix(id.Arn, "/jane") {
		t.Fatalf("id %+v err %v", id, err)
	}
	if _, err := CallerIdentity(t.Context(), "bad"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("err = %v", err)
	}
	if _, err := CallerIdentity(t.Context(), "--debug"); err == nil {
		t.Fatal("flag accepted as a profile")
	}
}
