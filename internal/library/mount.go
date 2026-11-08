package library

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Partition is a block device with a filesystem (from blkid).
type Partition struct {
	Dev   string // /dev/sdb1
	Type  string // ntfs, vfat, exfat, ext4...
	Label string
}

// mountableTypes are filesystems that can hold the music library.
var mountableTypes = map[string]bool{
	"ntfs": true, "ntfs3": true, "vfat": true, "exfat": true, "ext2": true, "ext3": true,
	"ext4": true, "btrfs": true, "xfs": true, "hfsplus": true, "f2fs": true,
}

// runCmd runs a command (replaced in tests).
var runCmd = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// parseBlkid reads `blkid` output (util-linux or busybox):
//
//	/dev/sdb1: LABEL="MUSICA" UUID="..." TYPE="ntfs" PARTUUID="..."
func parseBlkid(out string) []Partition {
	var parts []Partition
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		dev, rest, ok := strings.Cut(line, ":")
		if !ok || !strings.HasPrefix(dev, "/dev/") {
			continue
		}
		p := Partition{Dev: strings.TrimSpace(dev)}
		for _, kv := range splitQuoted(rest) {
			k, v, _ := strings.Cut(kv, "=")
			v = strings.Trim(v, `"`)
			switch k {
			case "TYPE":
				p.Type = strings.ToLower(v)
			case "LABEL":
				p.Label = v
			}
		}
		if mountableTypes[p.Type] {
			parts = append(parts, p)
		}
	}
	return parts
}

// splitQuoted splits `A="x y" B="z"` on spaces outside quotes.
func splitQuoted(s string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// parseMounts returns the devices currently mounted (from /proc/mounts).
func parseMounts(out string) map[string]bool {
	m := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if f := strings.Fields(line); len(f) >= 2 && strings.HasPrefix(f[0], "/dev/") {
			m[f[0]] = true
			if real, err := filepath.EvalSymlinks(f[0]); err == nil {
				m[real] = true
			}
		}
	}
	return m
}

// AutoMount mounts every unmounted partition read-only under /mnt/<name>
// (Puppy Linux mounts nothing by itself) and keeps the one holding the
// library folder dirName; the others are unmounted again. It returns the
// library root it found ("" if none). Partitions already tried are skipped.
func (l *Library) AutoMount() string {
	out, err := runCmd("blkid")
	if err != nil && len(out) == 0 {
		log.Printf("montaje automático: blkid: %v", err)
		return ""
	}
	mountsRaw, _ := os.ReadFile("/proc/mounts")
	mounted := parseMounts(string(mountsRaw))

	for _, p := range parseBlkid(string(out)) {
		l.mu.Lock()
		tried := l.tried[p.Dev]
		if l.tried == nil {
			l.tried = map[string]bool{}
		}
		l.tried[p.Dev] = true
		l.mu.Unlock()
		if tried || mounted[p.Dev] {
			continue
		}
		mp := filepath.Join("/mnt", filepath.Base(p.Dev))
		if err := mountRO(p, mp); err != nil {
			log.Printf("montaje automático: %s (%s %s): %v", p.Dev, p.Type, p.Label, err)
			continue
		}
		if root := search(mp, l.cfg.DirName, 3); root != "" {
			log.Printf("montaje automático: disco de música en %s (%s %q)", p.Dev, p.Type, p.Label)
			return root
		}
		if _, err := runCmd("umount", mp); err == nil {
			_ = os.Remove(mp)
		}
	}
	return ""
}

// mountRO mounts p read-only at mp: the music disk is only read, and an NTFS
// disk left "hibernated" by Windows can still be mounted read-only.
func mountRO(p Partition, mp string) error {
	if err := os.MkdirAll(mp, 0o755); err != nil {
		return err
	}
	attempts := [][]string{{"mount", "-o", "ro", p.Dev, mp}}
	if strings.HasPrefix(p.Type, "ntfs") {
		attempts = append(attempts, []string{"ntfs-3g", "-o", "ro", p.Dev, mp}, []string{"mount", "-t", "ntfs3", "-o", "ro", p.Dev, mp})
	}
	var errs []string
	for _, a := range attempts {
		out, err := runCmd(a[0], a[1:]...)
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v %s", a[0], err, strings.TrimSpace(string(out))))
	}
	// Without root (Debian desktop user): ask udisks, which mounts under /media.
	if os.Geteuid() != 0 {
		if out, err := runCmd("udisksctl", "mount", "--no-user-interaction", "-b", p.Dev, "-o", "ro"); err == nil {
			_ = os.Remove(mp)
			// udisks chose its own mount point; Detect finds it under /media.
			return fmt.Errorf("montado por udisks: %s", strings.TrimSpace(string(out)))
		}
	}
	_ = os.Remove(mp)
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}
