// Command split4 extracts individual character images from letters4.png.
// The image contains a grid of upper and lowercase letters on a white background.
// Uses connected-component labelling (same approach as split3).
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	srcPath     = "images/sources/letters4.png"
	outDir      = "images/chars"
	whiteThresh = 230
	minArea     = 80
)

// Characters in reading order (left to right, top to bottom).
// Row  0: a A B b C
// Row  1: c D d E e
// Row  2: F f G g H
// Row  3: h I i J j
// Row  4: K k L l M
// Row  5: m N n O o
// Row  6: P p Q q R
// Row  7: r S s T t
// Row  8: U u V v W
// Row  9: w X x Y y
// Row 10: Z z
var rowChars = [][]string{
	{"a", "A", "B", "b", "C"},
	{"c", "D", "d", "E", "e"},
	{"F", "f", "G", "g", "H"},
	{"h", "I", "i", "J", "j"},
	{"K", "k", "L", "l", "M"},
	{"m", "N", "n", "O", "o"},
	{"P", "p", "Q", "q", "R"},
	{"r", "S", "s", "T", "t"},
	{"U", "u", "V", "v", "W"},
	{"w", "X", "x", "Y", "y"},
	{"Z", "z"},
}

func isWhite(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return r>>8 >= whiteThresh && g>>8 >= whiteThresh && b>>8 >= whiteThresh
}

type region struct {
	minX, minY, maxX, maxY int
	pixelCount             int
}

func main() {
	f, err := os.Open(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	src, err := png.Decode(f)
	f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding: %v\n", err)
		os.Exit(1)
	}

	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	fmt.Printf("Image: %dx%d\n", w, h)

	// Connected component labelling.
	labels := make([]int, w*h)
	nextLabel := 1
	parent := []int{0}

	idx := func(x, y int) int { return y*w + x }
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			if ra < rb {
				parent[rb] = ra
			} else {
				parent[ra] = rb
			}
		}
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if isWhite(src.At(x+bounds.Min.X, y+bounds.Min.Y)) {
				continue
			}
			var neighbours []int
			if x > 0 && labels[idx(x-1, y)] != 0 {
				neighbours = append(neighbours, labels[idx(x-1, y)])
			}
			if y > 0 && labels[idx(x, y-1)] != 0 {
				neighbours = append(neighbours, labels[idx(x, y-1)])
			}
			if x > 0 && y > 0 && labels[idx(x-1, y-1)] != 0 {
				neighbours = append(neighbours, labels[idx(x-1, y-1)])
			}
			if x < w-1 && y > 0 && labels[idx(x+1, y-1)] != 0 {
				neighbours = append(neighbours, labels[idx(x+1, y-1)])
			}

			if len(neighbours) == 0 {
				labels[idx(x, y)] = nextLabel
				parent = append(parent, nextLabel)
				nextLabel++
			} else {
				minL := neighbours[0]
				for _, n := range neighbours[1:] {
					if find(n) < find(minL) {
						minL = n
					}
				}
				labels[idx(x, y)] = find(minL)
				for _, n := range neighbours {
					union(n, minL)
				}
			}
		}
	}

	// Resolve labels and compute bounding boxes.
	regions := make(map[int]*region)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			l := labels[idx(x, y)]
			if l == 0 {
				continue
			}
			root := find(l)
			labels[idx(x, y)] = root

			r, ok := regions[root]
			if !ok {
				r = &region{x, y, x, y, 0}
				regions[root] = r
			}
			r.pixelCount++
			if x < r.minX {
				r.minX = x
			}
			if x > r.maxX {
				r.maxX = x
			}
			if y < r.minY {
				r.minY = y
			}
			if y > r.maxY {
				r.maxY = y
			}
		}
	}

	// Filter and collect.
	var boxes []region
	for _, r := range regions {
		area := (r.maxX - r.minX + 1) * (r.maxY - r.minY + 1)
		if area >= minArea && r.pixelCount > 20 {
			boxes = append(boxes, *r)
		}
	}

	fmt.Printf("Found %d regions after filtering\n", len(boxes))

	// Sort into reading order: group by rows (Y midpoint), then X.
	sort.Slice(boxes, func(i, j int) bool {
		yi := (boxes[i].minY + boxes[i].maxY) / 2
		yj := (boxes[j].minY + boxes[j].maxY) / 2
		if abs(yi-yj) > 25 {
			return yi < yj
		}
		return boxes[i].minX < boxes[j].minX
	})

	// Group into rows by Y midpoint proximity.
	type row struct {
		items []region
	}
	var rows []row
	if len(boxes) > 0 {
		currentRow := row{items: []region{boxes[0]}}
		for i := 1; i < len(boxes); i++ {
			prevMidY := (currentRow.items[len(currentRow.items)-1].minY + currentRow.items[len(currentRow.items)-1].maxY) / 2
			curMidY := (boxes[i].minY + boxes[i].maxY) / 2
			if abs(curMidY-prevMidY) > 25 {
				rows = append(rows, currentRow)
				currentRow = row{items: []region{boxes[i]}}
			} else {
				currentRow.items = append(currentRow.items, boxes[i])
			}
		}
		rows = append(rows, currentRow)
	}

	fmt.Printf("Grouped into %d rows\n", len(rows))
	for i, r := range rows {
		if len(r.items) > 0 {
			midY := (r.items[0].minY + r.items[0].maxY) / 2
			fmt.Printf("  Row %2d: %2d items, y~%d\n", i, len(r.items), midY)
		}
	}

	// Track the highest existing variant index per uppercase character.
	existingMax := existingVariantMax(outDir)

	os.MkdirAll(outDir, 0755)

	fmt.Printf("\nExtracting characters...\n")
	totalExtracted := 0

	for rowIdx, charRow := range rowChars {
		if rowIdx >= len(rows) {
			fmt.Printf("Warning: not enough rows for row %d\n", rowIdx)
			break
		}
		items := rows[rowIdx].items
		if len(items) != len(charRow) {
			fmt.Printf("  Row %d: expected %d items, got %d — assigning in order\n", rowIdx, len(charRow), len(items))
		}

		for i, ch := range charRow {
			if i >= len(items) {
				fmt.Printf("  Warning: missing item for %q in row %d\n", ch, rowIdx)
				break
			}
			target := strings.ToUpper(ch)
			item := items[i]
			num := existingMax[target] + 1

			// Extract with margin.
			margin := 2
			cropMinX := max(0, item.minX-margin)
			cropMinY := max(0, item.minY-margin)
			cropMaxX := min(w-1, item.maxX+margin)
			cropMaxY := min(h-1, item.maxY+margin)
			cw := cropMaxX - cropMinX + 1
			cHeight := cropMaxY - cropMinY + 1

			img := image.NewRGBA(image.Rect(0, 0, cw, cHeight))
			for y := 0; y < cHeight; y++ {
				for x := 0; x < cw; x++ {
					sx := cropMinX + x + bounds.Min.X
					sy := cropMinY + y + bounds.Min.Y
					c := src.At(sx, sy)
					if isWhite(c) {
						img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0})
					} else {
						cr, cg, cb, ca := c.RGBA()
						img.SetRGBA(x, y, color.RGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(ca >> 8)})
					}
				}
			}

			floodFillEdges(img)

			outPath := filepath.Join(outDir, fmt.Sprintf("%s.%d.png", target, num))
			out, err := os.Create(outPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", outPath, err)
				continue
			}
			png.Encode(out, img)
			out.Close()
			existingMax[target] = num
			totalExtracted++
			fmt.Printf("  %s -> %s: variant %d\n", ch, target, num)
		}
	}

	fmt.Printf("\nExtracted %d character images total.\n", totalExtracted)
}

