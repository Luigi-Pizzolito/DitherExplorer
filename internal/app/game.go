package app

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/luigipizzolito/ditherprinter/internal/capture"
	"github.com/luigipizzolito/ditherprinter/internal/dither"
	"github.com/luigipizzolito/ditherprinter/internal/layout"
)

type Game struct {
	layout layout.Split

	capture *capture.PortalCapture

	algorithms        []dither.Algorithm
	paletteAlgorithms []dither.PaletteAlgorithm
	paletteModes      []dither.PaletteMode
	resamples         []dither.ResampleAlgorithm
	sharpens          []dither.SharpenAlgorithm

	algorithm dither.Algorithm
	params    dither.Params
	state     dither.ProcessState

	lastCaptureFrameID uint64
	sourceFrame        image.Image
	stageImages        [3]*ebiten.Image
	pipelineDirty      bool

	algorithmMenuOpen bool
	paletteMenuOpen   bool
	paletteModeOpen   bool
	resampleMenuOpen  bool
	sharpenMenuOpen   bool
	outputFullscreen  bool

	lastMousePressed bool
}

func NewGame() *Game {
	return &Game{
		capture: capture.NewPortalCapture(),
		algorithms: []dither.Algorithm{
			dither.AlgorithmThreshold,
			dither.AlgorithmFloyd,
			dither.AlgorithmBayer,
			dither.AlgorithmAtkinson,
			dither.AlgorithmJarvisJudiceNinke,
			dither.AlgorithmSierra,
			dither.AlgorithmBlueNoiseThreshold,
			dither.AlgorithmBlueNoiseHybrid,
		},
		paletteAlgorithms: []dither.PaletteAlgorithm{dither.PaletteUniform, dither.PalettePopular, dither.PaletteMedian, dither.PaletteKMeans},
		paletteModes:      []dither.PaletteMode{dither.PaletteDynamic, dither.PaletteStatic},
		resamples: []dither.ResampleAlgorithm{
			dither.ResampleNearest,
			dither.ResampleBilinear,
			dither.ResampleBicubic,
			dither.ResampleLanczos,
		},
		sharpens: []dither.SharpenAlgorithm{
			dither.SharpenNone,
			dither.SharpenUnsharp,
			dither.SharpenLineBoost,
			dither.SharpenAnimeEdge,
		},
		algorithm: dither.AlgorithmBlueNoiseThreshold,
		params: dither.Params{
			Threshold:        0.5,
			Diffusion:        1.0,
			Levels:           16,
			Scale:            0.3,
			Resample:         dither.ResampleLanczos,
			SharpenAlgorithm: dither.SharpenLineBoost,
			SharpenStrength:  0.45,
			UseColor:         true,
			PaletteAlgorithm: dither.PaletteKMeans,
			PaletteMode:      dither.PaletteDynamic,
			PaletteUpdateInt: 4,
			OrderedStrength:  0.6,
			BlueNoiseAmount:  0.45,
		},
		pipelineDirty: true,
	}
}

func (game *Game) Update() error {
	windowW, windowH := ebiten.WindowSize()
	game.layout = layout.Compute(windowW, windowH)

	mousePressed := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	justPressed := mousePressed && !game.lastMousePressed
	mouseX, mouseY := ebiten.CursorPosition()

	if justPressed {
		if game.outputFullscreen {
			game.outputFullscreen = false
			game.closeMenus()
			game.lastMousePressed = mousePressed
			return nil
		}
		if game.anyMenuOpen() {
			game.handleOpenMenuClick(mouseX, mouseY)
		} else {
			game.handleBaseClick(mouseX, mouseY)
		}
	}

	if !game.outputFullscreen {
		if game.updateScaleSlider(mouseX, mouseY, mousePressed) {
			game.pipelineDirty = true
		}
		if game.updateSlider(mouseX, mouseY, mousePressed, game.sharpenStrengthSliderRect(), 0, 1, &game.params.SharpenStrength) {
			game.pipelineDirty = true
		}
		if game.updatePaletteIntervalSlider(mouseX, mouseY, mousePressed) {
			game.pipelineDirty = true
		}
		if game.updateLevelsSlider(mouseX, mouseY, mousePressed) {
			game.resetStaticPalette()
			game.pipelineDirty = true
		}
		if game.updateSlider(mouseX, mouseY, mousePressed, game.thresholdSliderRect(), 0, 1, &game.params.Threshold) {
			game.pipelineDirty = true
		}
		if game.algorithmUsesDiffusionControl() {
			if game.updateSlider(mouseX, mouseY, mousePressed, game.diffusionSliderRect(), 0, 1, &game.params.Diffusion) {
				game.pipelineDirty = true
			}
		}
		if game.algorithmUsesOrderedControl() {
			if game.updateSlider(mouseX, mouseY, mousePressed, game.orderedStrengthSliderRect(), 0, 1, &game.params.OrderedStrength) {
				game.pipelineDirty = true
			}
		}
		if game.algorithmUsesBlueNoiseControl() {
			if game.updateSlider(mouseX, mouseY, mousePressed, game.blueNoiseSliderRect(), 0, 1, &game.params.BlueNoiseAmount) {
				game.pipelineDirty = true
			}
		}
	}

	currentCaptureFrameID := game.capture.FrameID()
	if currentCaptureFrameID != game.lastCaptureFrameID {
		latest := game.capture.LatestFrame()
		if latest != nil {
			game.sourceFrame = latest
			game.lastCaptureFrameID = currentCaptureFrameID
			game.pipelineDirty = true
		}
	}

	if game.pipelineDirty && game.sourceFrame != nil {
		stage1, stage2, stage3 := dither.Process(game.sourceFrame, game.algorithm, game.params, &game.state)
		game.stageImages[0] = ebiten.NewImageFromImage(stage1)
		game.stageImages[1] = ebiten.NewImageFromImage(stage2)
		game.stageImages[2] = ebiten.NewImageFromImage(stage3)
		game.pipelineDirty = false
	}

	game.lastMousePressed = mousePressed
	return nil
}

