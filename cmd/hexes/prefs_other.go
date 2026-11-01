//go:build !js

package main

import (
	"os"
	"path/filepath"
)

// Desktop builds keep settings and the save as small files in the user's
// config folder (hexes/<key>). Errors read as "nothing saved".
func prefPath(key string) string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "hexes", key)
}

func loadPref(key string) string {
	p := prefPath(key)
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// savePref writes through a temporary file so a crash mid-write can't
// leave half a save.
func savePref(key, val string) {
	p := prefPath(key)
	if p == "" || os.MkdirAll(filepath.Dir(p), 0o755) != nil {
		return
	}
	tmp := p + ".tmp"
	if os.WriteFile(tmp, []byte(val), 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, p)
}
