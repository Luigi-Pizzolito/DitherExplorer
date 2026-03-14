package dither

import (
	"image"
	"image/color"
	"math"
	"sort"

	"github.com/nfnt/resize"
)

type Algorithm string

const (
	AlgorithmThreshold          Algorithm = "Threshold"
	AlgorithmFloyd              Algorithm = "Floyd-Steinberg"
	AlgorithmBayer              Algorithm = "Bayer Ordered"
	AlgorithmAtkinson           Algorithm = "Atkinson"
	AlgorithmJarvisJudiceNinke  Algorithm = "Jarvis-Judice-Ninke"
	AlgorithmSierra             Algorithm = "Sierra"
	AlgorithmBlueNoiseThreshold Algorithm = "Blue-Noise Threshold"
	AlgorithmBlueNoiseHybrid    Algorithm = "Blue-Noise Hybrid"
)

type ResampleAlgorithm string

const (
	ResampleNearest  ResampleAlgorithm = "Nearest"
	ResampleBilinear ResampleAlgorithm = "Bilinear"
	ResampleBicubic  ResampleAlgorithm = "Bicubic"
	ResampleLanczos  ResampleAlgorithm = "Lanczos"
)

type SharpenAlgorithm string

const (
	SharpenNone      SharpenAlgorithm = "Off"
	SharpenUnsharp   SharpenAlgorithm = "Unsharp"
	SharpenLineBoost SharpenAlgorithm = "Line Boost"
	SharpenAnimeEdge SharpenAlgorithm = "Anime Edge"
)

type PaletteAlgorithm string

const (
	PaletteUniform PaletteAlgorithm = "Uniform"
	PalettePopular PaletteAlgorithm = "Popular"
	PaletteMedian  PaletteAlgorithm = "Median Cut"
	PaletteKMeans  PaletteAlgorithm = "K-Means"
)

type PaletteMode string

const (
	PaletteDynamic PaletteMode = "Dynamic"
	PaletteStatic  PaletteMode = "Static"
)

type Params struct {
	Threshold        float64
	Diffusion        float64
	Levels           int
	Scale            float64
	Resample         ResampleAlgorithm
	SharpenAlgorithm SharpenAlgorithm
	SharpenStrength  float64
	UseColor         bool
	PaletteAlgorithm PaletteAlgorithm
	PaletteMode      PaletteMode
	PaletteUpdateInt int
	OrderedStrength  float64
	BlueNoiseAmount  float64
}

type ProcessState struct {
	StaticPalette []color.RGBA
	PaletteKey    string
	LastPalette   []color.RGBA

	DynamicPalette    []color.RGBA
	DynamicPaletteKey string
	DynamicPaletteAge int
}

func Process(source image.Image, algorithm Algorithm, params Params, state *ProcessState) (stage1, stage2, stage3 *image.RGBA) {
	prepared := preprocess(source, params)
	if prepared == nil {
		empty := image.NewRGBA(image.Rect(0, 0, 1, 1))
		return empty, empty, empty
	}

	levels := normalizeLevels(params.Levels)

	if params.UseColor {
		palette := paletteForFrame(prepared, levels, params, state)
		if state != nil {
			state.LastPalette = clonePalette(palette)
		}
		stage1 = copyToRGBA(prepared)
		stage2 = quantizeImage(prepared, palette)
		switch algorithm {
		case AlgorithmFloyd:
			stage3 = diffuseColorKernel(prepared, palette, params.Diffusion, floydSteinbergKernel, 0)
		case AlgorithmAtkinson:
			stage3 = diffuseColorKernel(prepared, palette, params.Diffusion, atkinsonKernel, 0)
		case AlgorithmJarvisJudiceNinke:
			stage3 = diffuseColorKernel(prepared, palette, params.Diffusion, jjnKernel, 0)
		case AlgorithmSierra:
			stage3 = diffuseColorKernel(prepared, palette, params.Diffusion, sierraKernel, 0)
		case AlgorithmBayer:
			stage3 = orderedDitherColor(prepared, palette, params.OrderedStrength)
		case AlgorithmBlueNoiseThreshold:
			stage3 = blueNoiseThresholdColor(prepared, palette, params.BlueNoiseAmount)
		case AlgorithmBlueNoiseHybrid:
			stage3 = diffuseColorKernel(prepared, palette, params.Diffusion, floydSteinbergKernel, params.BlueNoiseAmount)
		default:
			stage3 = quantizeImage(prepared, palette)
		}
		return stage1, stage2, stage3
	}

	gray, width, height := toGray(prepared)
	stage1 = grayscalePreview(gray, width, height)

	switch algorithm {
	case AlgorithmFloyd:
		intermediate, result := diffuseGrayKernel(gray, width, height, params.Threshold, levels, params.Diffusion, floydSteinbergKernel, 0)
		stage2 = grayscalePreview(intermediate, width, height)
		stage3 = grayscalePreview(result, width, height)
	case AlgorithmAtkinson:
		intermediate, result := diffuseGrayKernel(gray, width, height, params.Threshold, levels, params.Diffusion, atkinsonKernel, 0)
		stage2 = grayscalePreview(intermediate, width, height)
		stage3 = grayscalePreview(result, width, height)
	case AlgorithmJarvisJudiceNinke:
		intermediate, result := diffuseGrayKernel(gray, width, height, params.Threshold, levels, params.Diffusion, jjnKernel, 0)
		stage2 = grayscalePreview(intermediate, width, height)
		stage3 = grayscalePreview(result, width, height)
	case AlgorithmSierra:
		intermediate, result := diffuseGrayKernel(gray, width, height, params.Threshold, levels, params.Diffusion, sierraKernel, 0)
		stage2 = grayscalePreview(intermediate, width, height)
		stage3 = grayscalePreview(result, width, height)
	case AlgorithmBayer:
		intermediate, result := orderedGrayDither(gray, width, height, params.Threshold, levels, params.OrderedStrength)
		stage2 = grayscalePreview(intermediate, width, height)
		stage3 = grayscalePreview(result, width, height)
	case AlgorithmBlueNoiseThreshold:
		intermediate, result := blueNoiseThresholdGray(gray, width, height, params.Threshold, levels, params.BlueNoiseAmount)
		stage2 = grayscalePreview(intermediate, width, height)
		stage3 = grayscalePreview(result, width, height)
	case AlgorithmBlueNoiseHybrid:
		intermediate, result := diffuseGrayKernel(gray, width, height, params.Threshold, levels, params.Diffusion, floydSteinbergKernel, params.BlueNoiseAmount)
		stage2 = grayscalePreview(intermediate, width, height)
		stage3 = grayscalePreview(result, width, height)
	default:
		intermediate, result := threshold(gray, width, height, params.Threshold, levels)
		stage2 = grayscalePreview(intermediate, width, height)
		stage3 = grayscalePreview(result, width, height)
	}

	if state != nil && params.PaletteMode == PaletteDynamic {
		state.StaticPalette = nil
		state.PaletteKey = ""
	}
	if state != nil {
		state.LastPalette = nil
	}

	return stage1, stage2, stage3
}

