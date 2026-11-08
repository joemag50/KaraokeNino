package player

import (
	"os/exec"
	"syscall"
)

// dieWithParent makes the kernel kill mpv if KaraokeNino dies.
func dieWithParent(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
