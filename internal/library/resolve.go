package library

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Resolve returns an existing path for p. The catalog comes from Windows,
// where names are case-insensitive; on Linux "Song.AVI" and "song.avi" differ.
// If p does not exist as-is, each path component is matched ignoring case.
//
// Matching a component means listing its folder. A genre folder holds
// thousands of artists on a slow USB/NTFS disk, so the listings of big
// folders are remembered (re-read when a name is not in them).
func Resolve(p string) (string, error) {
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	p = filepath.Clean(p)
	cur := string(filepath.Separator)
	if !filepath.IsAbs(p) {
		cur = "."
	}
	for _, part := range strings.Split(strings.Trim(p, string(filepath.Separator)), string(filepath.Separator)) {
		next := filepath.Join(cur, part)
		if _, err := os.Lstat(next); err == nil {
			cur = next
			continue
		}
		found, err := matchFold(cur, part)
		if err != nil {
			return "", err
		}
		if found == "" {
			return "", &os.PathError{Op: "resolve", Path: p, Err: os.ErrNotExist}
		}
		cur = filepath.Join(cur, found)
	}
	return cur, nil
}

// Folders with fewer entries are cheap to list again and are not kept.
const (
	minCachedEntries = 32
	maxCachedDirs    = 64
)

var listings = struct {
	sync.Mutex
	m map[string]map[string]string // folder -> lower(name) -> name
}{m: map[string]map[string]string{}}

// matchFold returns the entry of dir equal to part ignoring case ("" if none).
func matchFold(dir, part string) (string, error) {
	key := strings.ToLower(part)
	listings.Lock()
	names := listings.m[dir]
	listings.Unlock()
	if name, ok := names[key]; ok {
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			return name, nil
		}
	}
	// Not listed yet, or the folder changed since: read it again.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	names = make(map[string]string, len(entries))
	for _, e := range entries {
		k := strings.ToLower(e.Name())
		if _, dup := names[k]; !dup { // first in name order, like before
			names[k] = e.Name()
		}
	}
	if len(entries) >= minCachedEntries {
		listings.Lock()
		if len(listings.m) >= maxCachedDirs {
			listings.m = map[string]map[string]string{}
		}
		listings.m[dir] = names
		listings.Unlock()
	}
	return names[key], nil
}

// ForgetListings drops remembered folder listings (after the disk changes).
func ForgetListings() {
	listings.Lock()
	listings.m = map[string]map[string]string{}
	listings.Unlock()
}
