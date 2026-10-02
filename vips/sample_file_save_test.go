package vips

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func readPnmToken(reader *bufio.Reader) (string, error) {
	var token strings.Builder
	for {
		value, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		if value == '#' && token.Len() == 0 {
			if _, err := reader.ReadString('\n'); err != nil {
				return "", err
			}
			continue
		}
		if value == ' ' || value == '\n' || value == '\r' || value == '\t' {
			if token.Len() != 0 {
				return token.String(), nil
			}
			continue
		}
		token.WriteByte(value)
	}
}

func TestNativeSampleFileSaversPreserveRawAndPnmSamples(t *testing.T) {
	for _, bands := range []int{1, 3} {
		t.Run(fmt.Sprint(bands), func(t *testing.T) {
			pixels := make([]byte, 7*5*bands)
			for index := range pixels {
				pixels[index] = byte(index * 13)
			}
			interpretation := InterpretationBW
			if bands == 3 {
				interpretation = InterpretationSRGB
			}
			img, err := NewImageFromMemory(pixels, 7, 5, bands, BandFormatUchar, interpretation)
			if err != nil {
				t.Fatal(err)
			}
			defer img.Close()
			raw, pnm := filepath.Join(t.TempDir(), "samples[raw].bin"), filepath.Join(t.TempDir(), "samples.pnm")
			if err := img.SaveToFileRaw(raw); err != nil {
				t.Fatal(err)
			}
			actual, err := os.ReadFile(raw)
			if err != nil || !bytes.Equal(actual, pixels) {
				t.Fatalf("raw sample mismatch: %v", err)
			}
			if err := img.SaveToFilePnm(pnm); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(pnm)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			reader := bufio.NewReader(file)
			magic := "P5"
			if bands == 3 {
				magic = "P6"
			}
			for _, expected := range []string{magic, "7", "5", "255"} {
				value, err := readPnmToken(reader)
				if err != nil || value != expected {
					t.Fatalf("PNM header: %q expected %q err=%v", value, expected, err)
				}
			}
			actual, err = io.ReadAll(reader)
			if err != nil || !bytes.Equal(actual, pixels) {
				t.Fatalf("PNM samples changed: %v", err)
			}
			if err := img.SaveToFileRaw(filepath.Join(t.TempDir(), "missing", "output")); err == nil {
				t.Fatal("invalid sink accepted")
			}
		})
	}
}

func TestNativeSampleFileSaverAvoidsFullGoFrameAllocation(t *testing.T) {
	img, err := Black(10001, 8000)
	if err != nil {
		t.Fatal(err)
	}
	defer img.Close()
	path := filepath.Join(t.TempDir(), "samples.pgm")
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err = img.SaveToFilePnm(path)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Size() < 80_008_000 {
		t.Fatalf("output frame missing: %v", err)
	}
	if after.TotalAlloc-before.TotalAlloc > 4<<20 {
		t.Fatalf("raw samples copied to Go: %d", after.TotalAlloc-before.TotalAlloc)
	}
	img.Close()
	if err := img.SaveToFilePnm(path); err == nil {
		t.Fatal("closed image accepted")
	}
}

func TestNativePnmFileBufferAndReaderHaveSameSamples(t *testing.T) {
	for _, header := range []string{"P5\n7 5\n255\n", "P6\n7 5\n255\n", "P5\n7 5\n65535\n", "P6\n7 5\n65535\n"} {
		bands, depth := 1, 1
		if header[1] == '6' {
			bands = 3
		}
		if strings.Contains(header, "65535") {
			depth = 2
		}
		pixels := make([]byte, 7*5*bands*depth)
		for i := range pixels {
			pixels[i] = byte(i * 17)
		}
		encoded := append([]byte(header), pixels...)
		path := filepath.Join(t.TempDir(), "not-pnm[file].bin")
		if err := os.WriteFile(path, encoded, 0600); err != nil {
			t.Fatal(err)
		}
		file, err := LoadImageFromFile(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		buffered, err := NewImageFromBuffer(encoded)
		if err != nil {
			file.Close()
			t.Fatal(err)
		}
		streamed, err := LoadImageFromReader(bytes.NewReader(encoded), nil)
		if err != nil {
			file.Close()
			buffered.Close()
			t.Fatal(err)
		}
		expected, err := buffered.ToBytes()
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range []*ImageRef{file, streamed} {
			actual, err := im.ToBytes()
			if err != nil || !bytes.Equal(actual, expected) || im.Format() != ImageTypePNM || im.Width() != 7 || im.Height() != 5 || im.Bands() != bands {
				t.Fatalf("PNM %s mismatch: %v", header, err)
			}
		}
		file.Close()
		buffered.Close()
		streamed.Close()
	}
}
