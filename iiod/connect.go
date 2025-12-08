package iiod

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
)

// ----------------------------------------------------------------------
// Backend Interface (Polymorphic)
// ----------------------------------------------------------------------

type Backend interface {
	// Probe checks whether the backend supports this IIOD instance.
	// Should NOT modify state on failure.
	Probe(ctx context.Context, conn net.Conn) error

	// --- XML ---
	GetXMLContext(ctx context.Context) ([]byte, error)

	// --- Device Discovery ---
	ListDevices(ctx context.Context) ([]string, error)
	GetChannels(ctx context.Context, dev string) ([]string, error)

	// --- Attributes ---
	ReadAttr(ctx context.Context, dev, ch, attr string) (string, error)
	WriteAttr(ctx context.Context, dev, ch, attr, value string) error

	// --- Buffers ---
	OpenBuffer(ctx context.Context, dev string, samples int, cyclic bool) (int, error)
	ReadBuffer(ctx context.Context, bufID int, p []byte) (int, error)
	WriteBuffer(ctx context.Context, bufID int, p []byte) (int, error)
	CloseBuffer(ctx context.Context, bufID int) error

	// Shutdown backend (close local state)
	Close() error
}

// ----------------------------------------------------------------------
// Client
// ----------------------------------------------------------------------

type Client struct {
	uri     string
	conn    net.Conn
	backend Backend
}

// ----------------------------------------------------------------------
// Dial – binary-first probing, text fallback
// ----------------------------------------------------------------------

func Dial(ctx context.Context, uri string) (*Client, error) {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", uri)
	if err != nil {
		return nil, fmt.Errorf("connect to IIOD at %s: %w", uri, err)
	}

	c := &Client{
		uri:  uri,
		conn: conn,
	}

	// --- Probe Binary Backend First ---
	binBackend := NewBinaryBackend(conn)
	if err := binBackend.Probe(ctx, conn); err == nil {
		log.Printf("[IIOD] binary backend selected")
		c.backend = binBackend
		return c, nil
	}

	log.Printf("[IIOD] binary backend rejected (%v), falling back to text mode", err)

	// --- Fallback to Text Backend ---
	textBackend := NewTextBackend(conn)
	if err := textBackend.Probe(ctx, conn); err != nil {
		return nil, fmt.Errorf("text backend probe also failed: %w", err)
	}

	log.Printf("[IIOD] text backend selected (Pluto-style IIOD v0.25)")
	c.backend = textBackend
	return c, nil
}

// ----------------------------------------------------------------------
// Shutdown
// ----------------------------------------------------------------------

func (c *Client) Close() error {
	if c.backend != nil {
		_ = c.backend.Close()
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ----------------------------------------------------------------------
// XML Context
// ----------------------------------------------------------------------

func (c *Client) GetXMLContext(ctx context.Context) ([]byte, error) {
	if c.backend == nil {
		return nil, errors.New("client backend not initialized")
	}
	return c.backend.GetXMLContext(ctx)
}

// ----------------------------------------------------------------------
// Device Handling
// ----------------------------------------------------------------------

func (c *Client) ListDevices(ctx context.Context) ([]string, error) {
	if c.backend == nil {
		return nil, errors.New("client backend not initialized")
	}
	return c.backend.ListDevices(ctx)
}

func (c *Client) GetChannels(ctx context.Context, dev string) ([]string, error) {
	if c.backend == nil {
		return nil, errors.New("client backend not initialized")
	}
	return c.backend.GetChannels(ctx, dev)
}

// ----------------------------------------------------------------------
// Attribute Access
// ----------------------------------------------------------------------

func (c *Client) ReadAttr(ctx context.Context, dev, ch, attr string) (string, error) {
	if c.backend == nil {
		return "", errors.New("client backend not initialized")
	}
	return c.backend.ReadAttr(ctx, dev, ch, attr)
}

func (c *Client) WriteAttr(ctx context.Context, dev, ch, attr, value string) error {
	if c.backend == nil {
		return errors.New("client backend not initialized")
	}
	return c.backend.WriteAttr(ctx, dev, ch, attr, value)
}

// ----------------------------------------------------------------------
// Buffer Handling
// ----------------------------------------------------------------------

func (c *Client) OpenBuffer(ctx context.Context, dev string, samples int, cyclic bool) (int, error) {
	if c.backend == nil {
		return 0, errors.New("client backend not initialized")
	}
	return c.backend.OpenBuffer(ctx, dev, samples, cyclic)
}

func (c *Client) ReadBuffer(ctx context.Context, bufID int, p []byte) (int, error) {
	if c.backend == nil {
		return 0, errors.New("client backend not initialized")
	}
	return c.backend.ReadBuffer(ctx, bufID, p)
}

func (c *Client) WriteBuffer(ctx context.Context, bufID int, p []byte) (int, error) {
	if c.backend == nil {
		return 0, errors.New("client backend not initialized")
	}
	return c.backend.WriteBuffer(ctx, bufID, p)
}

func (c *Client) CloseBuffer(ctx context.Context, bufID int) error {
	if c.backend == nil {
		return errors.New("client backend not initialized")
	}
	return c.backend.CloseBuffer(ctx, bufID)
}
