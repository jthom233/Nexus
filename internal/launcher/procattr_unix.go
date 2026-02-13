//go:build !windows

package launcher

import (
	"os/exec"
	"syscall"
)

// setProcGroup starts the command in its own process group so it survives TUI exit.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
