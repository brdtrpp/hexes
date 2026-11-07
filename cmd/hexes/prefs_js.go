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

// touchDevice reports whether the browser says the screen takes touches,
// so a phone shows thumb buttons from the start.
func touchDevice() (on bool) {
	defer func() {
		if recover() != nil {
			on = false
		}
	}()
	n := js.Global().Get("navigator")
	if n.IsUndefined() || n.IsNull() {
		return false
	}
	m := n.Get("maxTouchPoints")
	return m.Type() == js.TypeNumber && m.Int() > 0
}
