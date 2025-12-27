package connectionmgr

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
)

type asciiMockStep struct {
	name              string
	expectLine        string
	expectPayloadLen  int
	expectPayload     []byte
	responseStatus    *int
	responseStatusRaw string
	responsePayload   []byte
}

type asciiMockResponder struct {
	t     *testing.T
	conn  net.Conn
	steps []asciiMockStep
	done  chan struct{}
	errCh chan error
}

func newASCIIMockResponder(t *testing.T, steps []asciiMockStep) (net.Conn, *asciiMockResponder) {
	t.Helper()

	client, server := net.Pipe()
	responder := &asciiMockResponder{
		t:     t,
		conn:  server,
		steps: steps,
		done:  make(chan struct{}),
		errCh: make(chan error, 1),
	}

	go responder.run()

	return client, responder
}

func (r *asciiMockResponder) run() {
	defer close(r.done)
	defer close(r.errCh)

	reader := bufio.NewReader(r.conn)
	for idx, step := range r.steps {
		line, err := reader.ReadString('\n')
		if err != nil {
			r.errCh <- fmt.Errorf("step %d (%s): read command: %w", idx, step.name, err)
			return
		}

		if step.expectLine != "" && line != step.expectLine {
			r.errCh <- fmt.Errorf("step %d (%s): unexpected command %q", idx, step.name, line)
			return
		}

		if step.expectPayloadLen > 0 {
			payload := make([]byte, step.expectPayloadLen)
			if _, err := io.ReadFull(reader, payload); err != nil {
				r.errCh <- fmt.Errorf("step %d (%s): read payload: %w", idx, step.name, err)
				return
			}
			if step.expectPayload != nil && !bytes.Equal(step.expectPayload, payload) {
				r.errCh <- fmt.Errorf("step %d (%s): payload mismatch: %q", idx, step.name, payload)
				return
			}
		}

		switch {
		case step.responseStatusRaw != "":
			if _, err := r.conn.Write([]byte(step.responseStatusRaw)); err != nil {
				r.errCh <- fmt.Errorf("step %d (%s): write raw status: %w", idx, step.name, err)
				return
			}
		case step.responseStatus != nil:
			writeIntegerLine(r.t, r.conn, *step.responseStatus)
		}

		if len(step.responsePayload) > 0 {
			if _, err := r.conn.Write(step.responsePayload); err != nil {
				r.errCh <- fmt.Errorf("step %d (%s): write payload: %w", idx, step.name, err)
				return
			}
		}
	}
}

func (r *asciiMockResponder) wait(t *testing.T) {
	t.Helper()
	<-r.done
	select {
	case err := <-r.errCh:
		if err != nil {
			t.Fatalf("mock responder error: %v", err)
		}
	default:
	}
}

