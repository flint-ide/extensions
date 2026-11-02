package awsprof

import (
	"os/exec"
	"syscall"
	"time"
)

// stopTree makes a cancelled login tool stop with its children: the tools
// are often scripts that start others (aws → python, saml2aws → browser
// helpers), and a child holding the output pipe would keep Run waiting.
func stopTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
}
