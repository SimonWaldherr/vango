//go:build js && wasm

package main

import (
	"syscall/js"
)

func registerVangoAPI() {
	api := js.Global().Get("Object").New()
	api.Set("ready", js.ValueOf(true))
	api.Set("version", js.ValueOf("dev"))
	api.Set("effects", js.ValueOf([]any{
		"blur", "unsharp", "contrast", "saturation", "brightness", "hue", "sepia", "invert",
		"gamma", "rotate", "skew", "resize", "resize_nearest", "crop", "smartcrop", "trim",
		"pixelate", "posterize", "threshold", "equalize", "tonemap", "dither", "text",
		"grayscale", "solarize", "emboss", "vignette", "whitebalance", "wb", "auto_contrast",
		"auto_color", "auto_brightness", "auto_vibrance", "edge", "watermark", "apply",
	}))
	js.Global().Set("vango", api)
}

func main() {
	registerVangoAPI()
	select {}
}