func (game *Game) handleOpenMenuClick(mouseX, mouseY int) {
	if game.algorithmMenuOpen {
		if algorithm, ok := game.algorithmMenuSelection(mouseX, mouseY); ok {
			game.algorithm = algorithm
			game.pipelineDirty = true
		}
		game.closeMenus()
		return
	}
	if game.paletteMenuOpen {
		if paletteAlgorithm, ok := game.paletteMenuSelection(mouseX, mouseY); ok {
			game.params.PaletteAlgorithm = paletteAlgorithm
			game.resetStaticPalette()
			game.pipelineDirty = true
		}
		game.closeMenus()
		return
	}
	if game.paletteModeOpen {
		if paletteMode, ok := game.paletteModeSelection(mouseX, mouseY); ok {
			game.params.PaletteMode = paletteMode
			game.resetStaticPalette()
			game.pipelineDirty = true
		}
		game.closeMenus()
		return
	}
	if game.resampleMenuOpen {
		if resample, ok := game.resampleMenuSelection(mouseX, mouseY); ok {
			game.params.Resample = resample
			game.pipelineDirty = true
		}
		game.closeMenus()
		return
	}
	if game.sharpenMenuOpen {
		if sharpen, ok := game.sharpenMenuSelection(mouseX, mouseY); ok {
			game.params.SharpenAlgorithm = sharpen
			game.pipelineDirty = true
		}
		game.closeMenus()
		return
	}
}

func (game *Game) handleBaseClick(mouseX, mouseY int) {
	switch {
	case pointInRect(mouseX, mouseY, game.layout.PanelAreas[2]):
		game.outputFullscreen = true
		game.closeMenus()
		return
	case pointInRect(mouseX, mouseY, game.captureButtonRect()):
		if game.capture.IsRunning() {
			game.capture.Stop()
		} else {
			_ = game.capture.Start()
		}
		game.closeMenus()
	case pointInRect(mouseX, mouseY, game.resampleButtonRect()):
		game.resampleMenuOpen = true
	case pointInRect(mouseX, mouseY, game.sharpenButtonRect()):
		game.sharpenMenuOpen = true
	case pointInRect(mouseX, mouseY, game.algorithmButtonRect()):
		game.algorithmMenuOpen = true
	case pointInRect(mouseX, mouseY, game.colorToggleRect()):
		game.params.UseColor = !game.params.UseColor
		game.resetStaticPalette()
		game.pipelineDirty = true
	case pointInRect(mouseX, mouseY, game.paletteButtonRect()):
		game.paletteMenuOpen = true
	case pointInRect(mouseX, mouseY, game.paletteModeButtonRect()):
		game.paletteModeOpen = true
	default:
		return
	}

	if !pointInRect(mouseX, mouseY, game.resampleButtonRect()) {
		game.resampleMenuOpen = false
	}
	if !pointInRect(mouseX, mouseY, game.sharpenButtonRect()) {
		game.sharpenMenuOpen = false
	}
	if !pointInRect(mouseX, mouseY, game.algorithmButtonRect()) {
		game.algorithmMenuOpen = false
	}
	if !pointInRect(mouseX, mouseY, game.paletteButtonRect()) {
		game.paletteMenuOpen = false
	}
	if !pointInRect(mouseX, mouseY, game.paletteModeButtonRect()) {
		game.paletteModeOpen = false
	}
}

