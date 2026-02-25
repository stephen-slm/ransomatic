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

	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/math/f64"
)

// charRenderer produces an RGBA image for a single character at the given height.
type charRenderer interface {
	renderChar(ch rune, height int) (*image.RGBA, error)
}

// --- Ransom-note (image-based) renderer ---

type ransomRenderer struct {
	lookup map[rune]string
}

func newRansomRenderer(charsDir string) (*ransomRenderer, error) {
	data, err := os.ReadFile(filepath.Join(charsDir, "_manifest.json"))
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var entries []struct {
		File string `json:"file"`
		Char string `json:"char"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	lookup := make(map[rune]string, len(entries))
	for _, e := range entries {
		runes := []rune(e.Char)
		if len(runes) == 1 {
			lookup[runes[0]] = filepath.Join(charsDir, e.File)
		}
	}
	return &ransomRenderer{lookup: lookup}, nil
}

func (r *ransomRenderer) renderChar(ch rune, height int) (*image.RGBA, error) {
	imgPath, ok := r.lookup[ch]
	if !ok {
		return nil, fmt.Errorf("no image for character %q", ch)
	}
	f, err := os.Open(imgPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	return scaleImage(src, height), nil
}

// --- Font-based renderer ---

type fontRenderer struct {
	face    font.Face
	ttFont  *truetype.Font
	fgColor color.Color
}

// defaultFontPaths returns known paths for Comic Sans on various OSes.
func defaultFontPaths() []string {
	return []string{
		// macOS
		"/System/Library/Fonts/Supplemental/Comic Sans MS.ttf",
		"/Library/Fonts/Comic Sans MS.ttf",
		// Linux common locations
		"/usr/share/fonts/truetype/msttcorefonts/Comic_Sans_MS.ttf",
		"/usr/share/fonts/truetype/comic-sans/ComicSans.ttf",
		// Windows
		`C:\Windows\Fonts\comic.ttf`,
	}
}

func newFontRenderer(fontPath string, fontSize float64, fg color.Color) (*fontRenderer, error) {
	if fontPath == "" {
		// Try to find Comic Sans automatically.
		for _, p := range defaultFontPaths() {
			if _, err := os.Stat(p); err == nil {
				fontPath = p
				break
			}
		}
		if fontPath == "" {
			return nil, fmt.Errorf("could not find Comic Sans font; use -font to specify path")
		}
	}

	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, fmt.Errorf("reading font file: %w", err)
	}

	ttFont, err := truetype.Parse(fontData)
	if err != nil {
		return nil, fmt.Errorf("parsing font: %w", err)
	}

	face := truetype.NewFace(ttFont, &truetype.Options{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})

	return &fontRenderer{face: face, ttFont: ttFont, fgColor: fg}, nil
}

func (fr *fontRenderer) renderChar(ch rune, height int) (*image.RGBA, error) {
	// Create a face at the target size. We use the height as the font size
	// and adjust if needed.
	fontSize := float64(height) * 0.85
	face := truetype.NewFace(fr.ttFont, &truetype.Options{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	defer face.Close()

	// Measure the character.
	adv, ok := face.GlyphAdvance(ch)
	if !ok {
		return nil, fmt.Errorf("no glyph for character %q", ch)
	}

	charW := adv.Ceil()
	if charW < 1 {
		charW = height / 2
	}

	// Add a little padding so descenders/ascenders aren't clipped.
	imgW := charW + 4
	imgH := height

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	ctx := freetype.NewContext()
	ctx.SetDPI(72)
	ctx.SetFont(fr.ttFont)
	ctx.SetFontSize(fontSize)
	ctx.SetClip(img.Bounds())
	ctx.SetDst(img)
	ctx.SetSrc(image.NewUniform(fr.fgColor))

	// Baseline: place text so it's vertically centered.
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	textH := ascent + descent
	yOffset := (imgH-textH)/2 + ascent

	pt := freetype.Pt(2, yOffset)
	if _, err := ctx.DrawString(string(ch), pt); err != nil {
		return nil, fmt.Errorf("drawing character: %w", err)
	}

	return img, nil
}

// --- Shared image utilities ---

func scaleImage(src image.Image, targetH int) *image.RGBA {
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcH == 0 {
		return image.NewRGBA(image.Rect(0, 0, 0, 0))
	}
	scale := float64(targetH) / float64(srcH)
	newW := int(math.Round(float64(srcW) * scale))

	dst := image.NewRGBA(image.Rect(0, 0, newW, targetH))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	return dst
}

func rotateImage(src *image.RGBA, angleDeg float64) *image.RGBA {
	if angleDeg == 0 {
		return src
	}

	angleRad := angleDeg * math.Pi / 180.0
	srcB := src.Bounds()
	srcW := float64(srcB.Dx())
	srcH := float64(srcB.Dy())

	cosA := math.Abs(math.Cos(angleRad))
	sinA := math.Abs(math.Sin(angleRad))
	newW := int(math.Ceil(srcW*cosA + srcH*sinA))
	newH := int(math.Ceil(srcW*sinA + srcH*cosA))

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))

	cx := srcW / 2
	cy := srcH / 2
	ncx := float64(newW) / 2
	ncy := float64(newH) / 2

	cosR := math.Cos(angleRad)
	sinR := math.Sin(angleRad)

	m := f64.Aff3{
		cosR, sinR, cx - ncx*cosR - ncy*sinR,
		-sinR, cosR, cy + ncx*sinR - ncy*cosR,
	}

	xdraw.BiLinear.Transform(dst, m, src, srcB, xdraw.Over, nil)
	return dst
}

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
		cmd := exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-i", absPath)
		return cmd.Run()
	case "windows":
		ps := fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.Clipboard]::SetImage([System.Drawing.Image]::FromFile('%s'))`, absPath)
		return exec.Command("powershell", "-command", ps).Run()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// --- Main ---

func main() {
	text := flag.String("text", "", "The text to render as an image")
	output := flag.String("output", "output.png", "Output file path")
	wordsPerLine := flag.Int("words-per-line", 5, "Maximum number of words per line")
	charHeight := flag.Int("char-height", 64, "Height of each character in pixels")
	charGap := flag.Int("char-gap", -8, "Gap in pixels between characters (negative = overlap)")
	wordGap := flag.Int("word-gap", 30, "Gap in pixels between words")
	lineGap := flag.Int("line-gap", 20, "Gap in pixels between lines")
	maxTilt := flag.Float64("max-tilt", 0, "Maximum random tilt in degrees (each direction)")
	bgHex := flag.String("bg", "FFFFFF", "Background color as hex (e.g. FFFFFF). Use 'none' for transparent")
	padding := flag.Int("padding", 20, "Padding around the entire image in pixels")
	noClipboard := flag.Bool("no-clipboard", false, "Disable copying the image to the clipboard")
	align := flag.String("align", "center", "Text alignment: left, center, or right")
	mode := flag.String("mode", "ransom", "Rendering mode: ransom (cut-out images) or comic-sans (font)")
	fontPath := flag.String("font", "", "Path to TTF font file (only used with -mode comic-sans)")
	fontColor := flag.String("font-color", "000000", "Font color as hex (only used with -mode comic-sans)")

	flag.Parse()

	// Validate flags.
	switch *align {
	case "left", "center", "right":
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid alignment %q (use left, center, or right)\n", *align)
		os.Exit(1)
	}

	switch *mode {
	case "ransom", "comic-sans":
	default:
		fmt.Fprintf(os.Stderr, "Error: invalid mode %q (use ransom or comic-sans)\n", *mode)
		os.Exit(1)
	}

	if *text == "" {
		if flag.NArg() > 0 {
			joined := strings.Join(flag.Args(), " ")
			text = &joined
		} else {
			fmt.Fprintf(os.Stderr, "Usage: ransomatic -text \"Hello World\"\n")
			flag.PrintDefaults()
			os.Exit(1)
		}
	}

	// Create the appropriate renderer.
	var renderer charRenderer
	switch *mode {
	case "ransom":
		charsDir := filepath.Join("images", "chars")
		r, err := newRansomRenderer(charsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading character images: %v\n", err)
			os.Exit(1)
		}
		renderer = r
	case "comic-sans":
		fg := parseHexColor(*fontColor)
		r, err := newFontRenderer(*fontPath, float64(*charHeight), fg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading font: %v\n", err)
			os.Exit(1)
		}
		renderer = r
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

	// Pre-render each line as a slice of word images.
	type renderedLine struct {
		wordImages []*image.RGBA
		height     int
	}

	var rendered []renderedLine

	for _, lineWords := range lines {
		var rl renderedLine

		for _, word := range lineWords {
			var charImages []*image.RGBA
			for _, ch := range word {
				charImg, err := renderer.renderChar(ch, *charHeight)
				if err != nil {
					continue
				}

				tilt := (rand.Float64()*2 - 1) * *maxTilt
				rotated := rotateImage(charImg, tilt)
				charImages = append(charImages, rotated)
			}

			if len(charImages) == 0 {
				continue
			}

			// Compose characters horizontally into a word image.
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
			if totalW < 1 {
				totalW = 1
			}

			wordImg := image.NewRGBA(image.Rect(0, 0, totalW, maxH))
			x := 0
			for i, ci := range charImages {
				if i > 0 {
					x += *charGap
				}
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

	// Calculate per-line widths and total canvas size.
	lineWidths := make([]int, len(rendered))
	totalHeight := 0
	totalWidth := 0
	for i, rl := range rendered {
		if i > 0 {
			totalHeight += *lineGap
		}
		totalHeight += rl.height

		w := 0
		for j, wi := range rl.wordImages {
			if j > 0 {
				w += *wordGap
			}
			w += wi.Bounds().Dx()
		}
		lineWidths[i] = w
		if w > totalWidth {
			totalWidth = w
		}
	}

	totalWidth += *padding * 2
	totalHeight += *padding * 2

	canvas := image.NewRGBA(image.Rect(0, 0, totalWidth, totalHeight))

	if *bgHex != "none" {
		bgColor := parseHexColor(*bgHex)
		draw.Draw(canvas, canvas.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)
	}

	contentWidth := totalWidth - *padding*2

	curY := *padding
	for i, rl := range rendered {
		if i > 0 {
			curY += *lineGap
		}

		var xOffset int
		switch *align {
		case "center":
			xOffset = (contentWidth - lineWidths[i]) / 2
		case "right":
			xOffset = contentWidth - lineWidths[i]
		default:
			xOffset = 0
		}

		curX := *padding + xOffset
		for j, wi := range rl.wordImages {
			if j > 0 {
				curX += *wordGap
			}
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

	if !*noClipboard {
		if err := copyImageToClipboard(*output); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not copy to clipboard: %v\n", err)
		} else {
			fmt.Println("Copied to clipboard")
		}
	}
}
