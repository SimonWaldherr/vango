//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/SimonWaldherr/vango"
)

var wasmVersion = "dev"

func registerVangoAPI() {
	api := js.Global().Get("Object").New()
	api.Set("ready", js.ValueOf(true))
	api.Set("version", js.ValueOf(wasmVersion))
	names := vango.EffectNames()
	// js.ValueOf requires []any for JS arrays; []string is not accepted directly.
	effects := make([]any, len(names))
	for i, e := range names {
		effects[i] = e
	}
	api.Set("effects", js.ValueOf(effects))
	js.Global().Set("vango", api)
}

func main() {
	registerVangoAPI()
	select {}
}
