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
