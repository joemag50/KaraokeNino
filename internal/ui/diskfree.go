package ui

import (
	"fmt"
	"syscall"
)

// diskFree reports free space on the filesystem holding path.
func diskFree(path string) string {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return "?"
	}
	free := float64(st.Bavail) * float64(st.Bsize)
	total := float64(st.Blocks) * float64(st.Bsize)
	return fmt.Sprintf("%.0f GB de %.0f GB", free/1e9, total/1e9)
}
