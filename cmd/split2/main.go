// Command split2 extracts individual uppercase character images from letters2.jpg.
// Uses manually-defined bounding boxes for a grid layout with a blue-grey background.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
)

const (
	srcPath = "images/sources/letters2.jpg"
	outDir  = "images/chars"

	bgR, bgG, bgB = 0x7E, 0x8C, 0xB5
	bgThreshold   = 42
)

func isBg(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	ri, gi, bi := int(r>>8), int(g>>8), int(b>>8)
	dr := abs(ri - bgR)
	dg := abs(gi - bgG)
	db := abs(bi - bgB)
	return dr < bgThreshold && dg < bgThreshold && db < bgThreshold
}

type charDef struct {
	ch                     rune
	xMin, yMin, xMax, yMax int
}

func main() {
	f, err := os.Open(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	src, err := jpeg.Decode(f)
	f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Image: %dx%d\n", src.Bounds().Dx(), src.Bounds().Dy())

	chars := []charDef{
		{'A', 80, 120, 250, 325},
		{'B', 250, 120, 405, 310},
		{'C', 408, 120, 565, 310},
		{'D', 568, 125, 725, 310},
		{'E', 728, 125, 868, 315},
		{'F', 870, 125, 1008, 300},
		{'G', 1010, 120, 1155, 300},
		{'H', 80, 325, 275, 505},
		{'I', 278, 325, 395, 505},
		{'J', 398, 325, 535, 505},
		{'K', 538, 320, 675, 510},
		{'L', 678, 320, 810, 500},
		{'M', 830, 320, 980, 510},
		{'N', 982, 320, 1140, 505},
		{'O', 80, 515, 255, 695},
		{'P', 258, 510, 415, 695},
		{'Q', 418, 510, 568, 695},
		{'R', 570, 515, 705, 688},
		{'S', 728, 515, 810, 690},
		{'T', 848, 510, 980, 695},
		{'U', 985, 510, 1140, 695},
		{'V', 230, 695, 385, 870},
		{'W', 390, 695, 540, 862},
		{'X', 560, 695, 718, 870},
		{'Y', 720, 695, 858, 870},
		{'Z', 860, 695, 1010, 870},
	}

	os.MkdirAll(outDir, 0755)

	// Determine next variant number for each character.
	existing := countExisting(outDir)

	for _, cd := range chars {
		ch := string(cd.ch)
		num := existing[ch] + 1

		bbox := tightBBox(src, cd.xMin, cd.xMax, cd.yMin, cd.yMax)
		if bbox.Dx() < 10 || bbox.Dy() < 10 {
			fmt.Printf("  %c: no content found, skipping\n", cd.ch)
			continue
		}
		if bbox.Min.X < cd.xMin {
			bbox.Min.X = cd.xMin
		}
		if bbox.Min.Y < cd.yMin {
			bbox.Min.Y = cd.yMin
		}
		if bbox.Max.X > cd.xMax {
			bbox.Max.X = cd.xMax
		}
		if bbox.Max.Y > cd.yMax {
			bbox.Max.Y = cd.yMax
		}

		img := extractChar(src, bbox)
		outPath := filepath.Join(outDir, fmt.Sprintf("%s.%d.png", ch, num))
		out, err := os.Create(outPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", outPath, err)
			continue
		}
		png.Encode(out, img)
		out.Close()
		existing[ch] = num
		fmt.Printf("  %c -> %s (%dx%d)\n", cd.ch, outPath, img.Bounds().Dx(), img.Bounds().Dy())
	}

	fmt.Println("Done.")
}

func countExisting(dir string) map[string]int {
	counts := make(map[string]int)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		name := e.Name()
		if len(name) < 5 {
			continue
		}
		lastDot := -1
		for k := 1; k < len(name); k++ {
			if name[k] == '.' {
				lastDot = k
				break
			}
		}
		if lastDot > 0 {
			ch := name[:lastDot]
			counts[ch]++
		}
	}
	return counts
}

func tightBBox(src image.Image, xStart, xEnd, yStart, yEnd int) image.Rectangle {
	minX, minY := xEnd, yEnd
	maxX, maxY := xStart, yStart
	for y := yStart; y < yEnd; y++ {
		for x := xStart; x < xEnd; x++ {
			if !isBg(src.At(x, y)) {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if maxX < minX || maxY < minY {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

func extractChar(src image.Image, bbox image.Rectangle) *image.RGBA {
	margin := 4
	r := image.Rect(max(0, bbox.Min.X-margin), max(0, bbox.Min.Y-margin), bbox.Max.X+margin, bbox.Max.Y+margin)
	w, h := r.Dx(), r.Dy()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := src.At(r.Min.X+x, r.Min.Y+y)
			cr, cg, cb, ca := c.RGBA()
			img.SetRGBA(x, y, color.RGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(ca >> 8)})
		}
	}
	floodFillEdges(img)
	return img
}

func floodFillEdges(img *image.RGBA) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	isTransparentOrBg := func(x, y int) bool {
		c := img.RGBAAt(x, y)
		if c.A < 10 {
			return true
		}
		dr := abs(int(c.R) - bgR)
		dg := abs(int(c.G) - bgG)
		db := abs(int(c.B) - bgB)
		return dr < bgThreshold+15 && dg < bgThreshold+15 && db < bgThreshold+15
	}
	visited := make([]bool, w*h)
	idx := func(x, y int) int { return y*w + x }
	var queue []image.Point
	for x := 0; x < w; x++ {
		for _, y := range []int{0, h - 1} {
			if isTransparentOrBg(x, y) && !visited[idx(x, y)] {
				visited[idx(x, y)] = true
				queue = append(queue, image.Pt(x, y))
			}
		}
	}
	for y := 0; y < h; y++ {
		for _, x := range []int{0, w - 1} {
			if isTransparentOrBg(x, y) && !visited[idx(x, y)] {
				visited[idx(x, y)] = true
				queue = append(queue, image.Pt(x, y))
			}
		}
	}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		img.SetRGBA(p.X, p.Y, color.RGBA{0, 0, 0, 0})
		for _, d := range []image.Point{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			nx, ny := p.X+d.X, p.Y+d.Y
			if nx < 0 || nx >= w || ny < 0 || ny >= h {
				continue
			}
			if visited[idx(nx, ny)] {
				continue
			}
			if isTransparentOrBg(nx, ny) {
				visited[idx(nx, ny)] = true
				queue = append(queue, image.Pt(nx, ny))
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