func preprocess(source image.Image, params Params) image.Image {
	if source == nil {
		return nil
	}
	prepared := source
	scale := params.Scale
	if scale > 0 && scale < 1 {
		bounds := source.Bounds()
		width := int(math.Round(float64(bounds.Dx()) * scale))
		height := int(math.Round(float64(bounds.Dy()) * scale))
		if width < 1 {
			width = 1
		}
		if height < 1 {
			height = 1
		}

		prepared = resize.Resize(uint(width), uint(height), source, resampleInterpolation(params.Resample))
	}
	return applySharpen(prepared, params.SharpenAlgorithm, params.SharpenStrength)
}

func applySharpen(source image.Image, algorithm SharpenAlgorithm, strength float64) image.Image {
	if source == nil {
		return nil
	}
	strength = clamp01(strength)
	if algorithm == SharpenNone || strength <= 0.0001 {
		return source
	}

	rgba := copyToRGBA(source)
	blurred := boxBlur3x3RGBA(rgba)

	switch algorithm {
	case SharpenUnsharp:
		return sharpenUnsharp(rgba, blurred, strength)
	case SharpenLineBoost:
		return sharpenLineBoost(rgba, blurred, strength)
	case SharpenAnimeEdge:
		return sharpenAnimeEdge(rgba, blurred, strength)
	default:
		return rgba
	}
}

func sharpenUnsharp(source, blurred *image.RGBA, strength float64) *image.RGBA {
	bounds := source.Bounds()
	output := image.NewRGBA(bounds)
	amount := 1.0 + strength*1.6
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			s := source.RGBAAt(x, y)
			b := blurred.RGBAAt(x, y)
			r := clampByte(float64(s.R) + amount*(float64(s.R)-float64(b.R)))
			g := clampByte(float64(s.G) + amount*(float64(s.G)-float64(b.G)))
			bl := clampByte(float64(s.B) + amount*(float64(s.B)-float64(b.B)))
			output.SetRGBA(x, y, color.RGBA{R: r, G: g, B: bl, A: 255})
		}
	}
	return output
}

func sharpenLineBoost(source, blurred *image.RGBA, strength float64) *image.RGBA {
	bounds := source.Bounds()
	output := image.NewRGBA(bounds)
	amount := 0.65 + strength*0.95
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			s := source.RGBAAt(x, y)
			b := blurred.RGBAAt(x, y)
			edge := edgeMagnitudeLuma(source, x, y)
			darken := strength * edge * 0.55
			r := (float64(s.R) + amount*(float64(s.R)-float64(b.R))) * (1.0 - darken)
			g := (float64(s.G) + amount*(float64(s.G)-float64(b.G))) * (1.0 - darken)
			bl := (float64(s.B) + amount*(float64(s.B)-float64(b.B))) * (1.0 - darken)
			output.SetRGBA(x, y, color.RGBA{R: clampByte(r), G: clampByte(g), B: clampByte(bl), A: 255})
		}
	}
	return output
}