func (game *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 15, G: 17, B: 20, A: 255})
	if game.outputFullscreen {
		window := game.layout.Window
		if game.stageImages[2] != nil {
			drawImageIntoRect(screen, game.stageImages[2], window)
		}
		return
	}

	for index := range 3 {
		panelArea := game.layout.PanelAreas[index]
		contentArea := game.layout.PanelContent[index]

		ebitenutil.DrawRect(screen, float64(panelArea.Min.X), float64(panelArea.Min.Y), float64(panelArea.Dx()), float64(panelArea.Dy()), color.RGBA{R: 29, G: 33, B: 39, A: 255})
		vector.StrokeRect(screen, float32(panelArea.Min.X), float32(panelArea.Min.Y), float32(panelArea.Dx()), float32(panelArea.Dy()), 2, color.RGBA{R: 70, G: 80, B: 92, A: 255}, false)

		ebitenutil.DrawRect(screen, float64(contentArea.Min.X), float64(contentArea.Min.Y), float64(contentArea.Dx()), float64(contentArea.Dy()), color.RGBA{R: 8, G: 9, B: 10, A: 255})
		vector.StrokeRect(screen, float32(contentArea.Min.X), float32(contentArea.Min.Y), float32(contentArea.Dx()), float32(contentArea.Dy()), 1, color.RGBA{R: 95, G: 105, B: 120, A: 255}, false)

		if game.stageImages[index] != nil {
			drawImageIntoRect(screen, game.stageImages[index], contentArea)
		} else {
			ebitenutil.DebugPrintAt(screen, "Waiting for live capture...", contentArea.Min.X+10, contentArea.Min.Y+10)
		}

		labels := [3]string{"Input", game.algorithmPreviewLabel(), "Output"}
		ebitenutil.DebugPrintAt(screen, labels[index], panelArea.Min.X+8, panelArea.Min.Y+8)
	}

	sidebar := game.layout.Sidebar
	ebitenutil.DrawRect(screen, float64(sidebar.Min.X), float64(sidebar.Min.Y), float64(sidebar.Dx()), float64(sidebar.Dy()), color.RGBA{R: 24, G: 26, B: 32, A: 255})
	vector.StrokeRect(screen, float32(sidebar.Min.X), float32(sidebar.Min.Y), float32(sidebar.Dx()), float32(sidebar.Dy()), 2, color.RGBA{R: 76, G: 86, B: 103, A: 255}, false)

	game.drawSidebar(screen)
	game.drawAlgorithmMenuOverlay(screen)
	game.drawPaletteMenuOverlay(screen)
	game.drawPaletteModeOverlay(screen)
	game.drawResampleMenuOverlay(screen)
	game.drawSharpenMenuOverlay(screen)
}

