# AWS Profiles

The AWS CLI profiles of `~/.aws` with the state of their credentials: how much time an SSO login or session keys have left, which profiles are logged out, and who each one is. From a profile you can log in, refresh its credentials without a terminal, and open a terminal working as it. This is Flint's former built-in AWS panel as a separate extension.

| | |
|---|---|
| Id | `flint.aws` |
| Version | 0.1.0 |
| Contract | 1, with terminal tabs (Flint with `window/openTerminal`) |
| Permissions | `terminal` |
| Starts | when its panel is shown or its command runs |

## What it adds

- **Panel "AWS Profiles"**: a tree with one node per profile (`default` first). The description shows the credentials' state (`2h 05m` left, `expired`, `logged out`, `keys` for long-term access keys, `—` when it is decided on use), the kind (SSO, session keys, access keys, assumed role, credential process, settings only), the account and the region; the tooltip adds the role and when the credentials expire. Expand a profile for its details, the last identity check and the last credential refresh. With no profiles, the panel says so (and whether the AWS CLI is missing). A profile's context menu:
  - **Open Terminal**: a terminal tab with `AWS_PROFILE` (and `AWS_REGION` when the profile has a region), tagged `aws:<profile>`.
  - **Refresh Credentials** (SSO, saml2aws session keys, assumed roles, credential processes): renews them without a terminal (`aws sso login --profile <p>`, `saml2aws login … --skip-prompt --force`, or `aws sts get-caller-identity --profile <p>`, which makes the CLI fetch and cache role credentials), with a toast when it is done or failed. SSO waits up to 5 minutes for you to approve in the browser; **Stop Refreshing** ends it. When the login needs an answer (MFA code, password), Flint offers **Log In** instead.
  - **Log In** (SSO and saml2aws profiles): a terminal tab titled `aws login: <profile>` that runs the same command, with the profile's environment.
  - **Check Identity**: `aws sts get-caller-identity --profile <p>`; the account and ARN (or the error) show under the profile.
- **Command**: **AWS: Refresh AWS Profiles** reads `~/.aws` again.

## Settings

In **Settings ▸ AWS Profiles**, stored in `settings.json` under `extensions.flint.aws`:

| Setting | Default | Meaning |
|---|---|---|
| `refreshSeconds` | `30` | How often profiles and credential expiry are re-read while the panel is shown (5–3600); nothing runs while it is hidden |

## Privacy

It reads `~/.aws/config` and `~/.aws/credentials`, the expiry time from the IAM Identity Center token cache (`~/.aws/sso/cache`), and the IdP account names from `~/.saml2aws`. In the credentials file it looks only at which keys exist (to tell access keys from session keys) and at the expiry and principal ARN saml2aws writes; secret values never leave the process and are never shown, logged or sent to Flint.

Re-reading the panel only reads those files. The network is used only when you ask: **Check Identity** and **Refresh Credentials** run the AWS CLI (or saml2aws), which talk to AWS STS, IAM Identity Center or your identity provider; SSO opens your browser. With the `terminal` permission it opens terminal tabs for **Open Terminal** and **Log In**; you see what runs and can stop it. It stores nothing: no files of its own (`FLINT_EXTENSION_DATA` is unused) and nothing outside Flint's settings.

The manifest asks Flint for `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE`, `AWS_CA_BUNDLE` and `SAML2AWS_CONFIGFILE` from your environment (`run.env`), so the panel reads the same files as the AWS CLI in your shell. Terminals it opens are ordinary Flint terminals with your shell's environment.

## Build and install

Needs Go 1.27 or newer; only the standard library is used. The AWS CLI (`aws`) is needed to log in, refresh and check identities, and `saml2aws` for saml2aws profiles; listing profiles needs neither.

```bash
go build -o aws-ext .
```

In Flint, run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The manifest: command, panel, settings schema, permissions |
| `main.go` | Requests and notifications from Flint, settings |
| `view.go` | The panel's tree: profiles, their details and context menus |
| `status.go` | Credential status, labels and a profile's terminal environment |
| `actions.go` | Terminals, logins, credential refreshes and identity checks |
| `poll.go` | Re-reading the panel while it is shown |
| `rpc.go` | JSON-RPC over stdin and stdout |
| `internal/awsprof/` | Reading `~/.aws`, the refresh commands and `sts get-caller-identity` (from Flint's `internal/awsprof`) |