func sharpenAnimeEdge(source, blurred *image.RGBA, strength float64) *image.RGBA {
	bounds := source.Bounds()
	output := image.NewRGBA(bounds)
	amount := 0.9 + strength*1.4
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			s := source.RGBAAt(x, y)
			b := blurred.RGBAAt(x, y)
			edge := edgeMagnitudeLuma(source, x, y)
			lineMask := 0.0
			if edge > 0.12 {
				lineMask = (edge - 0.12) / 0.88
			}
			darken := strength * clamp01(lineMask) * 0.72
			r := (float64(s.R) + amount*(float64(s.R)-float64(b.R))) * (1.0 - darken)
			g := (float64(s.G) + amount*(float64(s.G)-float64(b.G))) * (1.0 - darken)
			bl := (float64(s.B) + amount*(float64(s.B)-float64(b.B))) * (1.0 - darken)
			output.SetRGBA(x, y, color.RGBA{R: clampByte(r), G: clampByte(g), B: clampByte(bl), A: 255})
		}
	}
	return output
}

func boxBlur3x3RGBA(source *image.RGBA) *image.RGBA {
	bounds := source.Bounds()
	output := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var rSum, gSum, bSum float64
			count := 0.0
			for offsetY := -1; offsetY <= 1; offsetY++ {
				for offsetX := -1; offsetX <= 1; offsetX++ {
					cx := clampInt(x+offsetX, bounds.Min.X, bounds.Max.X-1)
					cy := clampInt(y+offsetY, bounds.Min.Y, bounds.Max.Y-1)
					pixel := source.RGBAAt(cx, cy)
					rSum += float64(pixel.R)
					gSum += float64(pixel.G)
					bSum += float64(pixel.B)
					count += 1
				}
			}
			output.SetRGBA(x, y, color.RGBA{
				R: clampByte(rSum / count),
				G: clampByte(gSum / count),
				B: clampByte(bSum / count),
				A: 255,
			})
		}
	}
	return output
}

func edgeMagnitudeLuma(source *image.RGBA, x, y int) float64 {
	bounds := source.Bounds()
	luma := func(px, py int) float64 {
		cx := clampInt(px, bounds.Min.X, bounds.Max.X-1)
		cy := clampInt(py, bounds.Min.Y, bounds.Max.Y-1)
		pixel := source.RGBAAt(cx, cy)
		return (0.299*float64(pixel.R) + 0.587*float64(pixel.G) + 0.114*float64(pixel.B)) / 255.0
	}

	gx := -1*luma(x-1, y-1) + 1*luma(x+1, y-1) +
		-2*luma(x-1, y) + 2*luma(x+1, y) +
		-1*luma(x-1, y+1) + 1*luma(x+1, y+1)

	gy := -1*luma(x-1, y-1) - 2*luma(x, y-1) - 1*luma(x+1, y-1) +
		1*luma(x-1, y+1) + 2*luma(x, y+1) + 1*luma(x+1, y+1)

	return clamp01(math.Sqrt(gx*gx+gy*gy) / 4.0)
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampByte(value float64) uint8 {
	if value < 0 {
		value = 0
	}
	if value > 255 {
		value = 255
	}
	return uint8(math.Round(value))
}

func paletteForFrame(source image.Image, levels int, params Params, state *ProcessState) []color.RGBA {
	key := paletteConfigKey(levels, params)
	if params.PaletteMode == PaletteStatic && state != nil && state.PaletteKey == key && len(state.StaticPalette) > 0 {
		return state.StaticPalette
	}
	interval := normalizePaletteUpdateInt(params.PaletteUpdateInt)
	if params.PaletteMode == PaletteDynamic && state != nil && interval > 1 {
		if state.DynamicPaletteKey == key && len(state.DynamicPalette) > 0 && state.DynamicPaletteAge < interval-1 {
			state.DynamicPaletteAge++
			return state.DynamicPalette
		}
	}

	var palette []color.RGBA
	switch params.PaletteAlgorithm {
	case PaletteKMeans:
		palette = kMeansPalette(source, levels)
	case PaletteMedian:
		palette = medianCutPalette(source, levels)
	case PalettePopular:
		palette = popularPalette(source, levels)
	default:
		palette = uniformPalette(levels)
	}

	if params.PaletteMode == PaletteStatic && state != nil {
		state.StaticPalette = palette
		state.PaletteKey = key
	}
	if params.PaletteMode == PaletteDynamic && state != nil {
		state.DynamicPalette = clonePalette(palette)
		state.DynamicPaletteKey = key
		state.DynamicPaletteAge = 0
	}
	return palette
}

func paletteConfigKey(levels int, params Params) string {
	return string(params.PaletteAlgorithm) + "|" + string(params.PaletteMode) + "|" + strconvItoa(levels) + "|" + strconvItoa(normalizePaletteUpdateInt(params.PaletteUpdateInt))
}

func normalizePaletteUpdateInt(value int) int {
	if value <= 1 {
		return 1
	}
	if value >= 32 {
		return 32
	}
	options := []int{1, 2, 4, 8, 16, 32}
	best := options[0]
	bestDelta := absInt(value - best)
	for _, option := range options[1:] {
		delta := absInt(value - option)
		if delta < bestDelta {
			best = option
			bestDelta = delta
		}
	}
	return best
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func uniformPalette(levels int) []color.RGBA {
	levels = normalizeLevels(levels)
	if levels <= 2 {
		return []color.RGBA{{R: 0, G: 0, B: 0, A: 255}, {R: 255, G: 255, B: 255, A: 255}}
	}

	grid := int(math.Ceil(math.Cbrt(float64(levels))))
	if grid < 2 {
		grid = 2
	}
	palette := make([]color.RGBA, 0, grid*grid*grid)
	for r := 0; r < grid; r++ {
		for g := 0; g < grid; g++ {
			for b := 0; b < grid; b++ {
				palette = append(palette, color.RGBA{
					R: uint8(math.Round(float64(r) * 255.0 / float64(grid-1))),
					G: uint8(math.Round(float64(g) * 255.0 / float64(grid-1))),
					B: uint8(math.Round(float64(b) * 255.0 / float64(grid-1))),
					A: 255,
				})
			}
		}
	}
	if len(palette) <= levels {
		return palette
	}

	sampled := make([]color.RGBA, 0, levels)
	if levels == 1 {
		return []color.RGBA{palette[len(palette)/2]}
	}
	maxIndex := len(palette) - 1
	for index := 0; index < levels; index++ {
		position := int(math.Round(float64(index) * float64(maxIndex) / float64(levels-1)))
		if position < 0 {
			position = 0
		}
		if position > maxIndex {
			position = maxIndex
		}
		sampled = append(sampled, palette[position])
	}
	return sampled
}

type histogramEntry struct {
	key   int
	count int
}

type sampledPixel struct {
	r float64
	g float64
	b float64
}

func samplePixels(source image.Image, maxSamples int) []sampledPixel {
	bounds := source.Bounds()
	area := bounds.Dx() * bounds.Dy()
	if area <= 0 {
		return nil
	}
	if maxSamples < 1 {
		maxSamples = 1
	}

	step := int(math.Sqrt(float64(area) / float64(maxSamples)))
	if step < 1 {
		step = 1
	}

	samples := make([]sampledPixel, 0, maxSamples)
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r, g, b, _ := source.At(x, y).RGBA()
			samples = append(samples, sampledPixel{
				r: float64(r>>8) / 255.0,
				g: float64(g>>8) / 255.0,
				b: float64(b>>8) / 255.0,
			})
		}
	}
	return samples
}

