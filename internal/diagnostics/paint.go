package diagnostics

import "image"

// blankTolerance is the per-channel distance, in 8-bit units, at which two
// sampled pixels still count as the same colour. The title bar is painted with
// a vertical gradient, so demanding an exact match would report a fully painted
// window as blank.
const blankTolerance = 2

// BlankRatio reports the share of sampled pixels that match the image's most
// common colour. A window that never painted is one flat colour, so the ratio
// approaches 1; anything carrying text, meters or icons falls well below it.
//
// The reference colour is derived from the image rather than passed in. The
// window background depends on theme, gradient and compositor alpha, so every
// caller that tried to name it in advance would name a different colour.
//
// Sampling every stride-th pixel on both axes keeps a full-window capture cheap
// enough to run on the UI thread. An empty or nil image returns 0, which reads
// as "not blank" and so never triggers a recovery on missing evidence.
func BlankRatio(img image.Image, stride int) float64 {
	if img == nil {
		return 0
	}
	bounds := img.Bounds()
	if bounds.Empty() {
		return 0
	}
	if stride < 1 {
		stride = 1
	}

	type rgb struct{ r, g, b uint8 }
	type bucket struct {
		count int
		first rgb
	}
	samples := make([]rgb, 0, 4096)
	buckets := make(map[uint32]bucket)
	var reference rgb
	best := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stride {
		for x := bounds.Min.X; x < bounds.Max.X; x += stride {
			r16, g16, b16, _ := img.At(x, y).RGBA()
			sample := rgb{uint8(r16 >> 8), uint8(g16 >> 8), uint8(b16 >> 8)}
			samples = append(samples, sample)
			// Quantise to 6 bits per channel so near-identical gradient steps
			// land in one bucket. The representative kept alongside the count
			// is a real sampled colour, not the bucket's floor, so the second
			// pass compares against something that actually appeared.
			key := uint32(sample.r>>2)<<16 | uint32(sample.g>>2)<<8 | uint32(sample.b>>2)
			entry, seen := buckets[key]
			if !seen {
				entry.first = sample
			}
			entry.count++
			buckets[key] = entry
			if entry.count > best {
				best, reference = entry.count, entry.first
			}
		}
	}
	if len(samples) == 0 {
		return 0
	}

	matched := 0
	for _, sample := range samples {
		if within(sample.r, reference.r) && within(sample.g, reference.g) && within(sample.b, reference.b) {
			matched++
		}
	}
	return float64(matched) / float64(len(samples))
}

func within(value, reference uint8) bool {
	if value > reference {
		return value-reference <= blankTolerance
	}
	return reference-value <= blankTolerance
}
