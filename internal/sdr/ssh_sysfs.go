package sdr

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHConfig describes the parameters required to configure a sysfs attribute over SSH.
// It mirrors IIO attribute writes so the backend can fall back when IIOD write support
// is unavailable (e.g., protocol v0.25 on older Pluto firmware).
type SSHConfig struct {
	Host      string
	User      string
	Password  string
	KeyPath   string
	Port      int
	SysfsRoot string
}

// SSHAttributeWriter establishes an SSH session to the Pluto SDR and writes sysfs
// attributes that correspond to IIO device/channel attributes.
type SSHAttributeWriter struct {
	mu     sync.Mutex
	cfg    SSHConfig
	client *ssh.Client
}

// NewSSHAttributeWriter validates configuration and prepares a writer instance.
func NewSSHAttributeWriter(cfg SSHConfig) (*SSHAttributeWriter, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("ssh host is required for sysfs fallback")
	}
	if cfg.User == "" {
		cfg.User = "root"
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.SysfsRoot == "" {
		cfg.SysfsRoot = "/sys/bus/iio/devices"
	}

	return &SSHAttributeWriter{cfg: cfg}, nil
}

// WriteAttribute writes the provided value to the sysfs path derived from the IIO
// attribute triple (device/channel/attr).
func (w *SSHAttributeWriter) WriteAttribute(ctx context.Context, device, channel, attr, value string) error {
	client, err := w.dial(ctx)
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	target := w.attributePath(device, channel, attr)
	// Use printf to avoid shell interpretation of the value contents.
	quotedValue := shellQuote(value)
	cmd := fmt.Sprintf("printf %s > %s", quotedValue, target)
	if err := session.Run(cmd); err != nil {
		return fmt.Errorf("write sysfs attribute via ssh: %w", err)
	}

	return nil
}

func (w *SSHAttributeWriter) dial(ctx context.Context) (*ssh.Client, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.client != nil {
		return w.client, nil
	}

	auth := []ssh.AuthMethod{}
	if w.cfg.Password != "" {
		auth = append(auth, ssh.Password(w.cfg.Password))
	}
	if w.cfg.KeyPath != "" {
		key, err := os.ReadFile(w.cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read ssh key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse ssh key: %w", err)
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if len(auth) == 0 {
		return nil, fmt.Errorf("no ssh password or key configured")
	}

	config := &ssh.ClientConfig{
		User:            w.cfg.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", w.cfg.Host, w.cfg.Port)
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial ssh: %w", err)
	}

	clientConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		return nil, fmt.Errorf("create ssh client: %w", err)
	}

	w.client = ssh.NewClient(clientConn, chans, reqs)
	return w.client, nil
}

func (w *SSHAttributeWriter) attributePath(device, channel, attr string) string {
	base := filepath.Join(w.cfg.SysfsRoot, device)
	if channel == "" {
		return filepath.Join(base, attr)
	}

	prefix := "in"
	if strings.HasPrefix(strings.ToLower(channel), "altvoltage") || strings.HasPrefix(strings.ToLower(channel), "out_") {
		prefix = "out"
	}

	filename := fmt.Sprintf("%s_%s_%s", prefix, channel, attr)
	return filepath.Join(base, filename)
}

// shellQuote returns a value wrapped in single quotes with embedded quotes escaped
// for safe shell usage.
func shellQuote(value string) string {
	escaped := strings.ReplaceAll(value, "'", "'\\''")
	return fmt.Sprintf("'%s'", escaped)
}
