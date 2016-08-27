#!/usr/bin/env bash
set -euo pipefail

IN="demo.jpg"

echo "==> Blur + Contrast + Sepia"
go run cmd/main.go -in "$IN" -out demo.blur_contrast_sepia.jpg \
  -cmds "blur 1.2; contrast 1.1; sepia 0.2"

echo "==> Brightness + Saturation + Hue shift"
go run cmd/main.go -in "$IN" -out demo.bright_sat_hue.jpg \
  -cmds "brightness 0.1; saturation 1.2; hue 30"

echo "==> Unsharp mask + Gamma + Invert"
go run cmd/main.go -in "$IN" -out demo.unsharp_gamma_invert.jpg \
  -cmds "unsharp 0.6 1.0; gamma 2.2; invert"

echo "==> Resize + Posterize + Threshold"
go run cmd/main.go -in "$IN" -out demo.resize_posterize_thresh.png \
  -cmds "resize 320 240; posterize 5; threshold 128"

echo "==> Crop + Equalize + Dither"
go run cmd/main.go -in "$IN" -out demo.crop_equalize_dither.png \
  -cmds "crop 50 50 400 300; equalize; dither 4"

echo "==> Rotate + Pixelate + Text"
go run cmd/main.go -in "$IN" -out demo.rotate_pixel_text.png \
  -cmds "rotate 25; pixelate 12; text Hello 40 40"

echo "==> Tonemap + Sepia + Contrast"
go run cmd/main.go -in "$IN" -out demo.tonemap_sepia_contrast.jpg \
  -cmds "tonemap 1.5; sepia 0.3; contrast 1.2"

echo "==> Done. Results saved as demo.*.jpg/png"