func (game *Game) drawSidebar(screen *ebiten.Image) {
	sidebar := game.layout.Sidebar
	ebitenutil.DebugPrintAt(screen, "Dither Explorer", sidebar.Min.X+16, sidebar.Min.Y+12)

	game.drawSection(screen, game.captureSectionRect(), "Capture")
	game.drawSection(screen, game.preScaleSectionRect(), "Pre-scale")
	game.drawSection(screen, game.preProcessSectionRect(), "Pre-processing")
	game.drawSection(screen, game.ditherSectionRect(), "Dither")
	game.drawSection(screen, game.paletteSectionRect(), "Palette")

	captureRect := game.captureButtonRect()
	buttonColor := color.RGBA{R: 57, G: 82, B: 130, A: 255}
	buttonLabel := "Start Capture"
	if game.capture.IsRunning() {
		buttonColor = color.RGBA{R: 120, G: 61, B: 61, A: 255}
		buttonLabel = "Stop Capture"
	}
	ebitenutil.DrawRect(screen, float64(captureRect.Min.X), float64(captureRect.Min.Y), float64(captureRect.Dx()), float64(captureRect.Dy()), buttonColor)
	vector.StrokeRect(screen, float32(captureRect.Min.X), float32(captureRect.Min.Y), float32(captureRect.Dx()), float32(captureRect.Dy()), 1, color.RGBA{R: 180, G: 190, B: 210, A: 255}, false)
	ebitenutil.DebugPrintAt(screen, buttonLabel, captureRect.Min.X+10, captureRect.Min.Y+10)

	status := fmt.Sprintf("Status: %s", game.capture.Status())
	ebitenutil.DebugPrintAt(screen, status, sidebar.Min.X+16, captureRect.Max.Y+8)

	game.drawDropdownButton(screen, game.resampleButtonRect(), "Resample", string(game.params.Resample), game.resampleMenuOpen)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Pre-scale: %.2fx", game.params.Scale), sidebar.Min.X+16, game.resampleButtonRect().Max.Y+8)
	game.drawSlider(screen, game.scaleSliderRect(), (game.params.Scale-0.10)/0.90, color.RGBA{R: 104, G: 180, B: 148, A: 255})
	outW, outH := game.outputResolution()
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Output resolution: %dx%d", outW, outH), sidebar.Min.X+16, game.scaleSliderRect().Max.Y+8)

	game.drawDropdownButton(screen, game.sharpenButtonRect(), "Edge Sharpen", string(game.params.SharpenAlgorithm), game.sharpenMenuOpen)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Sharpen strength: %.2f", game.params.SharpenStrength), sidebar.Min.X+16, game.sharpenStrengthSliderRect().Min.Y-14)
	game.drawSlider(screen, game.sharpenStrengthSliderRect(), game.params.SharpenStrength, color.RGBA{R: 228, G: 163, B: 83, A: 255})

	game.drawDropdownButton(screen, game.algorithmButtonRect(), "Algorithm", string(game.algorithm), game.algorithmMenuOpen)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Output levels: %d", game.params.Levels), sidebar.Min.X+16, game.levelsSliderRect().Min.Y-14)
	game.drawSlider(screen, game.levelsSliderRect(), levelsToNormalized(game.params.Levels), color.RGBA{R: 188, G: 128, B: 214, A: 255})
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Threshold: %.2f", game.params.Threshold), sidebar.Min.X+16, game.thresholdSliderRect().Min.Y-14)
	game.drawSlider(screen, game.thresholdSliderRect(), game.params.Threshold, color.RGBA{R: 82, G: 150, B: 243, A: 255})
	if game.algorithmUsesDiffusionControl() {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Error Diffusion: %.2f", game.params.Diffusion), sidebar.Min.X+16, game.diffusionSliderRect().Min.Y-14)
		game.drawSlider(screen, game.diffusionSliderRect(), game.params.Diffusion, color.RGBA{R: 220, G: 166, B: 64, A: 255})
	}
	if game.algorithmUsesOrderedControl() {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Ordered strength: %.2f", game.params.OrderedStrength), sidebar.Min.X+16, game.orderedStrengthSliderRect().Min.Y-14)
		game.drawSlider(screen, game.orderedStrengthSliderRect(), game.params.OrderedStrength, color.RGBA{R: 165, G: 193, B: 97, A: 255})
	}
	if game.algorithmUsesBlueNoiseControl() {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Blue-noise amount: %.2f", game.params.BlueNoiseAmount), sidebar.Min.X+16, game.blueNoiseSliderRect().Min.Y-14)
		game.drawSlider(screen, game.blueNoiseSliderRect(), game.params.BlueNoiseAmount, color.RGBA{R: 104, G: 153, B: 209, A: 255})
	}

	colorRect := game.colorToggleRect()
	colorBackground := color.RGBA{R: 43, G: 49, B: 59, A: 255}
	if game.params.UseColor {
		colorBackground = color.RGBA{R: 62, G: 109, B: 88, A: 255}
	}
	ebitenutil.DrawRect(screen, float64(colorRect.Min.X), float64(colorRect.Min.Y), float64(colorRect.Dx()), float64(colorRect.Dy()), colorBackground)
	vector.StrokeRect(screen, float32(colorRect.Min.X), float32(colorRect.Min.Y), float32(colorRect.Dx()), float32(colorRect.Dy()), 1, color.RGBA{R: 130, G: 140, B: 156, A: 255}, false)
	modeText := "Color: ON"
	if !game.params.UseColor {
		modeText = "Color: OFF (B/W)"
	}
	ebitenutil.DebugPrintAt(screen, modeText, colorRect.Min.X+10, colorRect.Min.Y+10)

	game.drawDropdownButton(screen, game.paletteButtonRect(), "Palette Quant", string(game.params.PaletteAlgorithm), game.paletteMenuOpen)
	game.drawDropdownButton(screen, game.paletteModeButtonRect(), "Palette Mode", string(game.params.PaletteMode), game.paletteModeOpen)
	intervalLabel := "Palette update: Every frame"
	if game.params.PaletteUpdateInt > 1 {
		intervalLabel = fmt.Sprintf("Palette update: Every %d frames", game.params.PaletteUpdateInt)
	}
	ebitenutil.DebugPrintAt(screen, intervalLabel, sidebar.Min.X+16, game.paletteIntervalSliderRect().Min.Y-14)
	game.drawSlider(screen, game.paletteIntervalSliderRect(), paletteIntervalToNormalized(game.params.PaletteUpdateInt), color.RGBA{R: 114, G: 168, B: 226, A: 255})
	ebitenutil.DebugPrintAt(screen, "Palette preview:", sidebar.Min.X+16, game.palettePreviewRect().Min.Y-14)
	game.drawPalettePreview(screen)
}

func (game *Game) drawDropdownButton(screen *ebiten.Image, rect image.Rectangle, label, value string, open bool) {
	background := color.RGBA{R: 43, G: 49, B: 59, A: 255}
	ebitenutil.DrawRect(screen, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Dx()), float64(rect.Dy()), background)
	vector.StrokeRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), 1, color.RGBA{R: 130, G: 140, B: 156, A: 255}, false)
	indicator := "▼"
	if open {
		indicator = "▲"
	}
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s: %s %s", label, value, indicator), rect.Min.X+10, rect.Min.Y+10)
}

