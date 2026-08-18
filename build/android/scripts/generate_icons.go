package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/image/draw"
)

type IconSize struct {
	Folder         string
	Size           int
	ForegroundSize int
}

var sizes = []IconSize{
	{"mipmap-mdpi", 48, 108},
	{"mipmap-hdpi", 72, 162},
	{"mipmap-xhdpi", 96, 216},
	{"mipmap-xxhdpi", 144, 324},
	{"mipmap-xxxhdpi", 192, 432},
}

func main() {
	inputPath := filepath.Join("build", "appicon.png")
	if _, err := os.Stat(inputPath); err != nil {
		fmt.Printf("Warning: %s not found, skipping Android icon generation\n", inputPath)
		return
	}

	resDir := filepath.Join("build", "android", "app", "src", "main", "res")

	// Try macOS sips tool first for maximum Lanczos/CoreGraphics native quality
	if hasCommand("sips") {
		for _, s := range sizes {
			dir := filepath.Join(resDir, s.Folder)
			_ = os.MkdirAll(dir, 0755)

			launcher := filepath.Join(dir, "ic_launcher.png")
			launcherRound := filepath.Join(dir, "ic_launcher_round.png")
			launcherForeground := filepath.Join(dir, "ic_launcher_foreground.png")

			sizeStr := fmt.Sprintf("%d", s.Size)
			fgSizeStr := fmt.Sprintf("%d", s.ForegroundSize)

			_ = exec.Command("sips", "-z", sizeStr, sizeStr, inputPath, "--out", launcher).Run()
			_ = exec.Command("sips", "-z", sizeStr, sizeStr, inputPath, "--out", launcherRound).Run()
			_ = exec.Command("sips", "-z", fgSizeStr, fgSizeStr, inputPath, "--out", launcherForeground).Run()
		}
		fmt.Println("✓ Android app icons & adaptive foreground generated with native high quality (sips - CoreGraphics)")
		return
	}

	// Try ImageMagick if available
	if hasCommand("magick") || hasCommand("convert") {
		tool := "magick"
		if !hasCommand("magick") {
			tool = "convert"
		}
		for _, s := range sizes {
			dir := filepath.Join(resDir, s.Folder)
			_ = os.MkdirAll(dir, 0755)

			launcher := filepath.Join(dir, "ic_launcher.png")
			launcherRound := filepath.Join(dir, "ic_launcher_round.png")
			launcherForeground := filepath.Join(dir, "ic_launcher_foreground.png")

			sizeStr := fmt.Sprintf("%dx%d", s.Size, s.Size)
			fgSizeStr := fmt.Sprintf("%dx%d", s.ForegroundSize, s.ForegroundSize)

			_ = exec.Command(tool, inputPath, "-resize", sizeStr, launcher).Run()
			_ = exec.Command(tool, inputPath, "-resize", sizeStr, launcherRound).Run()
			_ = exec.Command(tool, inputPath, "-resize", fgSizeStr, launcherForeground).Run()
		}
		fmt.Println("✓ Android app icons & adaptive foreground generated with native high quality (ImageMagick)")
		return
	}

	// Go fallback using CatmullRom high quality bicubic interpolation
	file, err := os.Open(inputPath)
	if err != nil {
		fmt.Printf("Error opening %s: %v\n", inputPath, err)
		return
	}
	defer file.Close()

	src, err := png.Decode(file)
	if err != nil {
		fmt.Printf("Error decoding %s: %v\n", inputPath, err)
		return
	}

	for _, s := range sizes {
		dir := filepath.Join(resDir, s.Folder)
		_ = os.MkdirAll(dir, 0755)

		dst := image.NewRGBA(image.Rect(0, 0, s.Size, s.Size))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

		savePNG(filepath.Join(dir, "ic_launcher.png"), dst)
		savePNG(filepath.Join(dir, "ic_launcher_round.png"), dst)

		dstFg := image.NewRGBA(image.Rect(0, 0, s.ForegroundSize, s.ForegroundSize))
		draw.CatmullRom.Scale(dstFg, dstFg.Bounds(), src, src.Bounds(), draw.Over, nil)
		savePNG(filepath.Join(dir, "ic_launcher_foreground.png"), dstFg)
	}

	fmt.Println("✓ Android app icons & adaptive foreground generated with high quality (Catmull-Rom Bicubic)")
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func savePNG(path string, img image.Image) {
	out, err := os.Create(path)
	if err != nil {
		return
	}
	defer out.Close()
	png.Encode(out, img)
}
