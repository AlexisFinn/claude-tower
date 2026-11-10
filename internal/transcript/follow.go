package transcript

import (
	"bytes"
	"errors"
	"io"
	"os"
)

const readChunk = 256 << 10

// Follower reads the lines that a transcript gained since the previous read.
// The zero value is ready to use. It is not safe for concurrent use.
type Follower struct {
	path    string
	info    os.FileInfo
	off     int64
	partial []byte
	buf     []byte
}

// Read passes line each complete line that the transcript at path gained since
// the last call; a trailing line without its newline waits for the next call.
// A transcript that shrank, was replaced by another file, or moved starts a
// new read from the beginning, after reset is called. A failed read keeps the
// position known so far. The line slice is only valid during the call.
func (f *Follower) Read(path string, reset func(), line func([]byte)) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return err
	}
	if f.path != path || (f.info != nil && !os.SameFile(f.info, info)) || info.Size() < f.off {
		f.info, f.off, f.partial = nil, 0, nil
		reset()
	}
	f.path, f.info = path, info
	if info.Size() == f.off {
		return nil
	}
	if _, err := fh.Seek(f.off, io.SeekStart); err != nil {
		return err
	}
	return f.consume(fh, line)
}

func (f *Follower) consume(r io.Reader, line func([]byte)) error {
	if f.buf == nil {
		f.buf = make([]byte, readChunk)
	}
	for {
		n, err := r.Read(f.buf)
		if n > 0 {
			f.off += int64(n)
			f.feed(f.buf[:n], line)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// feed passes the complete lines of data, after the partial line kept from the
// previous read, and keeps the trailing incomplete line.
func (f *Follower) feed(data []byte, line func([]byte)) {
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			f.partial = append(f.partial, data...)
			return
		}
		l := data[:i]
		if len(f.partial) > 0 {
			l = append(f.partial, l...)
			f.partial = nil
		}
		line(l)
		data = data[i+1:]
	}
}
