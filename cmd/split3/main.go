// Command split3 extracts individual character images from a dense grid source image
// with a white background. Uses connected-component labelling to find each cut-out,
// then assigns characters based on row position.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
)

const (
	srcPath     = "images/sources/letters3.png"
	outDir      = "images/chars"
	whiteThresh = 230 // pixels with R,G,B all above this are "background"
	minArea     = 200 // minimum bounding box area to keep a region
)

// Each row in the image corresponds to a set of characters.
// We define the character(s) expected per row and let the splitter assign them.
var rowChars = []string{
	"A", // row 0: A variants + B variants
	"B",
	"C", // row 1: C + D
	"D",
	"E", // row 2: E + F
	"F",
	"G", // row 3: G + H
	"H",
	"I", // row 4: I + J
	"J",
	"K", // row 5: K + L
	"L",
	"M", // row 6: M + N
	"N",
	"O", // row 7: O + P
	"P",
	"Q", // row 8: Q + R
	"R",
	"S", // row 9: S + T
	"T",
	"U", // row 10: U + V
	"V",
	"W", // row 11: W + X
	"X",
	"Y", // row 12: Y + Z
	"Z",
	"1", // row 13: numbers
	"2",
	"3",
	"4",
	"5",
	"6", // row 14: numbers continued
	"7",
	"8",
	"9",
	"0",
	"!", // row 15: symbols
	"?",
	"&",
	"$",
	"+", // row 16: more symbols
	"#",
	":",
	"{",
	"-",
	",",
	"'",
	"(",
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
		if area >= minArea && r.pixelCount > 50 {
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

	// Define what characters are in each row.
	// Looking at the image layout:
	// Row  0: A (about 5) + B (about 8)       = ~13
	// Row  1: C (about 5) + D (about 8)       = ~13
	// Row  2: E (about 7) + F (about 6)       = ~13
	// Row  3: G (about 6) + H (about 7)       = ~13
	// Row  4: I (about 6) + J (about 7)       = ~13
	// Row  5: K (about 6) + L (about 7)       = ~13
	// Row  6: M (about 5) + N (about 8)       = ~13
	// Row  7: O (about 5) + P (about 8)       = ~13
	// Row  8: Q (about 4) + R (about 9)       = ~13
	// Row  9: S (about 6) + T (about 7)       = ~13
	// Row 10: U (about 5) + V (about 8)       = ~13
	// Row 11: W (about 6) + X (about 7)       = ~13
	// Row 12: Y (about 6) + Z (about 7)       = ~13
	// Row 13: 1,2,3,4,5                       = numbers row 1
	// Row 14: 6,7,8,9,0                       = numbers row 2
	// Row 15: !,?,&,$                          = symbols row 1
	// Row 16: +,#,:,{,-,comma,',( etc          = symbols row 2

	// For letters (rows 0-12), each row has two letters side by side.
	// We split roughly at the midpoint of the row's x-range,
	// but more accurately, we look for a gap in x positions.

	type charGroup struct {
		ch    string
		items []region
	}
	var groups []charGroup

	// Process letter rows (0-12): each row has 2 letters.
	letterPairs := [][2]string{
		{"A", "B"}, {"C", "D"}, {"E", "F"}, {"G", "H"},
		{"I", "J"}, {"K", "L"}, {"M", "N"}, {"O", "P"},
		{"Q", "R"}, {"S", "T"}, {"U", "V"}, {"W", "X"},
		{"Y", "Z"},
	}

	for i, pair := range letterPairs {
		if i >= len(rows) {
			fmt.Printf("Warning: not enough rows for letter pair %s/%s\n", pair[0], pair[1])
			break
		}
		items := rows[i].items

		// Find the best split point using minimum within-cluster variance on
		// item X-centroids (1D k-means with k=2). For each candidate split
		// index j the items are divided into a left cluster [0..j) and right
		// cluster [j..n). We pick the j that minimises the total within-cluster
		// variance of X-centroids, giving the tightest two groups. A minimum
		// group size of 2 prevents degenerate single-item clusters.
		bestSplit := len(items) / 2

		if len(items) > 2 {
			n := len(items)
			centers := make([]float64, n)
			for j := range items {
				centers[j] = float64(items[j].minX+items[j].maxX) / 2
			}

			// Prefix sums for O(1) variance per split.
			prefixSum := make([]float64, n+1)
			prefixSumSq := make([]float64, n+1)
			for j, c := range centers {
				prefixSum[j+1] = prefixSum[j] + c
				prefixSumSq[j+1] = prefixSumSq[j] + c*c
			}

			clusterVar := func(start, end int) float64 {
				cnt := float64(end - start)
				if cnt <= 1 {
					return 0
				}
				s := prefixSum[end] - prefixSum[start]
				sq := prefixSumSq[end] - prefixSumSq[start]
				return sq - s*s/cnt
			}

			bestVar := math.MaxFloat64
			for j := 2; j <= n-2; j++ {
				v := clusterVar(0, j) + clusterVar(j, n)
				if v < bestVar {
					bestVar = v
					bestSplit = j
				}
			}
		}

		groups = append(groups, charGroup{pair[0], items[:bestSplit]})
		groups = append(groups, charGroup{pair[1], items[bestSplit:]})
	}

	// Process number rows.
	numRows := [][5]string{
		{"1", "2", "3", "4", "5"},
		{"6", "7", "8", "9", "0"},
	}
	for i, nums := range numRows {
		rowIdx := 13 + i
		if rowIdx >= len(rows) {
			break
		}
		items := rows[rowIdx].items
		// Numbers come in groups of ~2-3 variants each, 5 numbers per row.
		// Split into 5 equal groups.
		perNum := len(items) / 5
		if perNum < 1 {
			perNum = 1
		}
		for j, n := range nums {
			start := j * perNum
			end := start + perNum
			if j == 4 {
				end = len(items) // last group gets remainder
			}
			if start >= len(items) {
				break
			}
			if end > len(items) {
				end = len(items)
			}
			groups = append(groups, charGroup{n, items[start:end]})
		}
	}

	// Symbol rows.
	symRow1 := []string{"!", "?", "&", "$"}
	symRow2 := []string{"+", "#", ":", "-", ",", "'"}

	for si, syms := range [][]string{symRow1, symRow2} {
		rowIdx := 15 + si
		if rowIdx >= len(rows) {
			break
		}
		items := rows[rowIdx].items
		perSym := len(items) / len(syms)
		if perSym < 1 {
			perSym = 1
		}
		for j, s := range syms {
			start := j * perSym
			end := start + perSym
			if j == len(syms)-1 {
				end = len(items)
			}
			if start >= len(items) {
				break
			}
			if end > len(items) {
				end = len(items)
			}
			groups = append(groups, charGroup{s, items[start:end]})
		}
	}

	// Now extract each character variant.
	// Count existing variants in outDir to determine starting number.
	existingCount := make(map[string]int)
	entries, _ := os.ReadDir(outDir)
	for _, e := range entries {
		name := e.Name()
		if len(name) > 2 {
			// Parse "X.N.png" -> char "X"
			dotIdx := -1
			for k := 1; k < len(name); k++ {
				if name[k] == '.' {
					dotIdx = k
					break
				}
			}
			if dotIdx > 0 {
				ch := name[:dotIdx]
				existingCount[ch]++
			}
		}
	}

	fmt.Printf("\nExtracting characters...\n")
	totalExtracted := 0

	for _, g := range groups {
		startNum := existingCount[g.ch] + 1
		for i, item := range g.items {
			num := startNum + i

			// Extract with margin.
			margin := 2
			cropMinX := max(0, item.minX-margin)
			cropMinY := max(0, item.minY-margin)
			cropMaxX := min(w-1, item.maxX+margin)
			cropMaxY := min(h-1, item.maxY+margin)
			cw := cropMaxX - cropMinX + 1
			ch := cropMaxY - cropMinY + 1

			img := image.NewRGBA(image.Rect(0, 0, cw, ch))
			for y := 0; y < ch; y++ {
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

			// Flood fill from edges.
			floodFillEdges(img)

			outPath := filepath.Join(outDir, fmt.Sprintf("%s.%d.png", g.ch, num))
			out, err := os.Create(outPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", outPath, err)
				continue
			}
			png.Encode(out, img)
			out.Close()
			totalExtracted++
		}
		existingCount[g.ch] = startNum + len(g.items) - 1
		fmt.Printf("  %s: %d variants (numbered %d..%d)\n", g.ch, len(g.items), startNum, existingCount[g.ch])
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

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
