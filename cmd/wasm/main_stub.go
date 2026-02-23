//go:build !js || !wasm

package main

// Empty main for non-WASM builds so ./... builds cleanly on regular targets.
func main() {}
