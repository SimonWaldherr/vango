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

# pro adjustments
./vango-cli -in demo.jpg -out demo.adj.jpg -cmds "vibrance 0.8; dehaze 0.5; shadow_highlight 0.4 0.3"

# distortion effects
./vango-cli -in demo.jpg -out demo.twirl.jpg -cmds "twirl 1.2"
./vango-cli -in demo.jpg -out demo.wave.jpg -cmds "wave 10 8 60 50"
./vango-cli -in demo.jpg -out demo.spherize.jpg -cmds "spherize 0.7"
./vango-cli -in demo.jpg -out demo.ripple.jpg -cmds "ripple 12 45"
./vango-cli -in demo.jpg -out demo.pinch.jpg -cmds "pinch 0.6"
./vango-cli -in demo.jpg -out demo.polar.jpg -cmds "polar_coordinates to"

# dodge / burn
./vango-cli -in demo.jpg -out demo.dodge.jpg -cmds "dodge 0.4 midtones"
./vango-cli -in demo.jpg -out demo.burn.jpg -cmds "burn 0.4 shadows"

# tone curves (control points as input,output pairs in 0..1 range)
./vango-cli -in demo.jpg -out demo.curves.jpg -cmds "curves 0 0 0.5 0.65 1 1"

# gradient map (list of RRGGBB,position stops)
./vango-cli -in demo.jpg -out demo.gradmap.jpg -cmds "gradient_map 000000,0 FF6600,0.5 FFFFFF,1"

# per-channel curves  (r:in,out,in,out...  g:...  b:...)
./vango-cli -in demo.jpg -out demo.chan_curves.jpg -cmds "channel_curves r:0,0,0.5,0.7,1,1 g:0,0,1,1 b:0,0,0.5,0.3,1,1"

# tint overlay (hex color + opacity 0..1)
./vango-cli -in demo.jpg -out demo.tinted.jpg -cmds "tint FF8800 0.25"

# content-aware seam carving resize
./vango-cli -in demo.jpg -out demo.seamcarve.jpg -cmds "seam_carve 800 600"

# perspective transform (4 source corners then output size)
./vango-cli -in demo.jpg -out demo.persp.jpg -cmds "perspective 50 30 750 10 780 570 20 590 800 600"

# save as a layered vango project file (.vango) for later editing
./vango-cli -in demo.jpg -cmds "brightness 0.1; contrast 1.1" -project-out demo.vango

# load a .vango project file and continue editing
./vango-cli -in demo.vango -out demo_final.jpg -cmds "sepia 0.2"
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

## Layered canvas & project files

vango ships a full layer system (Photoshop-style). You can create multi-layer canvases,
composite them with blend modes, apply per-layer masks and effects, and **save / reload
the complete project** as a JSON `.vango` file.

```go
// Build a layered canvas
canvas := vango.NewCanvas(1200, 800)
bg := vango.NewSolidLayer("Background", 1200, 800, color.NRGBA{40, 40, 40, 255})
bg.ZOrder = 0
canvas.AddLayer(bg)

photo := vango.NewLayer("Photo", img)
photo.ZOrder = 1
photo.Blend = vango.BlendMultiply
photo.Opacity = 0.9
canvas.AddLayer(photo)

// Save as a layered project (images stored as base64-encoded PNGs inside JSON)
f, _ := os.Create("project.vango")
vango.SaveProject(canvas, f)
f.Close()

// Reload it later
f, _ = os.Open("project.vango")
restored, _ := vango.LoadProject(f)
f.Close()

// Flatten and export
flat := restored.FlattenAll()
out, _ := os.Create("output.png")
vango.EncodePNG(out, flat)
```

The `.vango` project format preserves:
- Canvas dimensions
- All layers (name, blend mode, opacity, visibility, offset, z-order, lock, clip, group)
- Pixel data (losslessly as embedded PNGs)
- Per-layer masks

### Layer features
- **Blend modes**: Normal, Multiply, Screen, Overlay, Soft Light, Hard Light, Color Dodge,
  Color Burn, Darken, Lighten, Difference, Exclusion, Hue, Saturation, Color, Luminosity,
  Dissolve, Linear Burn, Linear Dodge, Vivid Light, Linear Light, Pin Light
- **Non-destructive adjustment layers**: Brightness/Contrast, Hue/Saturation, Levels, Curves,
  Color Balance, Vibrance, Threshold, Posterize, Invert, Gradient Map, Solid Color
- **Layer styles**: Drop Shadow, Inner Shadow, Outer/Inner Glow, Stroke, Bevel & Emboss
- **Layer groups**: group layers and flatten as a unit
- **Layer masks**: pixel-level selection masks (feathering, grow, shrink, boolean ops)
- **Undo/redo**: full history with `History.SaveState` / `Undo` / `Redo`

