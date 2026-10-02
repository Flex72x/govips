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
