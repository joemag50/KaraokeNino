//go:build unix

package maint

import "syscall"

// DiskFree returns the bytes available to the program and the size of the
// file system holding path.
func DiskFree(path string) (free, total uint64, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bs := uint64(st.Bsize)
	return st.Bavail * bs, st.Blocks * bs, nil
}