func (game *Game) drawSection(screen *ebiten.Image, rect image.Rectangle, title string) {
	ebitenutil.DrawRect(screen, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Dx()), float64(rect.Dy()), color.RGBA{R: 30, G: 34, B: 41, A: 255})
	vector.StrokeRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), 1, color.RGBA{R: 85, G: 95, B: 110, A: 255}, false)
	ebitenutil.DebugPrintAt(screen, title, rect.Min.X+8, rect.Min.Y+6)
}

func (game *Game) drawSlider(screen *ebiten.Image, rect image.Rectangle, value float64, fill color.Color) {
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}

	ebitenutil.DrawRect(screen, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Dx()), float64(rect.Dy()), color.RGBA{R: 53, G: 59, B: 71, A: 255})
	ebitenutil.DrawRect(screen, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Dx())*value, float64(rect.Dy()), fill)
	vector.StrokeRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), 1, color.RGBA{R: 140, G: 150, B: 167, A: 255}, false)

	knobX := rect.Min.X + int(float64(rect.Dx())*value)
	if knobX < rect.Min.X {
		knobX = rect.Min.X
	}
	if knobX > rect.Max.X {
		knobX = rect.Max.X
	}
	ebitenutil.DrawRect(screen, float64(knobX-4), float64(rect.Min.Y-3), 8, float64(rect.Dy()+6), color.RGBA{R: 240, G: 245, B: 255, A: 255})
}

func (game *Game) drawPalettePreview(screen *ebiten.Image) {
	rect := game.palettePreviewRect()
	ebitenutil.DrawRect(screen, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Dx()), float64(rect.Dy()), color.RGBA{R: 28, G: 32, B: 38, A: 255})
	vector.StrokeRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), 1, color.RGBA{R: 140, G: 150, B: 167, A: 255}, false)

	palette := game.state.LastPalette
	if !game.params.UseColor {
		ebitenutil.DebugPrintAt(screen, "Disabled in B/W mode", rect.Min.X+8, rect.Min.Y+6)
		return
	}
	if len(palette) == 0 {
		ebitenutil.DebugPrintAt(screen, "No palette yet", rect.Min.X+8, rect.Min.Y+6)
		return
	}

	columns := 8
	if columns < 1 {
		columns = 1
	}
	cellSize := rect.Dx() / columns
	if cellSize < 1 {
		cellSize = 1
	}
	rows := rect.Dy() / cellSize
	if rows < 1 {
		rows = 1
	}
	capacity := columns * rows
	swatchCount := len(palette)
	if swatchCount > capacity {
		swatchCount = capacity
	}

	for index := 0; index < swatchCount; index++ {
		swatch := palette[index]
		column := index % columns
		row := index / columns
		x := rect.Min.X + column*cellSize
		y := rect.Min.Y + row*cellSize
		w := cellSize
		h := cellSize
		if x+w > rect.Max.X {
			w = rect.Max.X - x
		}
		if y+h > rect.Max.Y {
			h = rect.Max.Y - y
		}
		if w <= 0 || h <= 0 {
			continue
		}
		ebitenutil.DrawRect(screen, float64(x), float64(y), float64(w), float64(h), swatch)
	}
}

func (game *Game) updateSlider(mouseX, mouseY int, mousePressed bool, sliderRect image.Rectangle, minValue, maxValue float64, target *float64) bool {
	if game.anyMenuOpen() {
		return false
	}
	if !mousePressed || !pointInRect(mouseX, mouseY, sliderRect) {
		return false
	}
	normalized := float64(mouseX-sliderRect.Min.X) / float64(sliderRect.Dx())
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	newValue := minValue + normalized*(maxValue-minValue)
	if abs(*target-newValue) > 0.0001 {
		*target = newValue
		return true
	}
	return false
}

func (game *Game) updateScaleSlider(mouseX, mouseY int, mousePressed bool) bool {
	rect := game.scaleSliderRect()
	if game.anyMenuOpen() || !mousePressed || !pointInRect(mouseX, mouseY, rect) {
		return false
	}
	normalized := float64(mouseX-rect.Min.X) / float64(rect.Dx())
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	scale := 0.10 + normalized*0.90
	if abs(game.params.Scale-scale) > 0.001 {
		game.params.Scale = scale
		return true
	}
	return false
}

func (game *Game) updateLevelsSlider(mouseX, mouseY int, mousePressed bool) bool {
	rect := game.levelsSliderRect()
	if game.anyMenuOpen() || !mousePressed || !pointInRect(mouseX, mouseY, rect) {
		return false
	}
	normalized := float64(mouseX-rect.Min.X) / float64(rect.Dx())
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	levels := normalizedToLevels(normalized)
	if game.params.Levels != levels {
		game.params.Levels = levels
		return true
	}
	return false
}

