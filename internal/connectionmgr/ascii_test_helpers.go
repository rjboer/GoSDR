package connectionmgr

import (
	"fmt"
	"net"
	"testing"
)

func writeIntegerLine(t *testing.T, conn net.Conn, val int) {
	t.Helper()

	payload := make([]byte, 64)
	copy(payload, []byte(fmt.Sprintf("%d", val)))
	payload[len(payload)-1] = '\n'

	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("failed to write status line: %v", err)
	}
}

func writeStringLine(t *testing.T, conn net.Conn, line string) {
	t.Helper()

	payload := make([]byte, 64)
	copy(payload, []byte(line))
	payload[len(payload)-1] = '\n'

	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("failed to write line: %v", err)
	}
}