func TestASCIIMockResponderCommands(t *testing.T) {
	tests := []struct {
		name      string
		steps     []asciiMockStep
		run       func(*Manager) error
		wantError bool
	}{
		{
			name: "help drains response",
			steps: []asciiMockStep{{
				name:            "HELP",
				expectLine:      "HELP\n",
				responsePayload: []byte("available commands\n"),
			}},
			run: func(m *Manager) error { return m.HelpfunctionASCII() },
		},
		{
			name: "version response",
			steps: []asciiMockStep{{
				name:            "VERSION",
				expectLine:      "VERSION\n",
				responsePayload: []byte("0.26\n"),
			}},
			run: func(m *Manager) error { return m.VersionASCII() },
		},
		{
			name: "print command",
			steps: []asciiMockStep{{
				name:            "PRINT",
				expectLine:      "PRINT\n",
				responsePayload: fixedLengthPayload(512*1024, "context dump"),
			}},
			run: func(m *Manager) error { return m.PrintASCII() },
		},
		{
			name: "timeout success",
			steps: []asciiMockStep{{
				name:           "TIMEOUT",
				expectLine:     "TIMEOUT 1500\r\n",
				responseStatus: intPtr(0),
			}},
			run: func(m *Manager) error { return m.SetTimeoutASCII(1500) },
		},
		{
			name: "timeout negative errno",
			steps: []asciiMockStep{{
				name:           "TIMEOUT ERR",
				expectLine:     "TIMEOUT 10\r\n",
				responseStatus: intPtr(-110),
			}},
			run:       func(m *Manager) error { return m.SetTimeoutASCII(10) },
			wantError: true,
		},
		{
			name: "set trigger empty",
			steps: []asciiMockStep{{
				name:           "SETTRIG clear",
				expectLine:     "SETTRIG pluto\r\n",
				responseStatus: intPtr(0),
			}},
			run: func(m *Manager) error { return m.SetTriggerASCII("pluto", "") },
		},
		{
			name: "set trigger errno",
			steps: []asciiMockStep{{
				name:           "SETTRIG err",
				expectLine:     "SETTRIG pluto external\r\n",
				responseStatus: intPtr(-5),
			}},
			run:       func(m *Manager) error { return m.SetTriggerASCII("pluto", "external") },
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, responder := newASCIIMockResponder(t, tt.steps)
			mgr := &Manager{Mode: ModeASCII, conn: client}

			err := tt.run(mgr)
			responder.wait(t)

			if tt.wantError && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestASCIIMockResponderDataFlows(t *testing.T) {
	tests := []struct {
		name       string
		steps      []asciiMockStep
		runString  func(*Manager) (string, error)
		runStatus  func(*Manager) (int, error)
		wantString string
		wantStatus int
		wantErr    bool
	}{
		{
			name: "get trigger value",
			steps: []asciiMockStep{{
				name:            "GETTRIG",
				expectLine:      "GETTRIG ad9361-phy\r\n",
				responseStatus:  intPtr(len("external")),
				responsePayload: []byte("external\n"),
			}},
			runString:  func(m *Manager) (string, error) { return m.GetTriggerASCII("ad9361-phy") },
			wantString: "external",
		},
		{
			name: "get trigger negative length",
			steps: []asciiMockStep{{
				name:           "GETTRIG ERR",
				expectLine:     "GETTRIG pluto\r\n",
				responseStatus: intPtr(-5),
			}},
			runString: func(m *Manager) (string, error) { return m.GetTriggerASCII("pluto") },
			wantErr:   true,
		},
		{
			name: "read buffer attr edge length",
			steps: []asciiMockStep{{
				name:            "READ BUFFER",
				expectLine:      "READ cf-ad9361-lpc BUFFER watermark\r\n",
				responseStatus:  intPtr(64),
				responsePayload: []byte(strings.Repeat("z", 64) + "\n"),
			}},
			runString:  func(m *Manager) (string, error) { return m.ReadBufferAttrASCII("cf-ad9361-lpc", "watermark") },
			wantString: strings.Repeat("z", 64),
		},
		{
			name: "read buffer attr negative",
			steps: []asciiMockStep{{
				name:           "READ BUFFER ERR",
				expectLine:     "READ cf-ad9361-lpc BUFFER cyclic\r\n",
				responseStatus: intPtr(-9),
			}},
			runString: func(m *Manager) (string, error) { return m.ReadBufferAttrASCII("cf-ad9361-lpc", "cyclic") },
			wantErr:   true,
		},
		{
			name: "write buffer attr payload",
			steps: []asciiMockStep{{
				name:             "WRITE BUFFER",
				expectLine:       "WRITE cf-ad9361-lpc BUFFER watermark 4\r\n",
				expectPayloadLen: 4,
				expectPayload:    []byte("beep"),
				responseStatus:   intPtr(0),
			}},
			runStatus: func(m *Manager) (int, error) {
				return m.WriteBufferAttrASCII("cf-ad9361-lpc", "watermark", []byte("beep"))
			},
			wantStatus: 0,
		},
		{
			name: "write buffer attr errno",
			steps: []asciiMockStep{{
				name:             "WRITE BUFFER ERR",
				expectLine:       "WRITE cf-ad9361-lpc BUFFER timeout 0\r\n",
				expectPayloadLen: 0,
				responseStatus:   intPtr(-22),
			}},
			runStatus: func(m *Manager) (int, error) {
				return m.WriteBufferAttrASCII("cf-ad9361-lpc", "timeout", nil)
			},
			wantStatus: -22,
			wantErr:    true,
		},
		{
			name: "read device attr",
			steps: []asciiMockStep{{
				name:            "READ DEV",
				expectLine:      "READ ad9361-phy gain\r\n",
				responseStatus:  intPtr(len("5dB")),
				responsePayload: fixedLengthPayload(len("5dB")+1, "5dB"),
			}},
			runString:  func(m *Manager) (string, error) { return m.ReadDeviceAttrASCII("ad9361-phy", "gain") },
			wantString: "5dB",
		},
		{
			name: "write device attr",
			steps: []asciiMockStep{{
				name:             "WRITE DEV",
				expectLine:       "WRITE ad9361-phy calibration 3\r\n",
				expectPayloadLen: 3,
				expectPayload:    []byte("abc"),
				responseStatus:   intPtr(-1),
			}},
			runStatus: func(m *Manager) (int, error) {
				return m.WriteDeviceAttrASCII("ad9361-phy", "calibration", "abc")
			},
			wantStatus: -1,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, responder := newASCIIMockResponder(t, tt.steps)
			mgr := &Manager{Mode: ModeASCII, conn: client}

			var (
				str    string
				status int
				err    error
			)

			switch {
			case tt.runString != nil:
				str, err = tt.runString(mgr)
			case tt.runStatus != nil:
				status, err = tt.runStatus(mgr)
			default:
				t.Fatalf("no runner provided")
			}

			responder.wait(t)

			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.runString != nil && str != tt.wantString {
				t.Fatalf("unexpected string: got %q want %q", str, tt.wantString)
			}
			if tt.runStatus != nil && status != tt.wantStatus {
				t.Fatalf("unexpected status: got %d want %d", status, tt.wantStatus)
			}
		})
	}
}

func newReadbufStep(deviceID string, announcedLength int, mask string, payload []byte) asciiMockStep {
	resp := make([]byte, 0, len(mask)+len(payload)+2)
	resp = append(resp, []byte(mask)...)
	resp = append(resp, '\n')
	resp = append(resp, payload...)
	resp = append(resp, '\n')

	return asciiMockStep{
		name:            fmt.Sprintf("READBUF %s", deviceID),
		expectLine:      fmt.Sprintf("READBUF %s %d\r\n", deviceID, announcedLength),
		responseStatus:  intPtr(announcedLength),
		responsePayload: resp,
	}
}

func fixedLengthPayload(total int, body string) []byte {
	if total <= 0 {
		return nil
	}

	buf := make([]byte, total)
	copy(buf, []byte(body))
	buf[len(buf)-1] = '\n'
	return buf
}

func intPtr(v int) *int {
	return &v
}