func (game *Game) updatePaletteIntervalSlider(mouseX, mouseY int, mousePressed bool) bool {
	rect := game.paletteIntervalSliderRect()
	if game.anyMenuOpen() || !mousePressed || !pointInRect(mouseX, mouseY, rect) {
		return false
	}
	normalized := float64(mouseX-rect.Min.X) / float64(rect.Dx())
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}
	interval := normalizedToPaletteInterval(normalized)
	if game.params.PaletteUpdateInt != interval {
		game.params.PaletteUpdateInt = interval
		return true
	}
	return false
}

func (game *Game) closeMenus() {
	game.algorithmMenuOpen = false
	game.paletteMenuOpen = false
	game.paletteModeOpen = false
	game.resampleMenuOpen = false
	game.sharpenMenuOpen = false
}

func (game *Game) resetStaticPalette() {
	game.state.StaticPalette = nil
	game.state.PaletteKey = ""
	game.state.DynamicPalette = nil
	game.state.DynamicPaletteKey = ""
	game.state.DynamicPaletteAge = 0
}

func (game *Game) outputResolution() (int, int) {
	if game.sourceFrame == nil {
		return 0, 0
	}
	bounds := game.sourceFrame.Bounds()
	w := int(math.Round(float64(bounds.Dx()) * game.params.Scale))
	h := int(math.Round(float64(bounds.Dy()) * game.params.Scale))
	if game.params.Scale <= 0 || game.params.Scale >= 1 {
		w = bounds.Dx()
		h = bounds.Dy()
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

func (game *Game) captureButtonRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+60, sidebar.Max.X-16, sidebar.Min.Y+92)
}

func (game *Game) resampleButtonRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+158, sidebar.Max.X-16, sidebar.Min.Y+190)
}

func (game *Game) scaleSliderRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+220, sidebar.Max.X-16, sidebar.Min.Y+240)
}

func (game *Game) sharpenButtonRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+322, sidebar.Max.X-16, sidebar.Min.Y+354)
}

func (game *Game) sharpenStrengthSliderRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+382, sidebar.Max.X-16, sidebar.Min.Y+402)
}

func (game *Game) algorithmButtonRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+434, sidebar.Max.X-16, sidebar.Min.Y+466)
}

func (game *Game) levelsSliderRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+494, sidebar.Max.X-16, sidebar.Min.Y+514)
}

func (game *Game) thresholdSliderRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+550, sidebar.Max.X-16, sidebar.Min.Y+570)
}

func (game *Game) diffusionSliderRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+596, sidebar.Max.X-16, sidebar.Min.Y+616)
}

func (game *Game) orderedStrengthSliderRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+642, sidebar.Max.X-16, sidebar.Min.Y+662)
}

func (game *Game) blueNoiseSliderRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+16, sidebar.Min.Y+688, sidebar.Max.X-16, sidebar.Min.Y+708)
}

func (game *Game) colorToggleRect() image.Rectangle {
	section := game.paletteSectionRect()
	minY := section.Min.Y + 24
	return image.Rect(section.Min.X+4, minY, section.Max.X-4, minY+32)
}

func (game *Game) paletteButtonRect() image.Rectangle {
	colorToggle := game.colorToggleRect()
	minY := colorToggle.Max.Y + 12
	return image.Rect(colorToggle.Min.X, minY, colorToggle.Max.X, minY+32)
}

func (game *Game) paletteModeButtonRect() image.Rectangle {
	paletteButton := game.paletteButtonRect()
	minY := paletteButton.Max.Y + 12
	return image.Rect(paletteButton.Min.X, minY, paletteButton.Max.X, minY+32)
}

func (game *Game) paletteIntervalSliderRect() image.Rectangle {
	paletteMode := game.paletteModeButtonRect()
	minY := paletteMode.Max.Y + 26
	return image.Rect(paletteMode.Min.X, minY, paletteMode.Max.X, minY+20)
}

func (game *Game) palettePreviewRect() image.Rectangle {
	section := game.paletteSectionRect()
	intervalSlider := game.paletteIntervalSliderRect()
	minY := intervalSlider.Max.Y + 24
	maxY := section.Max.Y - 14
	limitMaxY := section.Max.Y - 14
	if maxY > limitMaxY {
		maxY = limitMaxY
	}
	minAllowed := section.Min.Y + 24
	if minY < minAllowed {
		minY = minAllowed
	}
	if maxY < minY+24 {
		maxY = minY + 24
	}
	return image.Rect(section.Min.X+4, minY, section.Max.X-4, maxY)
}

func (game *Game) captureSectionRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+12, sidebar.Min.Y+34, sidebar.Max.X-12, sidebar.Min.Y+126)
}

func (game *Game) preScaleSectionRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+12, sidebar.Min.Y+132, sidebar.Max.X-12, sidebar.Min.Y+290)
}

