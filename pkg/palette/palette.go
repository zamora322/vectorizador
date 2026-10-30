package palette

import (
	"image"
	"math"
	"math/rand"
	"sort"
)

// Palette represents a slice of RGB colors.
type Palette []RGB

// ExtractPalette extracts k dominant colors using K-Means++ clustering in Lab space.
func ExtractPalette(img image.Image, k int, maxIterations int) Palette {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if k <= 0 {
		k = 8
	}

	// 1. Sample pixels to keep performance predictable on large images
	sampleStep := 1
	totalPixels := width * height
	if totalPixels > 100000 {
		sampleStep = int(math.Ceil(math.Sqrt(float64(totalPixels) / 50000.0)))
	}

	type sample struct {
		rgb RGB
		lab Lab
	}

	samples := make([]sample, 0, 50000)
	for y := bounds.Min.Y; y < bounds.Max.Y; y += sampleStep {
		for x := bounds.Min.X; x < bounds.Max.X; x += sampleStep {
			r, g, b, a := img.At(x, y).RGBA()
			if a < 128*257 { // Ignore transparent pixels for palette extraction
				continue
			}

			// Anti-Fringe: Un-premultiply alpha so K-Means clusters true colors without dark halos
			var red, green, blue uint8
			if a > 0 && a < 65535 {
				red = uint8((r * 65535 / a) >> 8)
				green = uint8((g * 65535 / a) >> 8)
				blue = uint8((b * 65535 / a) >> 8)
			} else {
				red = uint8(r >> 8)
				green = uint8(g >> 8)
				blue = uint8(b >> 8)
			}

			rgb := RGB{R: red, G: green, B: blue}
			samples = append(samples, sample{rgb: rgb, lab: RGBToLab(rgb)})
		}
	}

	if len(samples) == 0 {
		return Palette{{R: 0, G: 0, B: 0}, {R: 255, G: 255, B: 255}}
	}

	if len(samples) < k {
		k = len(samples)
	}

	// 2. K-Means++ centroid initialization
	centroids := make([]Lab, k)
	rng := rand.New(rand.NewSource(42)) // Deterministic seed for reproducible vectorization
	centroids[0] = samples[rng.Intn(len(samples))].lab

	distSq := make([]float64, len(samples))
	for i := 1; i < k; i++ {
		var totalDistSq float64
		for j, s := range samples {
			d := DeltaE(s.lab, centroids[i-1])
			if i == 1 || d*d < distSq[j] {
				distSq[j] = d * d
			}
			totalDistSq += distSq[j]
		}

		target := rng.Float64() * totalDistSq
		var running float64
		selectedIdx := 0
		for j, d := range distSq {
			running += d
			if running >= target {
				selectedIdx = j
				break
			}
		}
		centroids[i] = samples[selectedIdx].lab
	}

	// 3. Iterative clustering
	if maxIterations <= 0 {
		maxIterations = 15
	}

	clusters := make([]int, len(samples))
	for iter := 0; iter < maxIterations; iter++ {
		changed := false

		// Assign each sample to nearest centroid
		for j, s := range samples {
			bestDist := math.MaxFloat64
			bestIdx := 0
			for ci, c := range centroids {
				d := DeltaE(s.lab, c)
				if d < bestDist {
					bestDist = d
					bestIdx = ci
				}
			}
			if clusters[j] != bestIdx {
				clusters[j] = bestIdx
				changed = true
			}
		}

		if !changed && iter > 2 {
			break
		}

		// Recompute centroids in Lab space
		sumL := make([]float64, k)
		sumA := make([]float64, k)
		sumB := make([]float64, k)
		counts := make([]int, k)

		for j, s := range samples {
			ci := clusters[j]
			sumL[ci] += s.lab.L
			sumA[ci] += s.lab.A
			sumB[ci] += s.lab.B
			counts[ci]++
		}

		for ci := 0; ci < k; ci++ {
			if counts[ci] > 0 {
				centroids[ci] = Lab{
					L: sumL[ci] / float64(counts[ci]),
					A: sumA[ci] / float64(counts[ci]),
					B: sumB[ci] / float64(counts[ci]),
				}
			}
		}
	}

	// 4. Convert centroids to RGB and sort by luminance descending
	result := make(Palette, k)
	for i, c := range centroids {
		result[i] = LabToRGB(c)
	}

	sort.Slice(result, func(i, j int) bool {
		labI := RGBToLab(result[i])
		labJ := RGBToLab(result[j])
		return labI.L > labJ.L
	})

	return result
}

// NearestIndex returns the index of the closest color in the palette to c.
func (p Palette) NearestIndex(c RGB) int {
	lab := RGBToLab(c)
	bestDist := math.MaxFloat64
	bestIdx := 0
	for i, palColor := range p {
		d := DeltaE(lab, RGBToLab(palColor))
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return bestIdx
}

// NearestColor returns the closest RGB in the palette.
func (p Palette) NearestColor(c RGB) RGB {
	if len(p) == 0 {
		return c
	}
	return p[p.NearestIndex(c)]
}

// DetectOptimalColorCount analyzes the image and detects the natural number of dominant colors.
func DetectOptimalColorCount(img image.Image) int {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	totalPixels := w * h
	if totalPixels == 0 {
		return 6
	}

	sampleStep := 1
	if totalPixels > 100000 {
		sampleStep = int(math.Ceil(math.Sqrt(float64(totalPixels) / 40000.0)))
	}

	bins := make(map[uint32]int)
	binColors := make(map[uint32]RGB)
	totalSampled := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y += sampleStep {
		for x := bounds.Min.X; x < bounds.Max.X; x += sampleStep {
			r, g, b, a := img.At(x, y).RGBA()
			if a < 128*257 {
				continue // Ignore transparent pixels
			}

			var red, green, blue uint8
			if a > 0 && a < 65535 {
				red = uint8((r * 65535 / a) >> 8)
				green = uint8((g * 65535 / a) >> 8)
				blue = uint8((b * 65535 / a) >> 8)
			} else {
				red = uint8(r >> 8)
				green = uint8(g >> 8)
				blue = uint8(b >> 8)
			}

			// 5-bit channel quantization (32 levels per channel)
			qKey := (uint32(red>>3) << 10) | (uint32(green>>3) << 5) | uint32(blue>>3)
			bins[qKey]++
			if _, ok := binColors[qKey]; !ok {
				binColors[qKey] = RGB{R: red, G: green, B: blue}
			}
			totalSampled++
		}
	}

	if totalSampled == 0 {
		return 2
	}

	type binEntry struct {
		key   uint32
		count int
	}
	sortedBins := make([]binEntry, 0, len(bins))
	for k, c := range bins {
		sortedBins = append(sortedBins, binEntry{key: k, count: c})
	}
	sort.Slice(sortedBins, func(i, j int) bool {
		return sortedBins[i].count > sortedBins[j].count
	})

	// Threshold: cluster must represent at least 0.4% of non-transparent image
	minCount := int(float64(totalSampled) * 0.004)
	if minCount < 4 {
		minCount = 4
	}

	var distinct []Lab
	for _, b := range sortedBins {
		if b.count < minCount {
			continue
		}
		rgb := binColors[b.key]
		lab := RGBToLab(rgb)

		isFar := true
		for _, d := range distinct {
			if DeltaE(lab, d) < 14.0 {
				isFar = false
				break
			}
		}
		if isFar {
			distinct = append(distinct, lab)
			if len(distinct) >= 16 {
				break
			}
		}
	}

	detected := len(distinct)
	if detected < 2 {
		detected = 2
	}
	return detected
}
