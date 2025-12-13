package sdrxml

import (
	"encoding/hex"
	"testing"
)

func pf(bits int, be, signed bool, shift int, scale float64) *ScanFormat {
	return &ScanFormat{
		Bits:      uint32(bits),
		IsBE:      be,
		IsSigned:  signed,
		Shift:     uint32(shift),
		WithScale: scale != 1.0,
		Scale:     scale,
		Repeat:    1,
		Length:    uint32(bits),
	}
}

func decodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

func TestExtract(t *testing.T) {

	tests := []struct {
		name   string
		rawHex string
		pf     *ScanFormat
		want   int64
	}{
		// ---------------------------------------------------------
		// BASIC UNSIGNED CASES
		// ---------------------------------------------------------

		{
			name:   "u8 basic",
			rawHex: "7F",
			pf:     pf(8, false, false, 0, 1.0),
			want:   127,
		},
		{
			name:   "u16 LE basic",
			rawHex: "3412",
			pf:     pf(16, false, false, 0, 1.0),
			want:   0x1234,
		},
		{
			name:   "u16 BE basic",
			rawHex: "1234",
			pf:     pf(16, true, false, 0, 1.0),
			want:   0x1234,
		},
		{
			name:   "u32 LE basic",
			rawHex: "78563412",
			pf:     pf(32, false, false, 0, 1.0),
			want:   0x12345678,
		},
		{
			name:   "u32 BE basic",
			rawHex: "12345678",
			pf:     pf(32, true, false, 0, 1.0),
			want:   0x12345678,
		},

		// ---------------------------------------------------------
		// SIGNED CASES
		// ---------------------------------------------------------

		{
			name:   "s8 -1",
			rawHex: "FF",
			pf:     pf(8, false, true, 0, 1.0),
			want:   -1,
		},
		{
			name:   "s8 -128",
			rawHex: "80",
			pf:     pf(8, false, true, 0, 1.0),
			want:   -128,
		},

		{
			name:   "s16 BE -2",
			rawHex: "FFFE",
			pf:     pf(16, true, true, 0, 1.0),
			want:   -2,
		},
		{
			name:   "s16 LE -2",
			rawHex: "FEFF",
			pf:     pf(16, false, true, 0, 1.0),
			want:   -2,
		},

		// ---------------------------------------------------------
		// SHIFT & MASK TESTS
		// ---------------------------------------------------------

		{
			name:   "shift right by 4, unsigned 12bit",
			rawHex: "AB0F",
			pf:     pf(12, false, false, 4, 1.0), // take low 12 bits after >>4
			want:   int64((0x0FAB >> 4) & 0xFFF),
		},

		{
			name:   "signed 12-bit with sign extension",
			rawHex: "F00F",
			pf:     pf(12, false, true, 0, 1.0),
			want: func() int64 {
				// manually compute 12-bit signed of 0x0FF0 (LE)
				u := uint64(0x0FF0)
				bits := uint64(12)
				mask := uint64((1 << bits) - 1)
				u &= mask
				sign := uint64(1 << (bits - 1))
				if u&sign != 0 {
					u |= ^mask
				}
				return int64(u)
			}(),
		},

		// ---------------------------------------------------------
		// SCALING TESTS
		// ---------------------------------------------------------

		{
			name:   "scale applied",
			rawHex: "0100", // 1 in LE u16
			pf:     pf(16, false, false, 0, 0.5),
			want:   int64(float64(1) * 0.5),
		},

		// ---------------------------------------------------------
		// NON-POWER-OF-TWO RAW BYTES (manual BE/LE fill)
		// e.g. 3-byte formats used occasionally in older IIO drivers
		// ---------------------------------------------------------

		{
			name:   "3 byte LE unsigned",
			rawHex: "112233",
			pf:     pf(24, false, false, 0, 1.0),
			want:   0x332211,
		},
		{
			name:   "3 byte BE unsigned",
			rawHex: "112233",
			pf:     pf(24, true, false, 0, 1.0),
			want:   0x112233,
		},
	}

	// ---------------------------------------------------------
	// RUN TESTS
	// ---------------------------------------------------------
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := decodeHex(tt.rawHex)
			got := extract(raw, tt.pf)
			if got != tt.want {
				t.Fatalf("extract() mismatch: got=%d want=%d (0x%X vs 0x%X)",
					got, tt.want, uint64(got), uint64(tt.want))
			}
		})
	}
}
