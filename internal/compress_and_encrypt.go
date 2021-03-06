package internal

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/wal-g/wal-g/internal/compression"
	"github.com/wal-g/wal-g/internal/tracelog"
	"github.com/wal-g/wal-g/utility"
	"io"
)

// CompressAndEncryptError is used to catch specific errors from CompressAndEncrypt
// when uploading to Storage. Will not retry upload if this error occurs.
type CompressAndEncryptError struct {
	error
}

func NewCompressingPipeWriterError(reason string) CompressAndEncryptError {
	return CompressAndEncryptError{errors.New(reason)}
}

func (err CompressAndEncryptError) Error() string {
	return fmt.Sprintf(tracelog.GetErrorFormatter(), err.error)
}

// CompressAndEncrypt compresses input to a pipe reader. Output must be used or
// pipe will block.
func CompressAndEncrypt(source io.Reader, compressor compression.Compressor, crypter Crypter) io.Reader {
	compressedReader, dstWriter := io.Pipe()

	var writeCloser io.WriteCloser = dstWriter
	if crypter.IsUsed() {
		var err error
		writeCloser, err = crypter.Encrypt(dstWriter)

		if err != nil {
			panic(err)
		}
	}

	writeIgnorer := &EmptyWriteIgnorer{writeCloser}
	lzWriter := compressor.NewWriter(writeIgnorer)

	go func() {
		_, err := utility.FastCopy(lzWriter, source)

		if err != nil {
			e := NewCompressingPipeWriterError("CompressAndEncrypt: compression failed")
			dstWriter.CloseWithError(e)
		}

		if err := lzWriter.Close(); err != nil {
			e := NewCompressingPipeWriterError("CompressAndEncrypt: writer close failed")
			dstWriter.CloseWithError(e)
			return
		}
		if crypter.IsUsed() {
			err := writeCloser.Close()

			if err != nil {
				e := NewCompressingPipeWriterError("CompressAndEncrypt: encryption failed")
				dstWriter.CloseWithError(e)
				return
			}
		}
		if err = dstWriter.Close(); err != nil {
			e := NewCompressingPipeWriterError("CompressAndEncrypt: pipe writer close failed")
			dstWriter.CloseWithError(e)
		}
	}()
	return compressedReader
}
