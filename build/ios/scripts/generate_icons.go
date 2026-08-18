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

type IOSIconSize struct {
	Filename string
	Size     int
}

var iosIcons = []IOSIconSize{
	{"icon-20.png", 20},
	{"icon-20@2x.png", 40},
	{"icon-20@3x.png", 60},
	{"icon-29.png", 29},
	{"icon-29@2x.png", 58},
	{"icon-29@3x.png", 87},
	{"icon-40.png", 40},
	{"icon-40@2x.png", 80},
	{"icon-40@3x.png", 120},
	{"icon-60@2x.png", 120},
	{"icon-60@3x.png", 180},
	{"icon-76.png", 76},
	{"icon-76@2x.png", 152},
	{"icon-83.5@2x.png", 167},
	{"icon-1024.png", 1024},
}

const contentsJSON = `{
  "info" : {
    "author" : "xcode",
    "version" : 1
  },
  "images" : [
    {
      "filename" : "icon-20@2x.png",
      "idiom" : "iphone",
      "scale" : "2x",
      "size" : "20x20"
    },
    {
      "filename" : "icon-20@3x.png",
      "idiom" : "iphone",
      "scale" : "3x",
      "size" : "20x20"
    },
    {
      "filename" : "icon-29@2x.png",
      "idiom" : "iphone",
      "scale" : "2x",
      "size" : "29x29"
    },
    {
      "filename" : "icon-29@3x.png",
      "idiom" : "iphone",
      "scale" : "3x",
      "size" : "29x29"
    },
    {
      "filename" : "icon-40@2x.png",
      "idiom" : "iphone",
      "scale" : "2x",
      "size" : "40x40"
    },
    {
      "filename" : "icon-40@3x.png",
      "idiom" : "iphone",
      "scale" : "3x",
      "size" : "40x40"
    },
    {
      "filename" : "icon-60@2x.png",
      "idiom" : "iphone",
      "scale" : "2x",
      "size" : "60x60"
    },
    {
      "filename" : "icon-60@3x.png",
      "idiom" : "iphone",
      "scale" : "3x",
      "size" : "60x60"
    },
    {
      "filename" : "icon-20.png",
      "idiom" : "ipad",
      "scale" : "1x",
      "size" : "20x20"
    },
    {
      "filename" : "icon-20@2x.png",
      "idiom" : "ipad",
      "scale" : "2x",
      "size" : "20x20"
    },
    {
      "filename" : "icon-29.png",
      "idiom" : "ipad",
      "scale" : "1x",
      "size" : "29x29"
    },
    {
      "filename" : "icon-29@2x.png",
      "idiom" : "ipad",
      "scale" : "2x",
      "size" : "29x29"
    },
    {
      "filename" : "icon-40.png",
      "idiom" : "ipad",
      "scale" : "1x",
      "size" : "40x40"
    },
    {
      "filename" : "icon-40@2x.png",
      "idiom" : "ipad",
      "scale" : "2x",
      "size" : "40x40"
    },
    {
      "filename" : "icon-76.png",
      "idiom" : "ipad",
      "scale" : "1x",
      "size" : "76x76"
    },
    {
      "filename" : "icon-76@2x.png",
      "idiom" : "ipad",
      "scale" : "2x",
      "size" : "76x76"
    },
    {
      "filename" : "icon-83.5@2x.png",
      "idiom" : "ipad",
      "scale" : "2x",
      "size" : "83.5x83.5"
    },
    {
      "filename" : "icon-1024.png",
      "idiom" : "ios-marketing",
      "scale" : "1x",
      "size" : "1024x1024"
    }
  ]
}
`

func main() {
	inputPath := filepath.Join("build", "appicon.png")
	if _, err := os.Stat(inputPath); err != nil {
		fmt.Printf("Warning: %s not found, skipping iOS icon generation\n", inputPath)
		return
	}

	targetDirs := []string{
		filepath.Join("build", "ios", "Assets.xcassets", "AppIcon.appiconset"),
		filepath.Join("build", "ios", "xcode", "main", "Assets.xcassets", "AppIcon.appiconset"),
	}

	for _, dir := range targetDirs {
		_ = os.MkdirAll(dir, 0755)
		_ = os.WriteFile(filepath.Join(dir, "Contents.json"), []byte(contentsJSON), 0644)
	}

	useSips := hasCommand("sips")

	if useSips {
		for _, item := range iosIcons {
			sizeStr := fmt.Sprintf("%d", item.Size)
			for _, dir := range targetDirs {
				outPath := filepath.Join(dir, item.Filename)
				_ = exec.Command("sips", "-z", sizeStr, sizeStr, inputPath, "--out", outPath).Run()
			}
		}
		fmt.Println("✓ iOS app icons generated with native high quality (sips - CoreGraphics)")
		return
	}

	// Go fallback
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

	for _, item := range iosIcons {
		dst := image.NewRGBA(image.Rect(0, 0, item.Size, item.Size))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
		for _, dir := range targetDirs {
			savePNG(filepath.Join(dir, item.Filename), dst)
		}
	}
	fmt.Println("✓ iOS app icons generated with high quality (Catmull-Rom Bicubic)")
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
