package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// MaxLogTail bounds how many past lines a log view starts with.
const MaxLogTail = 10000

// demuxReader turns Docker's multiplexed stream (8-byte header: stream type,
// three zero bytes, big-endian payload size) into the plain concatenated
// output. stdout and stderr are interleaved in arrival order.
type demuxReader struct {
	r      io.Reader
	remain int64
}

// Demux reads a non-TTY container stream (logs, attach, exec without TTY).
func Demux(r io.Reader) io.Reader { return &demuxReader{r: r} }

func (d *demuxReader) Read(p []byte) (int, error) {
	for d.remain == 0 {
		var hdr [8]byte
		if _, err := io.ReadFull(d.r, hdr[:]); err != nil {
			if err == io.ErrUnexpectedEOF {
				return 0, io.EOF // a cut-off header: the stream was closed
			}
			return 0, err
		}
		if hdr[0] > 2 || hdr[1] != 0 || hdr[2] != 0 || hdr[3] != 0 {
			return 0, fmt.Errorf("docker: unexpected log stream header % x", hdr[:4])
		}
		d.remain = int64(binary.BigEndian.Uint32(hdr[4:]))
	}
	if int64(len(p)) > d.remain {
		p = p[:d.remain]
	}
	n, err := d.r.Read(p)
	d.remain -= int64(n)
	return n, err
}

// crlfReader makes bare \n into \r\n for a terminal emulator.
type crlfReader struct {
	r      io.Reader
	prevCR bool
	buf    []byte
}

func (c *crlfReader) Read(p []byte) (int, error) {
	for len(c.buf) == 0 {
		in := make([]byte, max(1, len(p)/2))
		n, err := c.r.Read(in)
		out := make([]byte, 0, n*2)
		for _, b := range in[:n] {
			if b == '\n' && !c.prevCR {
				out = append(out, '\r')
			}
			c.prevCR = b == '\r'
			out = append(out, b)
		}
		c.buf = out
		if len(c.buf) == 0 {
			return 0, err
		}
		if err != nil && err != io.EOF {
			return 0, err
		}
	}
	n := copy(p, c.buf)
	c.buf = c.buf[n:]
	return n, nil
}

type logStream struct {
	io.Reader
	body io.Closer
	stop context.CancelFunc
}

func (l *logStream) Close() error {
	l.stop()
	return l.body.Close()
}

// Logs follows a container's log from its last tail lines, as terminal-ready
// output (stdout and stderr merged, \r\n line ends). Close stops it.
func (d *Docker) Logs(ctx context.Context, id string, tail int) (io.ReadCloser, error) {
	if !validID(id) {
		return nil, fmt.Errorf("invalid container id %q", id)
	}
	tail = max(0, min(tail, MaxLogTail))
	details, err := d.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	q := url.Values{"stdout": {"1"}, "stderr": {"1"}, "follow": {"1"}, "tail": {strconv.Itoa(tail)}}
	resp, err := d.request(ctx, http.MethodGet, "/containers/"+id+"/logs", q, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	var r io.Reader = resp.Body
	if !details.TTY {
		r = Demux(r)
	}
	return &logStream{Reader: &crlfReader{r: r}, body: resp.Body, stop: cancel}, nil
}
