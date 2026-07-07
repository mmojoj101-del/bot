package smpp

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// SMPPTransport abstracts the underlying TCP connection for the SMPP driver.
//
// Today: TCP. Testing: fakeTransport. Tomorrow: TLS, SOCKS, or WebSocket.
// The abstraction exists so the entire SMPP Session Engine can be tested
// without a real socket.
//
// Concurrency contract:
//   - ReadPDU must be called from a single goroutine (the Reader).
//   - WritePDU is safe for concurrent calls (mutex-protected in tcpTransport).
//   - Close is safe for concurrent calls and unblocks any in-flight Read/Write.
//
// The fakeTransport in tests should support at minimum:
//   - Normal request/response (happy path)
//   - Delayed responses (simulate SMSC processing time)
//   - Partial reads/writes (TCP fragmentation)
//   - EOF (graceful close)
//   - Connection reset (network failure)
//   - Malformed PDU bytes (corruption)
//   - Timeout on ReadPDU (idle timeout)
type SMPPTransport interface {
	// ReadPDU reads one complete PDU from the transport.
	// Returns the raw bytes (including the 16-byte header).
	// Must block until a full PDU is received or ctx is cancelled.
	ReadPDU(ctx context.Context) ([]byte, error)

	// WritePDU sends a complete PDU (including header) to the transport.
	// Must be safe for concurrent calls.
	WritePDU(ctx context.Context, data []byte) error

	// Close closes the transport and unblocks any in-flight Read/Write.
	Close() error
}

// tcpTransport implements SMPPTransport over a plain TCP connection.
type tcpTransport struct {
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex // protects write
}

// NewTCPTransport wraps a net.Conn as an SMPPTransport.
func NewTCPTransport(conn net.Conn) SMPPTransport {
	return &tcpTransport{
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
}

// ReadPDU reads one complete PDU from the TCP stream.
// It first reads the 4-byte length header, then reads the remaining body.
// Uses bufio.Reader for efficient buffered I/O.
func (t *tcpTransport) ReadPDU(ctx context.Context) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)

	go func() {
		// Read 4-byte length prefix
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(t.reader, lenBuf); err != nil {
			ch <- result{err: fmt.Errorf("read length: %w", err)}
			return
		}

		pduLen := binary.BigEndian.Uint32(lenBuf)
		if pduLen < 16 {
			ch <- result{err: fmt.Errorf("%w: length %d < 16", ErrMalformedPDU, pduLen)}
			return
		}

		// Read the rest of the PDU (length includes the 4-byte length field itself)
		body := make([]byte, pduLen)
		copy(body[0:4], lenBuf)
		if _, err := io.ReadFull(t.reader, body[4:]); err != nil {
			ch <- result{err: fmt.Errorf("read body: %w", err)}
			return
		}

		ch <- result{data: body}
	}()

	select {
	case r := <-ch:
		return r.data, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WritePDU sends a complete PDU to the TCP stream.
// Mutex-protected for concurrent call safety.
func (t *tcpTransport) WritePDU(ctx context.Context, data []byte) error {
	type result struct {
		err error
	}
	ch := make(chan result, 1)

	go func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		// Set write deadline if ctx has a deadline
		if deadline, ok := ctx.Deadline(); ok {
			t.conn.SetWriteDeadline(deadline)
		} else {
			t.conn.SetWriteDeadline(time.Time{})
		}
		_, err := t.conn.Write(data)
		ch <- result{err: err}
	}()

	select {
	case r := <-ch:
		return r.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close closes the underlying TCP connection.
func (t *tcpTransport) Close() error {
	return t.conn.Close()
}
