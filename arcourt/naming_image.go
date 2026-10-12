package arcourt

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"image/png"

	pdf "github.com/giraffesyo/pdf"
)

const (
	maxNamingImages            = 2
	maxNamingImagePixels       = 12 << 20
	maxNamingImageDimension    = 2048
	maxNamingImageEncodedBytes = 2 << 20
	minNamingImageDimension    = 256
)

func extractNamingPageImages(ctx context.Context, pages []pdf.Page) []NamingImage {
	images := make([]NamingImage, 0, min(len(pages), maxNamingImages))
	for _, page := range pages {
		if len(images) == maxNamingImages || ctx.Err() != nil {
			break
		}
		source, ok := largestNamingPageImage(page)
		if !ok {
			continue
		}
		decoded, err := source.Decode()
		if err != nil || ctx.Err() != nil {
			continue
		}
		if encoded, ok := encodeNamingImage(decoded); ok {
			images = append(images, encoded)
		}
	}
	return images
}

func largestNamingPageImage(page pdf.Page) (pdf.Image, bool) {
	var selected pdf.Image
	var selectedArea float64
	var selectedPixels int
	pageBounds := page.CropBox
	if pageBounds.MaxX <= pageBounds.MinX || pageBounds.MaxY <= pageBounds.MinY {
		pageBounds = page.MediaBox
	}
	pageArea := (pageBounds.MaxX - pageBounds.MinX) * (pageBounds.MaxY - pageBounds.MinY)
	for _, candidate := range page.Images {
		if candidate.Width < minNamingImageDimension || candidate.Height < minNamingImageDimension ||
			candidate.Height > 0 && candidate.Width > maxNamingImagePixels/candidate.Height {
			continue
		}
		bounds := candidate.Bounds()
		paintedArea := (bounds.MaxX - bounds.MinX) * (bounds.MaxY - bounds.MinY)
		if paintedArea < 0 {
			paintedArea = -paintedArea
		}
		if pageArea > 0 && paintedArea < pageArea/4 {
			continue
		}
		pixels := candidate.Width * candidate.Height
		if paintedArea > selectedArea || paintedArea == selectedArea && pixels > selectedPixels {
			selected, selectedArea, selectedPixels = candidate, paintedArea, pixels
		}
	}
	return selected, selectedPixels > 0
}

func encodeNamingImage(source image.Image) (NamingImage, bool) {
	resized := resizeNamingImage(source, maxNamingImageDimension)
	if isGrayNamingImage(resized) {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, resized); err == nil && encoded.Len() <= maxNamingImageEncodedBytes {
			return NamingImage{MediaType: "image/png", Data: encoded.Bytes()}, true
		}
	}
	for _, attempt := range []struct {
		dimension int
		quality   int
	}{
		{maxNamingImageDimension, 85},
		{1536, 75},
		{1024, 65},
	} {
		candidate := resizeNamingImage(source, attempt.dimension)
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, candidate, &jpeg.Options{Quality: attempt.quality}); err == nil && encoded.Len() <= maxNamingImageEncodedBytes {
			return NamingImage{MediaType: "image/jpeg", Data: encoded.Bytes()}, true
		}
	}
	return NamingImage{}, false
}

func resizeNamingImage(source image.Image, maximum int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maximum && height <= maximum {
		return source
	}
	scale := float64(maximum) / float64(max(width, height))
	target := image.NewRGBA(image.Rect(0, 0, max(1, int(float64(width)*scale)), max(1, int(float64(height)*scale))))
	targetWidth, targetHeight := target.Bounds().Dx(), target.Bounds().Dy()
	for y := range targetHeight {
		sourceY := bounds.Min.Y + y*height/targetHeight
		for x := range targetWidth {
			sourceX := bounds.Min.X + x*width/targetWidth
			target.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return target
}

func isGrayNamingImage(source image.Image) bool {
	switch source.(type) {
	case *image.Gray, *image.Gray16:
		return true
	default:
		return false
	}
}
