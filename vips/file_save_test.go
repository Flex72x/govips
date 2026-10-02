package vips

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestSaveToFileTiffOptionsAndLiteralFilename(t *testing.T) {
	image, err := NewImageFromBuffer(nativeFileFixture(t, "png"))
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	if image.HasAlpha() {
		if err := image.Flatten(&Color{R: 255, G: 255, B: 255}); err != nil {
			t.Fatal(err)
		}
	}
	for _, compression := range []TiffCompression{TiffCompressionNone, TiffCompressionLzw, TiffCompressionJpeg} {
		params := NewTiffExportParams()
		params.Compression = compression
		params.Quality = 73
		path := filepath.Join(t.TempDir(), "result[compression=none].tif")
		if err := image.SaveToFileTiff(path, params); err != nil {
			t.Fatal(err)
		}
		buffer, _, err := image.ExportTiff(params)
		if err != nil {
			t.Fatal(err)
		}
		fromFile, err := LoadImageFromFile(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		fromBuffer, err := LoadImageFromBuffer(buffer, nil)
		if err != nil {
			fromFile.Close()
			t.Fatal(err)
		}
		filePixels, _, fileErr := fromFile.ExportPng(nil)
		bufferPixels, _, bufferErr := fromBuffer.ExportPng(nil)
		fromFile.Close()
		fromBuffer.Close()
		if fileErr != nil || bufferErr != nil || !bytes.Equal(filePixels, bufferPixels) {
			t.Fatalf("file and buffer TIFF differ: %v / %v", fileErr, bufferErr)
		}
	}
	if err := image.SaveToFileTiff(filepath.Join(t.TempDir(), "absent", "out.tif"), nil); err == nil {
		t.Fatal("missing destination directory succeeded")
	}
	image.Close()
	if err := image.SaveToFileTiff(filepath.Join(t.TempDir(), "closed.tif"), nil); err == nil {
		t.Fatal("closed image succeeded")
	}
}

func TestMaterializeNativeFileDetachesSourceAndUsesScratch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.png")
	if err := os.WriteFile(path, nativeFileFixture(t, "png"), 0600); err != nil {
		t.Fatal(err)
	}
	params := NewImportParams()
	params.Access.Set(AccessSequential)
	image, err := LoadImageFromFile(path, params)
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	scratch := t.TempDir()
	SetStreamDiscThreshold(0)
	SetStreamScratchDir(scratch)
	defer SetStreamDiscThreshold(-1)
	defer SetStreamScratchDir("")
	if err := image.Materialize(); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	if err := image.Flip(DirectionVertical); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, _, err := image.ExportPng(nil); err != nil {
			t.Fatalf("materialized image still needs source: %v", err)
		}
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatalf("scratch was not unlinked: entries=%d err=%v", len(entries), err)
	}
}

type evaluationBlockingWriter struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *evaluationBlockingWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(p), nil
}

func TestSetKillDuringStreamingEvaluationDoesNotWaitForWriter(t *testing.T) {
	image, err := NewImageFromBuffer(nativeFileFixture(t, "png"))
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	writer := &evaluationBlockingWriter{started: make(chan struct{}), release: make(chan struct{})}
	saved := make(chan error, 1)
	go func() { saved <- image.SaveToWriterPng(writer, nil) }()
	<-writer.started
	killed := make(chan struct{})
	go func() {
		image.SetKill(true)
		close(killed)
	}()
	var timedOut bool
	select {
	case <-killed:
	case <-time.After(time.Second):
		timedOut = true
	}
	close(writer.release)
	<-saved
	<-killed
	if timedOut {
		t.Fatal("SetKill waited for the streaming encoder lock")
	}
	// Evaluation may have finished all pixels before the writer was paused.
	// The regression is cancellation reaching the native flag, not output size.
	image.SetKill(false)
	if err := image.SaveToWriterPng(io.Discard, nil); err != nil {
		t.Fatal(err)
	}
}