func medianCutPalette(source image.Image, levels int) []color.RGBA {
	levels = normalizeLevels(levels)
	samples := samplePixels(source, 4096)
	if len(samples) == 0 {
		return uniformPalette(levels)
	}

	type colorBox struct {
		pixels []sampledPixel
	}

	boxes := []colorBox{{pixels: samples}}

	for len(boxes) < levels {
		splitIndex := -1
		bestRange := -1.0
		bestChannel := 0

		for index, box := range boxes {
			if len(box.pixels) < 2 {
				continue
			}
			rMin, rMax, gMin, gMax, bMin, bMax := boxBounds(box.pixels)
			rRange := rMax - rMin
			gRange := gMax - gMin
			bRange := bMax - bMin

			channel := 0
			rangeValue := rRange
			if gRange > rangeValue {
				channel = 1
				rangeValue = gRange
			}
			if bRange > rangeValue {
				channel = 2
				rangeValue = bRange
			}

			if rangeValue > bestRange {
				bestRange = rangeValue
				bestChannel = channel
				splitIndex = index
			}
		}

		if splitIndex == -1 {
			break
		}

		box := boxes[splitIndex]
		sort.Slice(box.pixels, func(i, j int) bool {
			switch bestChannel {
			case 1:
				return box.pixels[i].g < box.pixels[j].g
			case 2:
				return box.pixels[i].b < box.pixels[j].b
			default:
				return box.pixels[i].r < box.pixels[j].r
			}
		})

		mid := len(box.pixels) / 2
		if mid <= 0 || mid >= len(box.pixels) {
			break
		}

		left := colorBox{pixels: append([]sampledPixel(nil), box.pixels[:mid]...)}
		right := colorBox{pixels: append([]sampledPixel(nil), box.pixels[mid:]...)}

		boxes[splitIndex] = left
		boxes = append(boxes, right)
	}

	palette := make([]color.RGBA, 0, len(boxes))
	for _, box := range boxes {
		if len(box.pixels) == 0 {
			continue
		}
		var rSum, gSum, bSum float64
		for _, pixel := range box.pixels {
			rSum += pixel.r
			gSum += pixel.g
			bSum += pixel.b
		}
		scale := 1.0 / float64(len(box.pixels))
		palette = append(palette, color.RGBA{
			R: uint8(clamp01(rSum*scale) * 255),
			G: uint8(clamp01(gSum*scale) * 255),
			B: uint8(clamp01(bSum*scale) * 255),
			A: 255,
		})
	}

	if len(palette) == 0 {
		return uniformPalette(levels)
	}
	if len(palette) > levels {
		palette = palette[:levels]
	}
	if len(palette) < levels {
		fallback := uniformPalette(levels)
		for len(palette) < levels {
			palette = append(palette, fallback[len(palette)%len(fallback)])
		}
	}
	return palette
}

