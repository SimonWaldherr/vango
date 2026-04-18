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

# full auto enhance (whitebalance + noise reduction + auto contrast/brightness/vibrance)
./vango-cli -in demo.jpg -out demo.auto_full.jpg -cmds "auto_full"

# noise reduction (median filter, radius=1 gives 3×3 window, radius=2 gives 5×5, etc.)
./vango-cli -in demo.jpg -out demo.denoised.jpg -cmds "noise_reduction 1"

# align/collage two images side by side (horizontal or vertical)
./vango-cli -in demo.jpg -out demo.collage.jpg -cmds "collage other.jpg horizontal"
./vango-cli -in demo.jpg -out demo.collage_v.jpg -cmds "collage other.jpg vertical"

# edge detection + smart crop + watermark
./vango-cli -in demo.jpg -out demo.edge.jpg -cmds "edge; smartcrop 1200 800; watermark logo.png 20 20 0.5"
```

## Generate demo outputs
- The repository includes a test that will generate demo images for every supported effect. Run:

```bash
# this writes demo.<effect>.jpg files into the repo root
go test -run TestGenerateDemos -v
```

## Browser/WASM preparation
- A minimal browser-facing WASM entrypoint is available in `cmd/wasm`.
- Build it with:

```bash
GOOS=js GOARCH=wasm go build -o vango.wasm ./cmd/wasm
```

- Optional version injection:

```bash
GOOS=js GOARCH=wasm go build -ldflags "-X main.wasmVersion=v0.1.0" -o vango.wasm ./cmd/wasm
```

- In the browser, load `wasm_exec.js` + `vango.wasm`; the module exposes a global `vango` object with:
  - `wasm_exec.js` is available in your Go installation at `$(go env GOROOT)/lib/wasm/wasm_exec.js`.
  - `vango.ready` (boolean)
  - `vango.version` (string)
  - `vango.effects` (array of supported CLI effect names)

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
- smartcrop
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
- auto_color (whitebalance + equalize + auto brightness + auto vibrance)
- auto_brightness
- auto_vibrance
- **auto_full** (full auto enhance: whitebalance + noise reduction + equalize + auto brightness + auto vibrance)
- **noise_reduction** / **denoise** (median-filter noise reduction; takes optional radius, default 1)
- **collage** (align two images side by side or stacked; `collage <file> [horizontal|vertical]`)
- edge
- watermark
- apply (registered plugin filter)
- LUT3D (ApplyLUT3D via Pipeline.Apply / LUT helpers)

## Image alignment / collage
The `AlignImages` function (and the `Collage` Pipeline method) place two or more images into a
single canvas:

- **horizontal** (default): images are scaled to the same height and placed left-to-right.
- **vertical**: images are scaled to the same width and placed top-to-bottom.

```go
// Code example – place two images side by side
result := vango.AlignImages([]image.Image{img1, img2}, "horizontal", color.NRGBA{255, 255, 255, 255})

// Fluent pipeline variant
out := vango.From(img1).Collage(img2, "vertical", color.NRGBA{0, 0, 0, 255}).Image()
```

CLI usage:
```bash
./vango-cli -in left.jpg -out combined.jpg -cmds "collage right.jpg horizontal"
```

## Noise reduction
`NoiseReduction` applies a median filter. A larger radius removes more noise but softens edges:

```go
denoised := vango.NoiseReduction(img, 1) // 3×3 median filter
denoised2 := vango.NoiseReduction(img, 2) // 5×5 median filter
// or via the pipeline:
out := vango.From(img).NoiseReduction(1).Image()
```

## Examples in code
- See `example_test.go` for small example usages (these are run as examples by `go test`).