## Supported effects (available via Pipeline and CLI)

### Core adjustments
- `blur <sigma>` — Gaussian blur
- `unsharp <amount> <sigma>` — unsharp mask
- `contrast <factor>`
- `saturation <factor>`
- `brightness <delta>`
- `hue <degrees>`
- `sepia <amount>`
- `invert`
- `gamma <gamma>`
- `grayscale`
- `solarize [cutoff]`
- `equalize` — histogram equalization
- `tonemap <exposure>` — Reinhard tone-mapping
- `levels <inBlack> <inWhite> <gamma> <outBlack> <outWhite>`
- `curves <in1> <out1> <in2> <out2> …` — tone curve (control points 0..1)
- `channel_curves r:in,out,… g:… b:…` — per-channel tone curves
- `colorbalance <sR> <sG> <sB> <mR> <mG> <mB> <hR> <hG> <hB>` — shadows/midtones/highlights
- `channelmix <rR> <rG> <rB> <gR> <gG> <gB> <bR> <bG> <bB>`
- `hsl_selective <targetHue> <range> <satFactor> <lightDelta>`
- `color_temperature <kelvin_shift>` — warm/cool shift
- `vibrance <amount>` — smart saturation boost
- `dehaze <strength>` — atmospheric dehazing
- `shadow_highlight <shadows> <highlights>` — recover shadows/highlights
- `tint <RRGGBB> <opacity>` — colour tint overlay
- `gradient_map <RRGGBB,pos> …` — gradient map effect

### Sharpening & detail
- `sharpen <amount>` — convolution sharpen
- `unsharp <amount> <sigma>` — unsharp mask
- `high_pass_sharpen <amount> <radius>` — high-pass sharpening
- `clarity <strength>` — local-contrast clarity

### Blur & focus
- `blur <sigma>`
- `motion_blur <angle> <distance>`
- `radial_blur <cx> <cy> <strength>`
- `tilt_shift <focusY> <bandWidth> <sigma>`
- `bilateral <sigmaSpatial> <sigmaRange>` — edge-preserving blur
- `noise_reduction [radius]` / `denoise`

### Geometry
- `rotate <degrees>`
- `skew <sx> <sy>`
- `resize <w> <h>` / `resize_nearest <w> <h>`
- `crop <x0> <y0> <x1> <y1>`
- `smartcrop <w> <h>`
- `trim`
- `flipx` / `flipy`
- `seam_carve <w> <h>` — content-aware resize
- `perspective <x0> <y0> <x1> <y1> <x2> <y2> <x3> <y3> <outW> <outH>`

### Distortions
- `twirl <angle> [radius]` — twirl/whirlpool
- `spherize <amount>` — spherical bulge (negative = pinch lens)
- `wave <ampX> <ampY> <wlX> <wlY>` — wave distortion
- `ripple <amplitude> <wavelength>` — ripple distortion
- `pinch <amount> [radius]` — pinch distortion
- `polar_coordinates [to|from]` — rectangular ↔ polar

### Artistic & creative
- `emboss <strength>`
- `vignette <strength>`
- `glow <sigma> <intensity>`
- `halftone <dotSize>`
- `oil_painting <radius> <levels>`
- `chromatic_aberration <shift>`
- `add_noise <amount>`
- `dither <levels>` — Floyd–Steinberg dithering
- `posterize <levels>`
- `threshold <cutoff>`

### Retouching
- `dodge <amount> [shadows|midtones|highlights]`
- `burn <amount> [shadows|midtones|highlights]`

### Auto modes
- `auto_contrast`
- `auto_brightness`
- `auto_vibrance`
- `auto_color` — white balance + equalize + auto brightness + auto vibrance
- `auto_full` — full auto enhance

### Utility
- `whitebalance [x0 y0 x1 y1]` / `wb`
- `collage <file> [horizontal|vertical]`
- `watermark <file> <x> <y> <opacity>`
- `edge` — Sobel edge detection
- `text <string> <x> <y>`
- `apply <plugin>` — registered plugin filter

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

## Selection tools
`Selection` provides Photoshop-style selection masks:

```go
sel := vango.NewSelection(img.Bounds())
sel.SelectEllipse(cx, cy, rx, ry)           // elliptical selection
sel.SelectByColor(img, x, y, tolerance, true) // magic wand
sel.SelectColorRange(img, r, g, b, fuzziness) // color range
sel.SelectByLuminosity(img, "highlights")     // luminosity mask
sel.Feather(5)                               // feather edges
sel.Grow(3)                                  // expand
sel.Shrink(3)                                // contract
// Apply effect only inside selection:
result := sel.ApplyToImage(original, processed)
```

## Examples in code
- See `example_test.go` for small example usages (these are run as examples by `go test`).
