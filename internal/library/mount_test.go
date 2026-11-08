package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josegamboa/karaokenino/internal/config"
)

func TestParseBlkid(t *testing.T) {
	out := `/dev/sda1: UUID="1111" TYPE="ext4" PARTUUID="a"
/dev/sda2: TYPE="swap"
/dev/sdb1: LABEL="DISCO MUSICA" UUID="2222" TYPE="ntfs" PTTYPE="dos"
/dev/sdc1: LABEL="USB" TYPE="vfat"
/dev/loop0: TYPE="squashfs"
`
	got := parseBlkid(out)
	if len(got) != 3 || got[1].Dev != "/dev/sdb1" || got[1].Label != "DISCO MUSICA" || got[1].Type != "ntfs" {
		t.Fatalf("%+v", got)
	}
}

func TestParseMounts(t *testing.T) {
	m := parseMounts("/dev/sda1 / ext4 rw 0 0\ntmpfs /tmp tmpfs rw 0 0\n/dev/sdc1 /mnt/sdc1 vfat ro 0 0\n")
	if !m["/dev/sda1"] || !m["/dev/sdc1"] || m["tmpfs"] {
		t.Fatalf("%v", m)
	}
}

// AutoMount with fake commands: sdb1 holds master.1000, sdc1 doesn't.
func TestAutoMountKeepsTheMusicDisk(t *testing.T) {
	if _, err := os.Stat("/mnt"); err != nil {
		t.Skip("sin /mnt")
	}
	fake := t.TempDir()
	var calls []string
	old := runCmd
	defer func() { runCmd = old }()
	runCmd = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		switch name {
		case "blkid":
			return []byte("/dev/kn-test-sdb1: TYPE=\"ntfs\" LABEL=\"M\"\n/dev/kn-test-sdc1: TYPE=\"vfat\"\n"), nil
		case "mount":
			mp := args[len(args)-1]
			// Simulate the content of the mounted disk with a symlink-free copy.
			if strings.HasSuffix(mp, "kn-test-sdb1") {
				os.MkdirAll(filepath.Join(fake, "sdb1", "master.1000", "karaokes"), 0o755)
			}
			return nil, nil
		}
		return nil, nil
	}
	l := New(config.LibraryConfig{DirName: "master.1000", AutoMount: true})
	// We can't really mount in a test; check the commands and parsing path.
	_ = l.AutoMount()
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "mount -o ro /dev/kn-test-sdb1 /mnt/kn-test-sdb1") {
		t.Fatalf("no montó de solo lectura:\n%s", joined)
	}
	// Second call: devices already tried are skipped.
	calls = nil
	_ = l.AutoMount()
	if strings.Contains(strings.Join(calls, "\n"), "mount ") {
		t.Fatalf("volvió a intentar: %v", calls)
	}
}
