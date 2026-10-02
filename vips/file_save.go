package vips

/*
#include "stream.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"strings"
	"unsafe"
)

// SaveToFileTiff encodes directly to a seekable file, without the encoded TIFF
// buffer used by ExportTiff/SaveToWriterTiff. Options share the same mapping.
// The filename is literal (libvips option syntax is not interpreted). The
// caller owns the destination and must discard partial output on error.
func (r *ImageRef) SaveToFileTiff(filename string, params *TiffExportParams) error {
	if filename == "" || strings.IndexByte(filename, 0) >= 0 {
		return errors.New("TIFF output filename is invalid")
	}
	if params == nil {
		params = NewTiffExportParams()
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	defer runtime.KeepAlive(r)
	if r.image == nil {
		return errors.New("attempt to save a closed ImageRef")
	}
	path := C.CString(filename)
	defer C.free(unsafe.Pointer(path))
	saveParams := newSaveParamsTIFF(r.image, *params)
	incOpCounter("save_tiff_file")
	if C.save_tiff_to_file(&saveParams, path) != 0 {
		err := handleVipsError()
		if r.streamSource != nil {
			return wrapStreamError("TIFF file save", err, r.streamSource.entry.takeErr())
		}
		return err
	}
	return nil
}

// SaveToFilePnm writes binary PNM samples directly to a file, without the raw
// frame buffer used by ToBytes. The native saver owns encode/close errors.
// The caller owns the literal destination and discards partial output on error.
func (r *ImageRef) SaveToFilePnm(filename string) error {
	return r.saveSamplesToFile(filename, false)
}

// SaveToFileRaw writes the same native, interleaved sample layout as ToBytes,
// without its C frame allocation and Go copy. Dimensions/band format remain
// the caller's explicit contract; the output contains no container metadata.
func (r *ImageRef) SaveToFileRaw(filename string) error {
	return r.saveSamplesToFile(filename, true)
}

func (r *ImageRef) saveSamplesToFile(filename string, raw bool) error {
	if filename == "" || strings.IndexByte(filename, 0) >= 0 {
		return errors.New("sample output filename is invalid")
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	defer runtime.KeepAlive(r)
	if r.image == nil {
		return errors.New("attempt to save a closed ImageRef")
	}
	path := C.CString(filename)
	defer C.free(unsafe.Pointer(path))
	var failed C.int
	if raw {
		incOpCounter("save_raw_file")
		failed = C.save_raw_to_file(r.image, path)
	} else {
		incOpCounter("save_pnm_file")
		failed = C.save_pnm_to_file(r.image, path)
	}
	if failed != 0 {
		err := handleVipsError()
		if r.streamSource != nil {
			return wrapStreamError("sample file save", err, r.streamSource.entry.takeErr())
		}
		return err
	}
	return nil
}
