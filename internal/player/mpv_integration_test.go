//go:build mpv

// Run with a real mpv (e.g. in Docker): go test -tags mpv ./internal/player
package player

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeWAV creates seconds of silence.
func writeWAV(t *testing.T, path string, seconds int) {
	t.Helper()
	const rate = 8000
	n := rate * seconds
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := func(v any) { _ = binary.Write(f, binary.LittleEndian, v) }
	f.WriteString("RIFF")
	w(uint32(36 + n))
	f.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))
	w(uint16(1))
	w(uint32(rate))
	w(uint32(rate))
	w(uint16(1))
	w(uint16(8))
	f.WriteString("data")
	w(uint32(n))
	f.Write(make([]byte, n))
}

func TestRealMPVForwardsKeysAndReportsProgress(t *testing.T) {
	dir := t.TempDir()
	// Mixed case on disk; the catalog asks for lower case (Linux is case-sensitive).
	writeWAV(t, filepath.Join(dir, "Cancion.WAV"), 4)

	m := New(MPVOptions{NormalizeAudio: true, MaxVolume: 100, Volume: 50, ExtraArgs: []string{"--ao=null"}})
	keys := make(chan string, 4)
	progress := make(chan float64, 50)
	m.OnKey = func(k string) { keys <- k }
	m.OnProgress = func(pos, dur float64) {
		select {
		case progress <- dur:
		default:
		}
	}
	done, err := m.Play(Item{Title: "x", File: filepath.Join(dir, "cancion.wav"), Audio: true})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Stop()

	// Wait for the IPC connection, then simulate keypad 8 inside mpv.
	deadline := time.Now().Add(5 * time.Second)
	for {
		m.mu.Lock()
		ok := m.conn != nil
		m.mu.Unlock()
		if ok || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	m.mu.Lock()
	m.commandLocked("keypress", "KP8")
	m.commandLocked("keypress", "KP_ENTER")
	m.mu.Unlock()
	for _, want := range []string{"8", "KP_Enter"} {
		select {
		case k := <-keys:
			if k != want {
				t.Fatalf("tecla %q, quería %q", k, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("mpv no reenvió la tecla %q", want)
		}
	}
	select {
	case dur := <-progress:
		if dur != 0 && (dur < 3 || dur > 5) {
			t.Fatalf("duración %v", dur)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sin progreso de mpv")
	}
	if v := m.ChangeVolume(20); v != 100 {
		t.Fatalf("volumen %d, el máximo es 100", v)
	}
	m.ShowText("Siguiente: prueba", time.Second)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("la canción no terminó")
	}
}
