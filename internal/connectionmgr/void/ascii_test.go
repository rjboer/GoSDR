package connectionmgr

import "testing"

func TestExecCommandBlockedInBinary(t *testing.T) {
	m := &Manager{Mode: ModeBinary}
	if _, err := m.ExecCommand("PRINT"); err == nil {
		t.Fatalf("expected ExecCommand to fail in binary mode")
	}
}
