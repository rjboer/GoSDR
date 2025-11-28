package iiod

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"time"
)

type Client struct {
	conn   net.Conn
	reader *bufio.Reader
}

func Dial(addr string) (*Client, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	c, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   c,
		reader: bufio.NewReader(c),
	}, nil
}

// Close shuts down the underlying network connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	c.conn = nil
	return err
}

// Send issues a command to the server and returns its response payload.
func (c *Client) Send(cmd string) (string, error) {
	return c.send(cmd)
}

func (c *Client) send(cmd string) (string, error) {
	fmt.Fprintf(c.conn, "%s\n", cmd)
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimSpace(line)

	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		return "", fmt.Errorf("malformed reply: %s", line)
	}

	status := parts[0]
	if status != "0" {
		return "", fmt.Errorf("iiod error: %s", line)
	}

	if len(parts) >= 3 {
		return parts[2], nil
	}
	return "", nil
}
