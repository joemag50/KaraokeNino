//go:build !linux

package player

import "os/exec"

func dieWithParent(*exec.Cmd) {}