func boxBounds(pixels []sampledPixel) (rMin, rMax, gMin, gMax, bMin, bMax float64) {
	rMin, gMin, bMin = 1.0, 1.0, 1.0
	rMax, gMax, bMax = 0.0, 0.0, 0.0
	for _, pixel := range pixels {
		if pixel.r < rMin {
			rMin = pixel.r
		}
		if pixel.r > rMax {
			rMax = pixel.r
		}
		if pixel.g < gMin {
			gMin = pixel.g
		}
		if pixel.g > gMax {
			gMax = pixel.g
		}
		if pixel.b < bMin {
			bMin = pixel.b
		}
		if pixel.b > bMax {
			bMax = pixel.b
		}
	}
	return rMin, rMax, gMin, gMax, bMin, bMax
}

func kMeansPalette(source image.Image, levels int) []color.RGBA {
	levels = normalizeLevels(levels)
	samples := samplePixels(source, 2048)
	if len(samples) == 0 {
		return uniformPalette(levels)
	}

	k := levels
	if len(samples) < k {
		k = len(samples)
	}
	if k < 1 {
		k = 1
	}

	seed := uniformPalette(k)
	centroids := make([]sampledPixel, k)
	for index := 0; index < k; index++ {
		centroids[index] = sampledPixel{
			r: float64(seed[index].R) / 255.0,
			g: float64(seed[index].G) / 255.0,
			b: float64(seed[index].B) / 255.0,
		}
	}

	for iteration := 0; iteration < 6; iteration++ {
		rSums := make([]float64, k)
		gSums := make([]float64, k)
		bSums := make([]float64, k)
		counts := make([]int, k)

		for _, sample := range samples {
			bestIndex := 0
			bestDistance := math.MaxFloat64
			for index, centroid := range centroids {
				dr := sample.r - centroid.r
				dg := sample.g - centroid.g
				db := sample.b - centroid.b
				distance := dr*dr + dg*dg + db*db
				if distance < bestDistance {
					bestDistance = distance
					bestIndex = index
				}
			}

			rSums[bestIndex] += sample.r
			gSums[bestIndex] += sample.g
			bSums[bestIndex] += sample.b
			counts[bestIndex]++
		}

		for index := 0; index < k; index++ {
			if counts[index] == 0 {
				reseed := samples[index%len(samples)]
				centroids[index] = reseed
				continue
			}
			inv := 1.0 / float64(counts[index])
			centroids[index] = sampledPixel{
				r: rSums[index] * inv,
				g: gSums[index] * inv,
				b: bSums[index] * inv,
			}
		}
	}

	palette := make([]color.RGBA, 0, levels)
	for _, centroid := range centroids {
		palette = append(palette, color.RGBA{
			R: uint8(clamp01(centroid.r) * 255),
			G: uint8(clamp01(centroid.g) * 255),
			B: uint8(clamp01(centroid.b) * 255),
			A: 255,
		})
	}

	if len(palette) < levels {
		fallback := uniformPalette(levels)
		for len(palette) < levels {
			palette = append(palette, fallback[len(palette)%len(fallback)])
		}
	}
	if len(palette) > levels {
		palette = palette[:levels]
	}
	return palette
}

func popularPalette(source image.Image, levels int) []color.RGBA {
	levels = normalizeLevels(levels)
	bounds := source.Bounds()
	counts := make(map[int]int, bounds.Dx()*bounds.Dy()/8+1)

	step := 2
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r, g, b, _ := source.At(x, y).RGBA()
			r4 := int((r >> 8) >> 4)
			g4 := int((g >> 8) >> 4)
			b4 := int((b >> 8) >> 4)
			key := (r4 << 8) | (g4 << 4) | b4
			counts[key]++
		}
	}

	entries := make([]histogramEntry, 0, len(counts))
	for key, count := range counts {
		entries = append(entries, histogramEntry{key: key, count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})

	if len(entries) == 0 {
		return uniformPalette(levels)
	}
	if len(entries) > levels {
		entries = entries[:levels]
	}

	palette := make([]color.RGBA, 0, len(entries))
	for _, entry := range entries {
		r4 := (entry.key >> 8) & 0xF
		g4 := (entry.key >> 4) & 0xF
		b4 := entry.key & 0xF
		palette = append(palette, color.RGBA{
			R: uint8(r4*17 + 8),
			G: uint8(g4*17 + 8),
			B: uint8(b4*17 + 8),
			A: 255,
		})
	}
	return palette
}

func quantizeImage(source image.Image, palette []color.RGBA) *image.RGBA {
	bounds := source.Bounds()
	output := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := source.At(x, y).RGBA()
			closest := nearestPaletteColor(
				float64(r>>8)/255.0,
				float64(g>>8)/255.0,
				float64(b>>8)/255.0,
				palette,
			)
			output.SetRGBA(x-bounds.Min.X, y-bounds.Min.Y, closest)
		}
	}
	return output
}