func (game *Game) preProcessSectionRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	return image.Rect(sidebar.Min.X+12, sidebar.Min.Y+296, sidebar.Max.X-12, sidebar.Min.Y+408)
}

func (game *Game) ditherSectionRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	maxY := game.thresholdSliderRect().Max.Y
	if game.algorithmUsesDiffusionControl() && game.diffusionSliderRect().Max.Y > maxY {
		maxY = game.diffusionSliderRect().Max.Y
	}
	if game.algorithmUsesOrderedControl() && game.orderedStrengthSliderRect().Max.Y > maxY {
		maxY = game.orderedStrengthSliderRect().Max.Y
	}
	if game.algorithmUsesBlueNoiseControl() && game.blueNoiseSliderRect().Max.Y > maxY {
		maxY = game.blueNoiseSliderRect().Max.Y
	}
	maxY += 18
	return image.Rect(sidebar.Min.X+12, sidebar.Min.Y+414, sidebar.Max.X-12, maxY)
}

func (game *Game) paletteSectionRect() image.Rectangle {
	sidebar := game.layout.Sidebar
	minY := game.ditherSectionRect().Max.Y + 12
	maxY := sidebar.Max.Y - 12
	if maxY < minY+150 {
		maxY = minY + 150
	}
	return image.Rect(sidebar.Min.X+12, minY, sidebar.Max.X-12, maxY)
}

func (game *Game) algorithmOptionRect(index int) image.Rectangle {
	buttonRect := game.algorithmButtonRect()
	return menuOptionRect(buttonRect, index, len(game.algorithms), true)
}

func (game *Game) paletteOptionRect(index int) image.Rectangle {
	buttonRect := game.paletteButtonRect()
	return menuOptionRect(buttonRect, index, len(game.paletteAlgorithms), true)
}

func (game *Game) paletteModeOptionRect(index int) image.Rectangle {
	buttonRect := game.paletteModeButtonRect()
	return menuOptionRect(buttonRect, index, len(game.paletteModes), true)
}

func (game *Game) resampleOptionRect(index int) image.Rectangle {
	buttonRect := game.resampleButtonRect()
	return menuOptionRect(buttonRect, index, len(game.resamples), true)
}

func (game *Game) sharpenOptionRect(index int) image.Rectangle {
	buttonRect := game.sharpenButtonRect()
	return menuOptionRect(buttonRect, index, len(game.sharpens), true)
}

func menuOptionRect(buttonRect image.Rectangle, index, total int, openUp bool) image.Rectangle {
	height := buttonRect.Dy()
	minY := buttonRect.Max.Y + index*height
	if openUp {
		minY = buttonRect.Min.Y - (total-index)*height
	}
	maxY := minY + height
	return image.Rect(buttonRect.Min.X, minY, buttonRect.Max.X, maxY)
}

func (game *Game) algorithmMenuSelection(mouseX, mouseY int) (dither.Algorithm, bool) {
	for index, algorithm := range game.algorithms {
		if pointInRect(mouseX, mouseY, game.algorithmOptionRect(index)) {
			return algorithm, true
		}
	}
	return "", false
}

func (game *Game) paletteMenuSelection(mouseX, mouseY int) (dither.PaletteAlgorithm, bool) {
	for index, palette := range game.paletteAlgorithms {
		if pointInRect(mouseX, mouseY, game.paletteOptionRect(index)) {
			return palette, true
		}
	}
	return "", false
}

func (game *Game) paletteModeSelection(mouseX, mouseY int) (dither.PaletteMode, bool) {
	for index, mode := range game.paletteModes {
		if pointInRect(mouseX, mouseY, game.paletteModeOptionRect(index)) {
			return mode, true
		}
	}
	return "", false
}

func (game *Game) resampleMenuSelection(mouseX, mouseY int) (dither.ResampleAlgorithm, bool) {
	for index, resample := range game.resamples {
		if pointInRect(mouseX, mouseY, game.resampleOptionRect(index)) {
			return resample, true
		}
	}
	return "", false
}

func (game *Game) sharpenMenuSelection(mouseX, mouseY int) (dither.SharpenAlgorithm, bool) {
	for index, sharpen := range game.sharpens {
		if pointInRect(mouseX, mouseY, game.sharpenOptionRect(index)) {
			return sharpen, true
		}
	}
	return "", false
}

func (game *Game) drawAlgorithmMenuOverlay(screen *ebiten.Image) {
	if !game.algorithmMenuOpen {
		return
	}
	for index, algorithm := range game.algorithms {
		rect := game.algorithmOptionRect(index)
		active := algorithm == game.algorithm
		drawMenuOption(screen, rect, string(algorithm), active)
	}
}

func (game *Game) drawPaletteMenuOverlay(screen *ebiten.Image) {
	if !game.paletteMenuOpen {
		return
	}
	for index, option := range game.paletteAlgorithms {
		rect := game.paletteOptionRect(index)
		active := option == game.params.PaletteAlgorithm
		drawMenuOption(screen, rect, string(option), active)
	}
}

