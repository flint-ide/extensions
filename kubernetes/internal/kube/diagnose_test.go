package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnoseGathersStateAndRedactedLogs(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"core.json": appsCore, "ingress.json": appsIngress} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	script := `#!/bin/sh
case "$*" in
  *" get deployments,"*) cat ` + dir + `/core.json ;;
  *" get ingresses "*) cat ` + dir + `/ingress.json ;;
  *" get events "*) echo '{"items": []}' ;;
  *"--previous"*) echo "panic: out of memory (AWS key AKIAIOSFODNN7EXAMPLE)" ;;
  *" logs "*) echo "connecting with password=hunter2" ;;
  *) echo "error: the server doesn't have a resource type" >&2; exit 1 ;;
esac
`
	bin := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	k := &Kubectl{bin: bin}
	out, err := k.Diagnose(t.Context(), "prod", "shop", "flux:flux-system/shop")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Namespace: shop", "App: shop (flux)", "Deployment shop: 1/2 ready", "acme/shop:1.4.0",
		"pod shop-new-a: CrashLoopBackOff", "OOMKilled (exit 137)", "shop-old-b", "(previous revision)",
		"Ingress shop: shop.example.com/ → shop:80", "logs of the previous (crashed) run, pod shop-new-a container app",
		"panic: out of memory", "password=[redacted]", "[aws key id]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "hunter2") || strings.Contains(out, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("secret sent to the agent:\n%s", out)
	}
	if _, err := k.Diagnose(t.Context(), "prod", "shop", "label:gone"); err == nil {
		t.Error("unknown app accepted")
	}
}
