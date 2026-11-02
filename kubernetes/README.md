# Kubernetes

Shows Kubernetes clusters through `kubectl`: any context of your kubeconfig, without switching the global one. You see apps with their health and problems, plus workloads, pods, services and jobs, and you can act on them. Logs and shells open in terminal tabs. It replaces the Kubernetes panel that used to be built into Flint and works the same way. Its UI is its own page (a webview).

| | |
|---|---|
| Id | `flint.kubernetes` |
| Version | 0.1.0 |
| Contract | 1, with webviews and terminal tabs (Flint with window/openTerminal) |
| Permissions | `terminal`, `agent.ask` |
| Starts | when its panel is shown, its command runs, or an agent calls its tool |

## What it adds

- **Panel "Kubernetes"** (webview, `ui/panel.html`):
  - **Context and namespace pickers.** The global current context is left alone. The panel remembers the context you picked, the namespace you picked for each context, and the view. **★** makes the shown context the kubeconfig's current one, for every terminal and tool. The terminal button opens a shell whose `kubectl` works in the shown context and namespace (see Privacy).
  - **Apps view.** Apps come from Flux Kustomizations, Argo CD, Helm releases, `app.kubernetes.io` labels, or single workloads, worst first. Each app shows its problems, ingresses, services, workloads with their rollout and pods. **✦ AI** gathers an unhealthy app's state and its failing pods' recent logs, with secrets redacted, into the agent chat's draft for you to review and send.
  - **Resources view.** Workloads, pods, services and jobs, with a filter.
  - **Context menus.**
    - Workloads: logs, describe, rollout status and history, rollout restart, undo rollout, scale.
    - Pods: logs, shell (one per container), describe, port forward, delete.
    - Services: port forward, describe.
    - Flux Kustomizations: `flux reconcile --with-source`.
    - Restart, undo, delete and scale to 0 ask you first.
  - **EKS logins.** For a context whose user runs `aws eks get-token` with a profile, the panel shows that profile's credential status. When the credentials have expired, **Refresh** renews them without a terminal, or **Log In** runs the profile's login command (`aws sso login --profile …`, `saml2aws login …`) in a terminal tab. Flint's AWS panel offers the same. The same choice appears under an authentication error.
  - The panel re-reads the shown namespace on its interval, but only while it is visible.
- **Command Kubernetes: Refresh**: reads the kubeconfig and the namespace again.
- **Agent tool `kubernetes_status`**: lists what is unhealthy in a namespace of the kubeconfig's current context: apps with problems, workloads not rolled out, failing pods and why, failed jobs, and recent warning events. It only reads: no secrets, no logs. Turn it off in **Settings ▸ Extensions**.

## Settings

In **Settings ▸ Kubernetes**, stored in `settings.json` under `extensions.flint.kubernetes`:

| Setting | Default | Meaning |
|---|---|---|
| `refreshSeconds` | `5` | How often the panel re-reads the shown namespace while visible (2–600) |

## Privacy

- **What it reads.** Everything goes through `kubectl` on your `PATH`, with your kubeconfig and its auth plugins. Logs are read only when you open them in a terminal, or for **✦ AI**. For EKS contexts it also reads `~/.aws/config`, `~/.aws/credentials`, the SSO token cache's expiry times and `~/.saml2aws` (the files `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE` and `SAML2AWS_CONFIGFILE` name, when set). From these it learns each profile's kind, expiry and login command. Secrets never leave the process.
- **Environment.** The manifest asks Flint for `KUBECONFIG`, the AWS variables (`AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE`, `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`, `AWS_CA_BUNDLE`, `SAML2AWS_CONFIGFILE`) and those of the GKE and AKS auth plugins (`GOOGLE_APPLICATION_CREDENTIALS`, `CLOUDSDK_CONFIG`, `AZURE_CONFIG_DIR`) from your environment (`run.env`), so `kubectl` and its auth plugins see the same configuration as in your shell.
- **Network.** The extension talks only to your clusters, through `kubectl`. **Refresh** of AWS credentials runs `aws sso login`, `saml2aws` or `aws sts get-caller-identity` for that profile.
- **`terminal`.** Opens terminal tabs. The panel's commands run there (`kubectl … logs/exec/describe/port-forward/rollout`, `flux reconcile`, the AWS login), with `--context` and `--namespace` always given. The context shell gets `KUBECONFIG`, and the AWS login gets `AWS_PROFILE` and `AWS_REGION`.
- **`agent.ask`.** **✦ AI** puts the diagnosis prompt into the agent chat's draft. It is never sent without you. The gathered state is redacted (tokens, keys, passwords, the home folder) and cut to fit.
- **Storage.** Everything is stored in the extension's data folder (`FLINT_EXTENSION_DATA`): the panel's choices (`state.json`) and, for context shells, small kubeconfig files (`kube/*.yaml`, mode 0600). Those files set only `current-context` and the namespace; no credentials are copied. Nothing else is stored, and there is no telemetry.

## Build and install

Needs Go 1.27 or newer; only the standard library is used. `kubectl` must be installed. `flux` is used only to reconcile, and the AWS CLI or `saml2aws` only for EKS logins.

```bash
go build -o kubernetes-ext .
```

In Flint, run **Install Extension from Folder…** and pick this folder.

## Files

| File | What it holds |
|---|---|
| `flint-extension.json` | The manifest: the webview panel, the command, the setting, the agent tool, permissions |
| `main.go` | Requests and notifications from Flint, the agent tool call |
| `requests.go` | The page's requests (`webview/request`): cluster reads, actions, terminal tabs, the diagnosis |
| `commands.go` | The kubectl and flux command lines, value checks, the diagnosis prompt |
| `kubeconfig.go` | The context shell's override kubeconfig |
| `aws.go` | EKS logins: the profile's status, its login terminal, refreshes |
| `overview.go` | Keeping an overview under Flint's message limit |
| `state.go` | What the panel remembers |
| `status.go` | The agent tool's summary |
| `rpc.go` | JSON-RPC over stdin and stdout |
| `ui/panel.html` | The panel's page, self-contained |
| `internal/kube` | kubectl: contexts, namespaces, overview, apps and health, actions, diagnosis |
| `internal/redact` | Redaction of what the diagnosis sends |
| `internal/awsprof` | AWS profiles of `~/.aws`, their login command and refresh |
| `*_test.go` | The requests, terminal argv, prompts, agent tool and JSON-RPC, with a fake kubectl |
