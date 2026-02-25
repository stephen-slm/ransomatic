package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
)

// ManifestEntry represents a single character entry from the manifest.
type ManifestEntry struct {
	File     string `json:"file"`
	Char     string `json:"char"`
	Category string `json:"category"`
}

// loadManifest reads the _manifest.json and returns a map of character to image path.
func loadManifest(baseDir string) (map[rune]string, error) {
	data, err := os.ReadFile(filepath.Join(baseDir, "_manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var entries []ManifestEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	lookup := make(map[rune]string, len(entries))
	for _, e := range entries {
		if len([]rune(e.Char)) == 1 {
			r := []rune(e.Char)[0]
			lookup[r] = filepath.Join(baseDir, e.File)
		}
	}
	return lookup, nil
}

// loadImage reads a PNG file and returns it as an image.Image.
func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

// scaleImage resizes an image so its height equals targetH, preserving aspect ratio.
func scaleImage(src image.Image, targetH int) *image.RGBA {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcH == 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	scale := float64(targetH) / float64(srcH)
	newW := int(math.Round(float64(srcW) * scale))
	newH := targetH

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	return dst
}

// rotateImage rotates an RGBA image by the given angle (in radians) around its center.
// Returns the rotated image and the offset needed to position it correctly.
func rotateImage(src *image.RGBA, angleDeg float64) *image.RGBA {
	if angleDeg == 0 {
		return src
	}

	angleRad := angleDeg * math.Pi / 180.0
	srcB := src.Bounds()
	srcW := float64(srcB.Dx())
	srcH := float64(srcB.Dy())

	// Calculate the bounding box of the rotated image.
	cosA := math.Abs(math.Cos(angleRad))
	sinA := math.Abs(math.Sin(angleRad))
	newW := int(math.Ceil(srcW*cosA + srcH*sinA))
	newH := int(math.Ceil(srcW*sinA + srcH*cosA))

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))

	// Build the affine transform: translate to center of dst, rotate, translate back from center of src.
	cx := srcW / 2
	cy := srcH / 2
	ncx := float64(newW) / 2
	ncy := float64(newH) / 2

	cosR := math.Cos(angleRad)
	sinR := math.Sin(angleRad)

	// Forward transform: src -> dst
	// dst = translate(ncx,ncy) * rotate(angle) * translate(-cx,-cy) * src
	//
	// The x/image Transformer wants the *inverse* (dst->src):
	// src = translate(cx,cy) * rotate(-angle) * translate(-ncx,-ncy) * dst
	m := f64.Aff3{
		cosR, sinR, cx - ncx*cosR - ncy*sinR,
		-sinR, cosR, cy + ncx*sinR - ncy*cosR,
	}

	xdraw.BiLinear.Transform(dst, m, src, srcB, xdraw.Over, nil)
	return dst
}

