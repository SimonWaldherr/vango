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
	effects := make([]any, 0, len(vango.EffectNames()))
	for _, e := range vango.EffectNames() {
		effects = append(effects, e)
	}
	api.Set("effects", js.ValueOf(effects))
	js.Global().Set("vango", api)
}

func main() {
	registerVangoAPI()
	select {}
}
