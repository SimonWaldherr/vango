package vango

// EffectNames returns built-in effect names supported by the pipeline/CLI layer.
func EffectNames() []string {
	return []string{
		"blur", "unsharp", "contrast", "saturation", "brightness", "hue", "sepia", "invert",
		"gamma", "rotate", "skew", "resize", "resize_nearest", "crop", "smartcrop", "trim",
		"pixelate", "posterize", "threshold", "equalize", "tonemap", "dither", "text",
		"grayscale", "solarize", "emboss", "vignette", "whitebalance", "wb", "auto_contrast",
		"auto_color", "auto_brightness", "auto_vibrance", "edge", "watermark", "apply",
	}
}

