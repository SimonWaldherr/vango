package vango

// EffectNames returns built-in effect names supported by the pipeline/CLI layer.
func EffectNames() []string {
	return []string{
		// Core adjustments
		"blur", "unsharp", "contrast", "saturation", "brightness", "hue", "sepia", "invert",
		"gamma", "grayscale", "solarize",
		// Sharpening
		"sharpen", "high_pass_sharpen", "clarity",
		// Geometry
		"rotate", "skew", "resize", "resize_nearest", "crop", "smartcrop", "trim",
		"flip_x", "flip_y", "perspective",
		// Filters & effects
		"pixelate", "posterize", "threshold", "equalize", "tonemap", "dither",
		"emboss", "vignette", "noise_reduction", "denoise", "bilateral",
		// Color tools
		"levels", "curves", "channel_mix", "color_balance", "hsl_selective",
		"gradient_map", "color_temperature", "tint", "whitebalance", "wb",
		// Blur variants
		"motion_blur", "radial_blur", "tilt_shift",
		// Artistic
		"glow", "halftone", "oil_painting", "chromatic_aberration", "add_noise",
		// Distortion
		"twirl", "spherize", "wave", "ripple", "polar_coordinates", "pinch",
		// Retouching
		"dodge", "burn", "clone_stamp", "healing_brush", "red_eye_removal",
		// Pro adjustments
		"vibrance", "dehaze", "shadow_highlight", "frequency_separation",
		"channel_curves", "blend_if",
		// Seam carving
		"seam_carve",
		// Auto
		"auto_contrast", "auto_color", "auto_brightness", "auto_vibrance", "auto_full",
		// Composition
		"edge", "watermark", "collage", "text", "apply",
		// ImageMagick-inspired
		"normalize", "auto_level",
		"charcoal", "sketch",
		"sigmoidal_contrast",
		"extent",
		"roll", "spread",
		"transpose", "transverse",
		"shave",
		"ordered_dither",
		"selective_blur",
		"auto_threshold",
		"adaptive_blur", "adaptive_sharpen",
		"morphology",
		"statistic",
		"mean_shift",
		"kuwahara",
	}
}

// BlendModeNames returns the names of all supported blend modes.
func BlendModeNames() []string {
	return []string{
		"normal", "multiply", "screen", "overlay", "soft_light", "hard_light",
		"color_dodge", "color_burn", "darken", "lighten", "difference", "exclusion",
		"hue", "saturation", "color", "luminosity", "dissolve",
		"linear_burn", "linear_dodge", "vivid_light", "linear_light", "pin_light",
	}
}
