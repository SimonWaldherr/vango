# vango
## a collection of various image manipulation effects named after Vincent van Gogh

A compact, dependency-free image processing toolkit implemented as one Go source file (`vango.go`).
It provides many common image operations, a fluent Pipeline API (with fusion of per-pixel ops),
context-aware parallel execution, and a small CLI in `cmd/main.go`.
It supports JPEG, PNG, GIF, BMP, and TIFF formats via the Go standard library.

## Quick start
1. Run the unit/examples and build locally:

```bash
# run tests (example tests + unit tests)
go test ./...

# build the CLI tool
go build -o vango-cli ./cmd
```

2. Use the CLI to process an image (zsh example):

```bash
# basic CLI usage (use ; or , as separators)
./vango-cli -in demo.jpg -out demo.out.jpg -cmds "blur 1.2; contrast 1.1; sepia 0.2"

# additional effects
./vango-cli -in demo.jpg -out demo.gray_vig.jpg -cmds "grayscale, vignette 0.6, skew 0.08 0.0, resize_nearest 1200 800"

# auto modes (auto_color already includes whitebalance + auto contrast/brightness/vibrance)
./vango-cli -in demo.jpg -out demo.auto.jpg -cmds "auto_color"
```

## Generate demo outputs
- The repository includes a test that will generate demo images for every supported effect. Run:

```bash
# this writes demo.<effect>.jpg files into the repo root
go test -run TestGenerateDemos -v
```

## Supported effects (available via Pipeline and CLI)
- blur (GaussianBlur)
- unsharp (UnsharpMask)
- contrast
- saturation
- brightness
- hue
- sepia
- invert
- gamma
- rotate
- skew
- resize
- resize_nearest
- crop
- trim
- pixelate
- posterize
- threshold
- equalize
- tonemap
- dither
- text (DrawText)
- grayscale
- solarize
- emboss
- vignette
- whitebalance
- wb (alias for whitebalance)
- auto_contrast
- auto_color
- auto_brightness
- auto_vibrance
- apply (registered plugin filter)
- LUT3D (ApplyLUT3D via Pipeline.Apply / LUT helpers)

## Examples in code
- See `example_test.go` for small example usages (these are run as examples by `go test`).