func floydSteinbergColor(source image.Image, palette []color.RGBA, diffusion float64) *image.RGBA {
	if diffusion < 0 {
		diffusion = 0
	}
	if diffusion > 1 {
		diffusion = 1
	}

	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	size := width * height
	rChan := make([]float64, size)
	gChan := make([]float64, size)
	bChan := make([]float64, size)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			index := y*width + x
			rChan[index] = float64(r>>8) / 255.0
			gChan[index] = float64(g>>8) / 255.0
			bChan[index] = float64(b>>8) / 255.0
		}
	}

	output := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			oldR := clamp01(rChan[index])
			oldG := clamp01(gChan[index])
			oldB := clamp01(bChan[index])
			newColor := nearestPaletteColor(oldR, oldG, oldB, palette)
			output.SetRGBA(x, y, newColor)

			newR := float64(newColor.R) / 255.0
			newG := float64(newColor.G) / 255.0
			newB := float64(newColor.B) / 255.0
			errR := (oldR - newR) * diffusion
			errG := (oldG - newG) * diffusion
			errB := (oldB - newB) * diffusion

			diffuseColorError(rChan, gChan, bChan, width, height, x, y, errR, errG, errB)
		}
	}

	return output
}

type diffusionTap struct {
	dx     int
	dy     int
	weight float64
}

var floydSteinbergKernel = []diffusionTap{
	{dx: 1, dy: 0, weight: 7.0 / 16.0},
	{dx: -1, dy: 1, weight: 3.0 / 16.0},
	{dx: 0, dy: 1, weight: 5.0 / 16.0},
	{dx: 1, dy: 1, weight: 1.0 / 16.0},
}

var atkinsonKernel = []diffusionTap{
	{dx: 1, dy: 0, weight: 1.0 / 8.0},
	{dx: 2, dy: 0, weight: 1.0 / 8.0},
	{dx: -1, dy: 1, weight: 1.0 / 8.0},
	{dx: 0, dy: 1, weight: 1.0 / 8.0},
	{dx: 1, dy: 1, weight: 1.0 / 8.0},
	{dx: 0, dy: 2, weight: 1.0 / 8.0},
}

var jjnKernel = []diffusionTap{
	{dx: 1, dy: 0, weight: 7.0 / 48.0},
	{dx: 2, dy: 0, weight: 5.0 / 48.0},
	{dx: -2, dy: 1, weight: 3.0 / 48.0},
	{dx: -1, dy: 1, weight: 5.0 / 48.0},
	{dx: 0, dy: 1, weight: 7.0 / 48.0},
	{dx: 1, dy: 1, weight: 5.0 / 48.0},
	{dx: 2, dy: 1, weight: 3.0 / 48.0},
	{dx: -2, dy: 2, weight: 1.0 / 48.0},
	{dx: -1, dy: 2, weight: 3.0 / 48.0},
	{dx: 0, dy: 2, weight: 5.0 / 48.0},
	{dx: 1, dy: 2, weight: 3.0 / 48.0},
	{dx: 2, dy: 2, weight: 1.0 / 48.0},
}

var sierraKernel = []diffusionTap{
	{dx: 1, dy: 0, weight: 5.0 / 32.0},
	{dx: 2, dy: 0, weight: 3.0 / 32.0},
	{dx: -2, dy: 1, weight: 2.0 / 32.0},
	{dx: -1, dy: 1, weight: 4.0 / 32.0},
	{dx: 0, dy: 1, weight: 5.0 / 32.0},
	{dx: 1, dy: 1, weight: 4.0 / 32.0},
	{dx: 2, dy: 1, weight: 2.0 / 32.0},
	{dx: -1, dy: 2, weight: 2.0 / 32.0},
	{dx: 0, dy: 2, weight: 3.0 / 32.0},
	{dx: 1, dy: 2, weight: 2.0 / 32.0},
}

func diffuseGrayKernel(gray []float64, width, height int, thresholdValue float64, levels int, diffusion float64, kernel []diffusionTap, blueNoiseAmount float64) ([]float64, []float64) {
	if thresholdValue < 0 {
		thresholdValue = 0
	}
	if thresholdValue > 1 {
		thresholdValue = 1
	}
	if diffusion < 0 {
		diffusion = 0
	}
	if diffusion > 1 {
		diffusion = 1
	}
	blueNoiseAmount = clamp01(blueNoiseAmount)

	working := make([]float64, len(gray))
	copy(working, gray)
	result := make([]float64, len(gray))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			old := clamp01(working[index])
			noisy := old + (blueNoise(x, y)-0.5)*blueNoiseAmount
			newValue := quantize(clamp01(noisy), levels, thresholdValue)
			result[index] = newValue
			errValue := (old - newValue) * diffusion
			for _, tap := range kernel {
				px := x + tap.dx
				py := y + tap.dy
				if px < 0 || py < 0 || px >= width || py >= height {
					continue
				}
				working[py*width+px] += errValue * tap.weight
			}
		}
	}

	intermediate := make([]float64, len(working))
	for index, value := range working {
		intermediate[index] = clamp01(value)
	}
	return intermediate, result
}

