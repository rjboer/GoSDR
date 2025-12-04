package iiod

import (
	"fmt"
	"sync"
)

// ClientPool is a bounded pool of IIOD clients for reuse across callers.
//
// The pool lazily creates clients using the provided factory when needed and
// enforces a maximum size to avoid exhausting server resources.
type ClientPool struct {
	factory func() (*Client, error)
	pool    chan *Client
	once    sync.Once
	initErr error
}

// NewClientPool creates a new pool with the given size and factory.
func NewClientPool(size int, factory func() (*Client, error)) (*ClientPool, error) {
	if size <= 0 {
		return nil, fmt.Errorf("pool size must be positive")
	}
	if factory == nil {
		return nil, fmt.Errorf("factory is required")
	}

	return &ClientPool{factory: factory, pool: make(chan *Client, size)}, nil
}

// Get acquires a client from the pool, creating one if necessary.
func (p *ClientPool) Get() (*Client, error) {
	if p == nil {
		return nil, fmt.Errorf("pool is nil")
	}

	p.once.Do(func() {})
	if p.initErr != nil {
		return nil, p.initErr
	}

	select {
	case cli := <-p.pool:
		return cli, nil
	default:
	}

	cli, err := p.factory()
	if err != nil {
		p.initErr = err
		return nil, err
	}

	return cli, nil
}

// Put returns a client back to the pool or closes it if the pool is full.
func (p *ClientPool) Put(cli *Client) error {
	if p == nil {
		return fmt.Errorf("pool is nil")
	}
	if cli == nil {
		return fmt.Errorf("client is nil")
	}

	select {
	case p.pool <- cli:
		return nil
	default:
		return cli.Close()
	}
}
