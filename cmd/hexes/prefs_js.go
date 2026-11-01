//go:build js

package main

import "syscall/js"

// loadPref reads a setting from the browser's localStorage. Storage can
// be missing or throw (private windows, blocked site data); then it's "".
func loadPref(key string) (v string) {
	defer func() {
		if recover() != nil {
			v = ""
		}
	}()
	ls := js.Global().Get("localStorage")
	if ls.IsUndefined() || ls.IsNull() {
		return ""
	}
	got := ls.Call("getItem", key)
	if got.IsNull() || got.IsUndefined() {
		return ""
	}
	return got.String()
}

// savePref writes a setting to localStorage, quietly doing nothing if it
// can't.
func savePref(key, val string) {
	defer func() { recover() }()
	ls := js.Global().Get("localStorage")
	if ls.IsUndefined() || ls.IsNull() {
		return
	}
	ls.Call("setItem", key, val)
}