func orderedGrayDither(gray []float64, width, height int, thresholdValue float64, levels int, strength float64) ([]float64, []float64) {
	strength = clamp01(strength)
	intermediate := make([]float64, len(gray))
	result := make([]float64, len(gray))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			value := clamp01(gray[index])
			noise := (bayer4(x, y) - 0.5) * strength
			adjusted := clamp01(value + noise/float64(normalizeLevels(levels)))
			intermediate[index] = adjusted
			result[index] = quantize(adjusted, levels, thresholdValue)
		}
	}
	return intermediate, result
}

func blueNoiseThresholdGray(gray []float64, width, height int, thresholdValue float64, levels int, amount float64) ([]float64, []float64) {
	amount = clamp01(amount)
	intermediate := make([]float64, len(gray))
	result := make([]float64, len(gray))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			value := clamp01(gray[index])
			noise := (blueNoise(x, y) - 0.5) * amount
			adjusted := clamp01(value + noise/float64(normalizeLevels(levels)))
			intermediate[index] = adjusted
			result[index] = quantize(adjusted, levels, thresholdValue)
		}
	}
	return intermediate, result
}

func orderedDitherColor(source image.Image, palette []color.RGBA, strength float64) *image.RGBA {
	bounds := source.Bounds()
	output := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	strength = clamp01(strength)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := source.At(x, y).RGBA()
			noise := (bayer4(x-bounds.Min.X, y-bounds.Min.Y) - 0.5) * strength * 0.25
			cr := clamp01(float64(r>>8)/255.0 + noise)
			cg := clamp01(float64(g>>8)/255.0 + noise)
			cb := clamp01(float64(b>>8)/255.0 + noise)
			closest := nearestPaletteColor(cr, cg, cb, palette)
			output.SetRGBA(x-bounds.Min.X, y-bounds.Min.Y, closest)
		}
	}
	return output
}

func blueNoiseThresholdColor(source image.Image, palette []color.RGBA, amount float64) *image.RGBA {
	bounds := source.Bounds()
	output := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	amount = clamp01(amount)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := source.At(x, y).RGBA()
			noise := (blueNoise(x-bounds.Min.X, y-bounds.Min.Y) - 0.5) * amount * 0.25
			cr := clamp01(float64(r>>8)/255.0 + noise)
			cg := clamp01(float64(g>>8)/255.0 + noise)
			cb := clamp01(float64(b>>8)/255.0 + noise)
			closest := nearestPaletteColor(cr, cg, cb, palette)
			output.SetRGBA(x-bounds.Min.X, y-bounds.Min.Y, closest)
		}
	}
	return output
}

func diffuseColorKernel(source image.Image, palette []color.RGBA, diffusion float64, kernel []diffusionTap, blueNoiseAmount float64) *image.RGBA {
	if diffusion < 0 {
		diffusion = 0
	}
	if diffusion > 1 {
		diffusion = 1
	}
	blueNoiseAmount = clamp01(blueNoiseAmount)

	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	size := width * height
	rChan := make([]float64, size)
	gChan := make([]float64, size)
	bChan := make([]float64, size)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			index := y*width + x
			rChan[index] = float64(r>>8) / 255.0
			gChan[index] = float64(g>>8) / 255.0
			bChan[index] = float64(b>>8) / 255.0
		}
	}

	output := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			noise := (blueNoise(x, y) - 0.5) * blueNoiseAmount * 0.25
			oldR := clamp01(rChan[index] + noise)
			oldG := clamp01(gChan[index] + noise)
			oldB := clamp01(bChan[index] + noise)
			newColor := nearestPaletteColor(oldR, oldG, oldB, palette)
			output.SetRGBA(x, y, newColor)

			newR := float64(newColor.R) / 255.0
			newG := float64(newColor.G) / 255.0
			newB := float64(newColor.B) / 255.0
			errR := (rChan[index] - newR) * diffusion
			errG := (gChan[index] - newG) * diffusion
			errB := (bChan[index] - newB) * diffusion

			for _, tap := range kernel {
				px := x + tap.dx
				py := y + tap.dy
				if px < 0 || py < 0 || px >= width || py >= height {
					continue
				}
				applyIndex := py*width + px
				rChan[applyIndex] += errR * tap.weight
				gChan[applyIndex] += errG * tap.weight
				bChan[applyIndex] += errB * tap.weight
			}
		}
	}

	return output
}

func bayer4(x, y int) float64 {
	bayer := [4][4]float64{
		{0, 8, 2, 10},
		{12, 4, 14, 6},
		{3, 11, 1, 9},
		{15, 7, 13, 5},
	}
	return bayer[y&3][x&3] / 16.0
}

func blueNoise(x, y int) float64 {
	fx := float64(x)
	fy := float64(y)
	v := 52.9829189 * frac(0.06711056*fx+0.00583715*fy)
	return frac(v)
}

func frac(value float64) float64 {
	return value - math.Floor(value)
}

func diffuseColorError(rChan, gChan, bChan []float64, width, height, x, y int, errR, errG, errB float64) {
	apply := func(px, py int, weight float64) {
		if px < 0 || py < 0 || px >= width || py >= height {
			return
		}
		index := py*width + px
		rChan[index] += errR * weight
		gChan[index] += errG * weight
		bChan[index] += errB * weight
	}
	apply(x+1, y, 7.0/16.0)
	apply(x-1, y+1, 3.0/16.0)
	apply(x, y+1, 5.0/16.0)
	apply(x+1, y+1, 1.0/16.0)
}

