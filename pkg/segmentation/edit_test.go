package segmentation

import (
	"image"
	"image/color"
	"testing"

	"github.com/zamora322/vectorizador/pkg/palette"
)

func createTestSegmentMap() *SegmentMap {
	// 4x4 image with two color blocks: Left 2 cols = Red, Right 2 cols = Blue
	pal := palette.Palette{
		palette.RGB{R: 255, G: 0, B: 0},
		palette.RGB{R: 0, G: 0, B: 255},
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
		for x := 2; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	return SegmentImage(img, pal, 1, false)
}

func TestRecolorSegmentAt(t *testing.T) {
	sm := createTestSegmentMap()
	green := palette.RGB{R: 0, G: 255, B: 0}

	ok := sm.RecolorSegmentAt(0, 0, green)
	if !ok {
		t.Fatalf("RecolorSegmentAt failed")
	}

	segID := sm.At(0, 0)
	seg := sm.Segments[segID]
	if seg.Color != green {
		t.Errorf("Expected segment color %v, got %v", green, seg.Color)
	}
}

func TestMergeSegmentsAt(t *testing.T) {
	sm := createTestSegmentMap()

	idLeft := sm.At(0, 0)
	idRight := sm.At(3, 3)

	if idLeft == idRight {
		t.Fatalf("Expected distinct segments before merge")
	}

	ok := sm.MergeSegmentsAt(0, 0, 3, 3)
	if !ok {
		t.Fatalf("MergeSegmentsAt failed")
	}

	newIdLeft := sm.At(0, 0)
	newIdRight := sm.At(3, 3)

	if newIdLeft != newIdRight {
		t.Errorf("Expected identical segment IDs after merge, got %d and %d", newIdLeft, newIdRight)
	}

	if sm.Segments[newIdLeft].PixelCount != 16 {
		t.Errorf("Expected merged segment count 16, got %d", sm.Segments[newIdLeft].PixelCount)
	}
}

func TestEraseSegmentAt(t *testing.T) {
	sm := createTestSegmentMap()

	ok := sm.EraseSegmentAt(0, 0)
	if !ok {
		t.Fatalf("EraseSegmentAt failed")
	}

	segID := sm.At(0, 0)
	if !sm.Segments[segID].Transparent {
		t.Errorf("Expected segment to be marked transparent")
	}

	rgba := sm.ToRGBA()
	if rgba == nil {
		t.Errorf("Expected non-nil RGBA image")
	}
}

func TestCloneSegmentMap(t *testing.T) {
	sm := createTestSegmentMap()
	clone := sm.Clone()

	if clone.Width != sm.Width || clone.Height != sm.Height {
		t.Fatalf("Dimensions mismatch")
	}

	// Mutate original
	sm.RecolorSegmentAt(0, 0, palette.RGB{R: 50, G: 50, B: 50})

	// Clone should retain original color
	id := clone.At(0, 0)
	expected := palette.RGB{R: 255, G: 0, B: 0}
	if clone.Segments[id].Color != expected {
		t.Errorf("Clone was affected by mutation: expected %v, got %v", expected, clone.Segments[id].Color)
	}
}

func TestZapSegmentAt(t *testing.T) {
	// Create a 6x4 image with Red (left 2), Pink/Artifact (middle 2), Blue (right 2)
	red := palette.RGB{R: 255, G: 0, B: 0}
	pink := palette.RGB{R: 255, G: 120, B: 120} // Closer to Red than Blue
	blue := palette.RGB{R: 0, G: 0, B: 255}

	pal := palette.Palette{red, pink, blue}
	img := image.NewRGBA(image.Rect(0, 0, 6, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
		for x := 2; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 120, B: 120, A: 255})
		}
		for x := 4; x < 6; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	sm := SegmentImage(img, pal, 1, false)

	zapID := sm.At(2, 2)
	if zapID == -1 {
		t.Fatalf("Expected valid segment at (2, 2)")
	}

	ok := sm.ZapSegmentAt(2, 2)
	if !ok {
		t.Fatalf("ZapSegmentAt returned false")
	}

	// Destroyed segment must be deleted from Segments
	if _, exists := sm.Segments[zapID]; exists {
		t.Errorf("Target segment %d still exists in Segments map after Zap", zapID)
	}

	// Total pixel count across all segments must equal 24
	totalPixels := 0
	for _, seg := range sm.Segments {
		totalPixels += seg.PixelCount
	}
	if totalPixels != 24 {
		t.Errorf("Expected total pixels 24, got %d", totalPixels)
	}

	// Check that the zapped pixels were redistributed (closer to red, so (2, 2) should be absorbed)
	absorbedID := sm.At(2, 2)
	if absorbedID == zapID {
		t.Errorf("Pixel at (2, 2) still has zapped ID %d", zapID)
	}
}

func TestPencilStroke(t *testing.T) {
	sm := createTestSegmentMap() // 4x4 image
	green := palette.RGB{R: 0, G: 255, B: 0}

	// Draw a horizontal stroke through row 1 from x=0 to x=3 with width 1
	points := []Point{
		{X: 0, Y: 1},
		{X: 3, Y: 1},
	}

	ok := sm.PencilStroke(points, 1, green)
	if !ok {
		t.Fatalf("PencilStroke returned false")
	}

	// Verify all pixels along (0,1) to (3,1) have the green stroke
	strokeID := sm.At(0, 1)
	if strokeID == -1 {
		t.Fatalf("Expected valid strokeID at (0, 1)")
	}
	seg := sm.Segments[strokeID]
	if seg.Color != green {
		t.Errorf("Expected green color %v, got %v", green, seg.Color)
	}

	for x := 0; x < 4; x++ {
		if id := sm.At(x, 1); id != strokeID {
			t.Errorf("Pixel at (%d, 1) expected to have strokeID %d, got %d", x, strokeID, id)
		}
	}

	// Verify grid pixel conservation: total across segments should be 16
	total := 0
	for _, s := range sm.Segments {
		total += s.PixelCount
	}
	if total != 16 {
		t.Errorf("Expected 16 total pixels, got %d", total)
	}
}