func main() {
	text := flag.String("text", "", "The text to render as an image")
	output := flag.String("output", "output.png", "Output file path")
	wordsPerLine := flag.Int("words-per-line", 5, "Maximum number of words per line")
	charHeight := flag.Int("char-height", 64, "Height of each character in pixels")
	charGap := flag.Int("char-gap", 2, "Gap in pixels between characters within a word")
	wordGap := flag.Int("word-gap", 30, "Gap in pixels between words")
	lineGap := flag.Int("line-gap", 20, "Gap in pixels between lines")
	maxTilt := flag.Float64("max-tilt", 12.0, "Maximum random tilt in degrees (each direction)")
	bgHex := flag.String("bg", "FFFFFF", "Background color as hex (e.g. FFFFFF). Use 'none' for transparent")
	padding := flag.Int("padding", 20, "Padding around the entire image in pixels")
	noClipboard := flag.Bool("no-clipboard", false, "Disable copying the image to the clipboard")

	flag.Parse()

	if *text == "" {
		// Also accept positional argument.
		if flag.NArg() > 0 {
			joined := strings.Join(flag.Args(), " ")
			text = &joined
		} else {
			fmt.Fprintf(os.Stderr, "Usage: randy -text \"Hello World\"\n")
			flag.PrintDefaults()
			os.Exit(1)
		}
	}

	charsDir := filepath.Join("images", "chars")
	lookup, err := loadManifest(charsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading manifest: %v\n", err)
		os.Exit(1)
	}

	// Split text into words and group into lines.
	words := strings.Fields(*text)
	if len(words) == 0 {
		fmt.Fprintf(os.Stderr, "Error: empty text\n")
		os.Exit(1)
	}

	var lines [][]string
	for i := 0; i < len(words); i += *wordsPerLine {
		end := i + *wordsPerLine
		if end > len(words) {
			end = len(words)
		}
		lines = append(lines, words[i:end])
	}

	// Pre-render each line: for each line, produce a slice of word images.
	// Each word image is composed of scaled + rotated character images.
	type renderedLine struct {
		wordImages []*image.RGBA
		height     int // max height across all word images in this line
	}

	var rendered []renderedLine

	for _, lineWords := range lines {
		var rl renderedLine

		for _, word := range lineWords {
			// Render each character in the word.
			var charImages []*image.RGBA
			for _, ch := range word {
				imgPath, ok := lookup[ch]
				if !ok {
					// Skip unknown characters.
					continue
				}
				srcImg, err := loadImage(imgPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not load image for '%c': %v\n", ch, err)
					continue
				}

				// Scale to uniform height.
				scaled := scaleImage(srcImg, *charHeight)

				// Apply random tilt.
				tilt := (rand.Float64()*2 - 1) * *maxTilt
				rotated := rotateImage(scaled, tilt)

				charImages = append(charImages, rotated)
			}

			if len(charImages) == 0 {
				continue
			}

			// Compose character images horizontally into a single word image.
			totalW := 0
			maxH := 0
			for i, ci := range charImages {
				totalW += ci.Bounds().Dx()
				if i > 0 {
					totalW += *charGap
				}
				if ci.Bounds().Dy() > maxH {
					maxH = ci.Bounds().Dy()
				}
			}

			wordImg := image.NewRGBA(image.Rect(0, 0, totalW, maxH))
			x := 0
			for i, ci := range charImages {
				if i > 0 {
					x += *charGap
				}
				// Center each character vertically within the word's height.
				yOff := (maxH - ci.Bounds().Dy()) / 2
				draw.Draw(wordImg, image.Rect(x, yOff, x+ci.Bounds().Dx(), yOff+ci.Bounds().Dy()),
					ci, ci.Bounds().Min, draw.Over)
				x += ci.Bounds().Dx()
			}

			rl.wordImages = append(rl.wordImages, wordImg)
			if maxH > rl.height {
				rl.height = maxH
			}
		}

		rendered = append(rendered, rl)
	}

	// Calculate total canvas size.
	totalHeight := 0
	totalWidth := 0
	for i, rl := range rendered {
		if i > 0 {
			totalHeight += *lineGap
		}
		totalHeight += rl.height

		lineWidth := 0
		for j, wi := range rl.wordImages {
			if j > 0 {
				lineWidth += *wordGap
			}
			lineWidth += wi.Bounds().Dx()
		}
		if lineWidth > totalWidth {
			totalWidth = lineWidth
		}
	}

	totalWidth += *padding * 2
	totalHeight += *padding * 2

	// Create the final canvas.
	canvas := image.NewRGBA(image.Rect(0, 0, totalWidth, totalHeight))

	// Fill background.
	if *bgHex != "none" {
		bgColor := parseHexColor(*bgHex)
		draw.Draw(canvas, canvas.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
	}

	// Draw each line.
	curY := *padding
	for i, rl := range rendered {
		if i > 0 {
			curY += *lineGap
		}
		curX := *padding
		for j, wi := range rl.wordImages {
			if j > 0 {
				curX += *wordGap
			}
			// Center word vertically within the line height.
			yOff := (rl.height - wi.Bounds().Dy()) / 2
			draw.Draw(canvas, image.Rect(curX, curY+yOff, curX+wi.Bounds().Dx(), curY+yOff+wi.Bounds().Dy()),
				wi, wi.Bounds().Min, draw.Over)
			curX += wi.Bounds().Dx()
		}
		curY += rl.height
	}

	// Write output.
	outFile, err := os.Create(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	if err := png.Encode(outFile, canvas); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding PNG: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %s (%dx%d)\n", *output, totalWidth, totalHeight)

	// Copy to clipboard by default.
	if !*noClipboard {
		if err := copyImageToClipboard(*output); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not copy to clipboard: %v\n", err)
		} else {
			fmt.Println("Copied to clipboard")
		}
	}
}

// copyImageToClipboard copies a PNG file to the system clipboard.
func copyImageToClipboard(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`set the clipboard to (read (POSIX file %q) as «class PNGf»)`, absPath)
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		// Requires xclip to be installed.
		cmd := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-i", absPath)
		return cmd.Run()
	case "windows":
		// PowerShell approach.
		ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Clipboard]::SetImage([System.Drawing.Image]::FromFile('%s'))`, absPath)
		return exec.Command("powershell", "-command", ps).Run()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// parseHexColor parses a hex color string (e.g. "FFFFFF" or "FF0000") into a color.RGBA.
func parseHexColor(hex string) color.RGBA {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		fmt.Fprintf(os.Stderr, "Warning: invalid hex color %q, using white\n", hex)
		return color.RGBA{255, 255, 255, 255}
	}
	var r, g, b uint8
	fmt.Sscanf(hex, "%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 255}
}