func nearestPaletteColor(r, g, b float64, palette []color.RGBA) color.RGBA {
	if len(palette) == 0 {
		value := uint8(clamp01((r+g+b)/3.0) * 255)
		return color.RGBA{R: value, G: value, B: value, A: 255}
	}
	best := palette[0]
	bestDist := math.MaxFloat64
	for _, candidate := range palette {
		cr := float64(candidate.R) / 255.0
		cg := float64(candidate.G) / 255.0
		cb := float64(candidate.B) / 255.0
		dr := r - cr
		dg := g - cg
		db := b - cb
		dist := dr*dr + dg*dg + db*db
		if dist < bestDist {
			bestDist = dist
			best = candidate
		}
	}
	return best
}

func copyToRGBA(source image.Image) *image.RGBA {
	bounds := source.Bounds()
	output := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := source.At(x, y).RGBA()
			output.SetRGBA(x-bounds.Min.X, y-bounds.Min.Y, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)})
		}
	}
	return output
}

func toGray(source image.Image) ([]float64, int, int) {
	bounds := source.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	gray := make([]float64, width*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, g, b, _ := source.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			rf := float64(r>>8) / 255.0
			gf := float64(g>>8) / 255.0
			bf := float64(b>>8) / 255.0
			gray[y*width+x] = 0.299*rf + 0.587*gf + 0.114*bf
		}
	}
	return gray, width, height
}

func threshold(gray []float64, width, height int, thresholdValue float64, levels int) ([]float64, []float64) {
	if thresholdValue < 0 {
		thresholdValue = 0
	}
	if thresholdValue > 1 {
		thresholdValue = 1
	}

	intermediate := make([]float64, len(gray))
	result := make([]float64, len(gray))
	for index, value := range gray {
		intermediate[index] = value
		result[index] = quantize(value, levels, thresholdValue)
	}
	return intermediate, result
}

func floydSteinberg(gray []float64, width, height int, thresholdValue, diffusion float64, levels int) ([]float64, []float64) {
	if thresholdValue < 0 {
		thresholdValue = 0
	}
	if thresholdValue > 1 {
		thresholdValue = 1
	}
	if diffusion < 0 {
		diffusion = 0
	}
	if diffusion > 1 {
		diffusion = 1
	}

	working := make([]float64, len(gray))
	copy(working, gray)
	result := make([]float64, len(gray))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			old := clamp01(working[index])
			newValue := quantize(old, levels, thresholdValue)
			result[index] = newValue
			errorValue := (old - newValue) * diffusion

			if x+1 < width {
				working[index+1] += errorValue * (7.0 / 16.0)
			}
			if y+1 < height {
				down := index + width
				if x > 0 {
					working[down-1] += errorValue * (3.0 / 16.0)
				}
				working[down] += errorValue * (5.0 / 16.0)
				if x+1 < width {
					working[down+1] += errorValue * (1.0 / 16.0)
				}
			}
		}
	}

	for index := range working {
		working[index] = clamp01(working[index])
	}

	return working, result
}

func grayscalePreview(values []float64, width, height int) *image.RGBA {
	output := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value := uint8(clamp01(values[y*width+x]) * 255)
			output.SetRGBA(x, y, color.RGBA{R: value, G: value, B: value, A: 255})
		}
	}
	return output
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func quantize(value float64, levels int, threshold float64) float64 {
	value = clamp01(value)
	levels = normalizeLevels(levels)
	if levels <= 2 {
		if value >= clamp01(threshold) {
			return 1
		}
		return 0
	}
	steps := levels - 1
	index := int(math.Round(value * float64(steps)))
	if index < 0 {
		index = 0
	}
	if index > steps {
		index = steps
	}
	return float64(index) / float64(steps)
}

func normalizeLevels(levels int) int {
	if levels < 2 {
		return 2
	}
	if levels > 64 {
		return 64
	}
	return levels
}

func resampleInterpolation(resample ResampleAlgorithm) resize.InterpolationFunction {
	switch resample {
	case ResampleNearest:
		return resize.NearestNeighbor
	case ResampleBilinear:
		return resize.Bilinear
	case ResampleBicubic:
		return resize.Bicubic
	default:
		return resize.Lanczos3
	}
}

func strconvItoa(value int) string {
	if value == 0 {
		return "0"
	}
	sign := ""
	if value < 0 {
		sign = "-"
		value = -value
	}
	digits := make([]byte, 0, 6)
	for value > 0 {
		digits = append(digits, byte('0'+value%10))
		value /= 10
	}
	for left, right := 0, len(digits)-1; left < right; left, right = left+1, right-1 {
		digits[left], digits[right] = digits[right], digits[left]
	}
	return sign + string(digits)
}

func clonePalette(palette []color.RGBA) []color.RGBA {
	if len(palette) == 0 {
		return nil
	}
	cloned := make([]color.RGBA, len(palette))
	copy(cloned, palette)
	return cloned
}
