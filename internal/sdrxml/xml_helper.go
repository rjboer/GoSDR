package sdrxml

import (
	"encoding/binary"
	"sort"
)

func (dev *DeviceEntry) BuildDecodeMap() {

	//the xml order is not the same as the scan order, so we need to sort the enabled channels
	enabled := []*ChannelEntry{}
	for i := range dev.Channel {
		if dev.Channel[i].Enabled {
			enabled = append(enabled, &dev.Channel[i])
		}
	}

	sort.Slice(enabled, func(i, j int) bool {
		return enabled[i].ParsedFormat.Index < enabled[j].ParsedFormat.Index
	})

	//then we can create the decode map
	DecodeMap := DecodeMap{}
	size := uint32(0)
	for i, ch := range enabled {
		if ch.Enabled {
			elementBytes := uint32((ch.ParsedFormat.Length + 7) / 8)
			DecodeMap.Entries = append(DecodeMap.Entries, DecodeEntry{
				Channel:   enabled[i],
				Offset:    size,
				Length:    ch.ParsedFormat.Length,
				TotalSize: elementBytes * ch.ParsedFormat.Repeat,
			})
			size += ch.SampleSize
		}
	}
	DecodeMap.SampleSize = size

	dev.DecodeMap = DecodeMap
}

func extract(raw []byte, pf *ScanFormat) int64 {
	var u uint64 = 0

	switch len(raw) {
	case 1:
		u = uint64(raw[0])
	case 2:
		if pf.IsBE {
			u = uint64(binary.BigEndian.Uint16(raw))
		} else {
			u = uint64(binary.LittleEndian.Uint16(raw))
		}
	case 4:
		if pf.IsBE {
			u = uint64(binary.BigEndian.Uint32(raw))
		} else {
			u = uint64(binary.LittleEndian.Uint32(raw))
		}
	case 8:
		if pf.IsBE {
			u = binary.BigEndian.Uint64(raw)
		} else {
			u = binary.LittleEndian.Uint64(raw)
		}
	default:
		if pf.IsBE {
			for _, b := range raw {
				u = (u << 8) | uint64(b)
			}
		} else {
			for i := len(raw) - 1; i >= 0; i-- {
				u = (u << 8) | uint64(raw[i])
			}
		}
	}

	if pf.Shift > 0 {
		u >>= pf.Shift
	}

	mask := uint64((1 << pf.Bits) - 1)
	u &= mask

	if pf.IsSigned {
		sign := uint64(1 << (pf.Bits - 1))
		if (u & sign) != 0 {
			u |= ^mask
		}
	}

	val := int64(u)
	if pf.WithScale {
		return int64(float64(val) * pf.Scale)
	}
	return val
}

func (dev *DeviceEntry) Decode(buf []byte) []map[string][]int64 {
	dm := dev.DecodeMap
	frameSize := int(dm.SampleSize)
	count := len(buf) / frameSize
	out := make([]map[string][]int64, count)

	for i := 0; i < count; i++ {
		frame := buf[i*frameSize : (i+1)*frameSize]
		m := make(map[string][]int64)

		for _, e := range dm.Entries {
			pf := e.Channel.ParsedFormat
			vals := make([]int64, pf.Repeat)
			per := int((pf.Length + 7) / 8)

			raw := frame[e.Offset : e.Offset+e.TotalSize]

			for r := uint32(0); r < pf.Repeat; r++ {
				s := int(r) * per
				vals[r] = extract(raw[s:s+per], pf)
			}

			name := e.Channel.Name
			if name == "" {
				name = e.Channel.ID
			}

			m[name] = vals
		}

		out[i] = m
	}

	return out
}