func (game *Game) drawPaletteModeOverlay(screen *ebiten.Image) {
	if !game.paletteModeOpen {
		return
	}
	for index, option := range game.paletteModes {
		rect := game.paletteModeOptionRect(index)
		active := option == game.params.PaletteMode
		drawMenuOption(screen, rect, string(option), active)
	}
}

func (game *Game) drawResampleMenuOverlay(screen *ebiten.Image) {
	if !game.resampleMenuOpen {
		return
	}
	for index, option := range game.resamples {
		rect := game.resampleOptionRect(index)
		active := option == game.params.Resample
		drawMenuOption(screen, rect, string(option), active)
	}
}

func (game *Game) drawSharpenMenuOverlay(screen *ebiten.Image) {
	if !game.sharpenMenuOpen {
		return
	}
	for index, option := range game.sharpens {
		rect := game.sharpenOptionRect(index)
		active := option == game.params.SharpenAlgorithm
		drawMenuOption(screen, rect, string(option), active)
	}
}

func drawMenuOption(screen *ebiten.Image, rect image.Rectangle, text string, active bool) {
	background := color.RGBA{R: 43, G: 49, B: 59, A: 255}
	if active {
		background = color.RGBA{R: 63, G: 84, B: 120, A: 255}
	}
	ebitenutil.DrawRect(screen, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Dx()), float64(rect.Dy()), background)
	vector.StrokeRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), 1, color.RGBA{R: 130, G: 140, B: 156, A: 255}, false)
	ebitenutil.DebugPrintAt(screen, text, rect.Min.X+10, rect.Min.Y+10)
}

func (game *Game) anyMenuOpen() bool {
	return game.algorithmMenuOpen || game.paletteMenuOpen || game.paletteModeOpen || game.resampleMenuOpen || game.sharpenMenuOpen
}

func normalizedToLevels(normalized float64) int {
	step := int(math.Round(normalized * 5.0))
	if step < 0 {
		step = 0
	}
	if step > 5 {
		step = 5
	}
	return 1 << (step + 1)
}

func levelsToNormalized(levels int) float64 {
	switch levels {
	case 2:
		return 0.0
	case 4:
		return 1.0 / 5.0
	case 8:
		return 2.0 / 5.0
	case 16:
		return 3.0 / 5.0
	case 32:
		return 4.0 / 5.0
	case 64:
		return 1.0
	default:
		return 0.0
	}
}

func normalizedToPaletteInterval(normalized float64) int {
	step := int(math.Round(normalized * 5.0))
	if step < 0 {
		step = 0
	}
	if step > 5 {
		step = 5
	}
	return 1 << step
}

func paletteIntervalToNormalized(interval int) float64 {
	switch interval {
	case 1:
		return 0.0
	case 2:
		return 1.0 / 5.0
	case 4:
		return 2.0 / 5.0
	case 8:
		return 3.0 / 5.0
	case 16:
		return 4.0 / 5.0
	case 32:
		return 1.0
	default:
		return 0.0
	}
}

func (game *Game) algorithmPreviewLabel() string {
	if game.params.UseColor {
		return "Quantized Preview"
	}
	if game.algorithmUsesDiffusionControl() {
		return "Diffusion Buffer"
	}
	return "Grayscale Preview"
}

func (game *Game) algorithmUsesDiffusionControl() bool {
	switch game.algorithm {
	case dither.AlgorithmFloyd, dither.AlgorithmAtkinson, dither.AlgorithmJarvisJudiceNinke, dither.AlgorithmSierra, dither.AlgorithmBlueNoiseHybrid:
		return true
	default:
		return false
	}
}

func (game *Game) algorithmUsesOrderedControl() bool {
	return game.algorithm == dither.AlgorithmBayer
}

func (game *Game) algorithmUsesBlueNoiseControl() bool {
	switch game.algorithm {
	case dither.AlgorithmBlueNoiseThreshold, dither.AlgorithmBlueNoiseHybrid:
		return true
	default:
		return false
	}
}

func (game *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func pointInRect(x, y int, rect image.Rectangle) bool {
	return x >= rect.Min.X && x < rect.Max.X && y >= rect.Min.Y && y < rect.Max.Y
}

func drawImageIntoRect(target *ebiten.Image, source *ebiten.Image, rect image.Rectangle) {
	if source == nil || rect.Dx() <= 0 || rect.Dy() <= 0 {
		return
	}

	sourceBounds := source.Bounds()
	scaleX := float64(rect.Dx()) / float64(sourceBounds.Dx())
	scaleY := float64(rect.Dy()) / float64(sourceBounds.Dy())

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scaleX, scaleY)
	op.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
	target.DrawImage(source, op)
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}