func floodFillEdges(img *image.RGBA) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	isBgLike := func(x, y int) bool {
		c := img.RGBAAt(x, y)
		if c.A < 10 {
			return true
		}
		return c.R >= whiteThresh && c.G >= whiteThresh && c.B >= whiteThresh
	}

	visited := make([]bool, w*h)
	idx := func(x, y int) int { return y*w + x }

	var queue []image.Point
	for x := 0; x < w; x++ {
		for _, y := range []int{0, h - 1} {
			if isBgLike(x, y) && !visited[idx(x, y)] {
				visited[idx(x, y)] = true
				queue = append(queue, image.Pt(x, y))
			}
		}
	}
	for y := 0; y < h; y++ {
		for _, x := range []int{0, w - 1} {
			if isBgLike(x, y) && !visited[idx(x, y)] {
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
			if isBgLike(nx, ny) {
				visited[idx(nx, ny)] = true
				queue = append(queue, image.Pt(nx, ny))
			}
		}
	}
}

func existingVariantMax(dir string) map[string]int {
	maxByChar := make(map[string]int)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return maxByChar
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".png") || strings.HasPrefix(name, "_") {
			continue
		}

		parts := strings.Split(name, ".")
		if len(parts) < 3 {
			continue
		}

		ch := parts[0]
		if len(ch) != 1 || ch[0] < 'A' || ch[0] > 'Z' {
			continue
		}

		n, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}

		if n > maxByChar[ch] {
			maxByChar[ch] = n
		}
	}

	return maxByChar
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
