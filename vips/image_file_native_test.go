package vips

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	stdjpeg "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func nativeFileFixture(t *testing.T, format string) []byte {
	t.Helper()
	pixels := image.NewNRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			pixels.SetNRGBA(x, y, color.NRGBA{uint8(x * 7), uint8(y * 9), 63, 255})
		}
	}
	var encoded bytes.Buffer
	var err error
	switch format {
	case "svg":
		return []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="32" height="24">` + strings.Repeat(`<rect width="32" height="24" fill="red"/>`, 200) + `</svg>`)
	case "jpeg":
		err = stdjpeg.Encode(&encoded, pixels, &stdjpeg.Options{Quality: 85})
	case "gif":
		palette := color.Palette{color.Black, color.White}
		first := image.NewPaletted(pixels.Bounds(), palette)
		second := image.NewPaletted(pixels.Bounds(), palette)
		second.SetColorIndex(4, 4, 1)
		err = gif.EncodeAll(&encoded, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{7, 13}, LoopCount: 3})
	default:
		err = png.Encode(&encoded, pixels)
	}
	if err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestNativeFileLoadParity(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif", "svg"} {
		t.Run(format, func(t *testing.T) {
			source := nativeFileFixture(t, format)
			// Wrong extension and literal bracket syntax must not change decoding.
			path := filepath.Join(t.TempDir(), "upload[page=99].wrong")
			if err := os.WriteFile(path, source, 0600); err != nil {
				t.Fatal(err)
			}
			params := NewImportParams()
			params.Access.Set(AccessRandom)
			params.AutoRotate.Set(true)
			if format == "gif" {
				params.NumPages.Set(-1)
			}
			if format == "jpeg" {
				params.JpegShrinkFactor.Set(2)
			}
			fromFile, err := LoadImageFromFile(path, params)
			if err != nil {
				t.Fatal(err)
			}
			defer fromFile.Close()
			fromBuffer, err := LoadImageFromBuffer(source, params)
			if err != nil {
				t.Fatal(err)
			}
			defer fromBuffer.Close()
			if *fromFile.Metadata() != *fromBuffer.Metadata() {
				t.Fatalf("metadata mismatch: %+v / %+v", fromFile.Metadata(), fromBuffer.Metadata())
			}
			filePixels, _, err := fromFile.ExportPng(NewPngExportParams())
			if err != nil {
				t.Fatal(err)
			}
			bufferPixels, _, err := fromBuffer.ExportPng(NewPngExportParams())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(filePixels, bufferPixels) {
				t.Fatal("file and buffer pixels differ")
			}
			if format == "gif" {
				fileGIF, _, err := fromFile.ExportGIF(NewGifExportParams())
				if err != nil {
					t.Fatal(err)
				}
				bufferGIF, _, err := fromBuffer.ExportGIF(NewGifExportParams())
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(fileGIF, bufferGIF) {
					t.Fatal("animation metadata changed")
				}
			}
		})
	}
}

func TestNativeFileLoadDoesNotBufferEncodedFile(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "upload")
			if err := os.WriteFile(path, nativeFileFixture(t, format), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(path, 128<<20); err != nil {
				t.Fatal(err)
			}
			if err := startupIfNeeded(); err != nil {
				t.Fatal(err)
			}
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			loaded, err := LoadImageFromFile(path, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = loaded.ExportPng(NewPngExportParams())
			loaded.Close()
			runtime.ReadMemStats(&after)
			if err != nil {
				t.Fatal(err)
			}
			if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 16<<20 {
				t.Fatalf("buffered encoded file: %d Go bytes allocated", allocated)
			}
		})
	}
}

func TestNativeFileLoadErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	if image, err := LoadImageFromFile(path, nil); err == nil || image != nil {
		t.Fatal("missing file accepted")
	}
	if err := os.WriteFile(path, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	if image, err := LoadImageFromFile(path, nil); err == nil || image != nil {
		t.Fatal("invalid image accepted")
	}
	if err := os.WriteFile(path, []byte("\x89PNG\r\n\x1a\ntruncated"), 0600); err != nil {
		t.Fatal(err)
	}
	if image, err := LoadImageFromFile(path, nil); err == nil || image != nil {
		t.Fatal("truncated image accepted")
	}
}
