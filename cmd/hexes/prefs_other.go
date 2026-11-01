//go:build !js

package main

// Desktop builds don't keep settings between runs yet.
func loadPref(key string) string { return "" }

func savePref(key, val string) {}
