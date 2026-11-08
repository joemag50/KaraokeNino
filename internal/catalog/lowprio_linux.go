package catalog

import (
	"runtime"
	"syscall"
)

// runLowPriority runs fn on its own OS thread with nice 10 and the lowest
// best-effort I/O priority, so a disk scan yields to mpv reading the video
// from the same disk (honored by the BFQ scheduler and kernel file systems;
// NTFS through ntfs-3g/FUSE reads in its own process, where only the scan's
// pacing helps).
func runLowPriority(fn func() error) error {
	done := make(chan error, 1)
	go func() {
		// Never unlocked: the thread exits with this goroutine and takes
		// its lower priority with it.
		runtime.LockOSThread()
		tid := syscall.Gettid()
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, tid, 10)
		const whoProcess, classBestEffort, lowest = 1, 2, 7
		_, _, _ = syscall.Syscall(syscall.SYS_IOPRIO_SET, whoProcess, uintptr(tid), classBestEffort<<13|lowest)
		done <- fn()
	}()
	return <-done
}
