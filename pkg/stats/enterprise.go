package stats

import (
	"encoding/binary"
	"fmt"
)

// E-bit / enterprise Stat Type encoding per draft-ietf-grow-bmp-tlv
// (BMP v4). When the most significant bit of Stat Type is set, a 4-byte
// IANA Private Enterprise Number (PEN) follows Stat Len and precedes
// Stat Data. Stat Len includes the PEN length.
const (
	EBit     Type = 0x8000
	TypeMask Type = 0x7fff

	// DefaultPEN is Cisco Systems' IANA Private Enterprise Number.
	// Used for provisional process resource gauges until/if IANA
	// allocates standard Stat Types for these metrics.
	DefaultPEN uint32 = 9

	// Enterprise Stat Type codes (15-bit; E-bit set on the wire).
	EntProcessCPU    Type = 1 // process CPU utilization gauge
	EntProcessMemory Type = 2 // process memory utilization gauge
)

// ResourceSpec describes an enterprise process-resource gauge.
type ResourceSpec struct {
	EntType     Type
	Name        string
	Description string
	Width       Width
}

// DefaultResourceSpecs returns the enterprise process CPU/memory gauges.
// Values are 32-bit gauges in hundredths of a percent (0 = 0.00%,
// 10000 = 100.00%).
func DefaultResourceSpecs() []ResourceSpec {
	return []ResourceSpec{
		{EntProcessCPU, "process_cpu", "BGP/BMP process CPU utilization (hundredths of a percent)", Width32},
		{EntProcessMemory, "process_memory", "BGP/BMP process memory utilization (hundredths of a percent)", Width32},
	}
}

// WireType returns the on-wire Stat Type with E-bit set for enterprise types.
func WireType(ent Type) Type {
	return EBit | (ent & TypeMask)
}

// IsEnterprise reports whether the on-wire Stat Type has the E-bit set.
func IsEnterprise(wire Type) bool {
	return wire&EBit != 0
}

// EnterpriseType strips the E-bit from an on-wire Stat Type.
func EnterpriseType(wire Type) Type {
	return wire & TypeMask
}

// EncodeEnterpriseTLV encodes E-bit Stat Type | Stat Len | PEN | Stat Data.
// Stat Len is 4 (PEN) + width of the gauge value.
func EncodeEnterpriseTLV(pen uint32, ent Type, value uint64, width Width) ([]byte, error) {
	wire := WireType(ent)
	switch width {
	case Width32:
		if value > 0xffffffff {
			return nil, fmt.Errorf("stats: enterprise value %d exceeds 32-bit Gauge", value)
		}
		buf := make([]byte, 4+4+4) // hdr + PEN + data
		binary.BigEndian.PutUint16(buf[0:2], uint16(wire))
		binary.BigEndian.PutUint16(buf[2:4], 4+4) // PEN + data
		binary.BigEndian.PutUint32(buf[4:8], pen)
		binary.BigEndian.PutUint32(buf[8:12], uint32(value))
		return buf, nil
	case Width64:
		buf := make([]byte, 4+4+8)
		binary.BigEndian.PutUint16(buf[0:2], uint16(wire))
		binary.BigEndian.PutUint16(buf[2:4], 4+8)
		binary.BigEndian.PutUint32(buf[4:8], pen)
		binary.BigEndian.PutUint64(buf[8:16], value)
		return buf, nil
	default:
		return nil, fmt.Errorf("stats: unsupported width %d", width)
	}
}

// DecodeTLV parses one IANA or enterprise Stat TLV from b.
func DecodeTLV(b []byte) (TLV, []byte, error) {
	if len(b) < 4 {
		return TLV{}, nil, fmt.Errorf("stats: TLV header truncated")
	}
	wire := Type(binary.BigEndian.Uint16(b[0:2]))
	n := int(binary.BigEndian.Uint16(b[2:4]))
	if len(b) < 4+n {
		return TLV{}, nil, fmt.Errorf("stats: TLV data truncated (type=%d len=%d)", wire, n)
	}
	payload := b[4 : 4+n]
	tlv := TLV{Type: wire}
	if IsEnterprise(wire) {
		if n < 4 {
			return TLV{}, nil, fmt.Errorf("stats: enterprise TLV len %d too short for PEN", n)
		}
		tlv.Enterprise = true
		tlv.Type = EnterpriseType(wire)
		tlv.PEN = binary.BigEndian.Uint32(payload[0:4])
		tlv.Data = make([]byte, n-4)
		copy(tlv.Data, payload[4:])
	} else {
		tlv.Data = make([]byte, n)
		copy(tlv.Data, payload)
	}
	return tlv, b[4+n:], nil
}

// EncodeTLV encodes a single IANA Stat Type / Stat Len / Stat Data TLV.
func EncodeTLV(t Type, value uint64, width Width) ([]byte, error) {
	if IsEnterprise(t) {
		return nil, fmt.Errorf("stats: use EncodeEnterpriseTLV for E-bit types")
	}
	switch width {
	case Width32:
		if value > 0xffffffff {
			return nil, fmt.Errorf("stats: value %d exceeds 32-bit Counter", value)
		}
		buf := make([]byte, 8)
		binary.BigEndian.PutUint16(buf[0:2], uint16(t))
		binary.BigEndian.PutUint16(buf[2:4], 4)
		binary.BigEndian.PutUint32(buf[4:8], uint32(value))
		return buf, nil
	case Width64:
		buf := make([]byte, 12)
		binary.BigEndian.PutUint16(buf[0:2], uint16(t))
		binary.BigEndian.PutUint16(buf[2:4], 8)
		binary.BigEndian.PutUint64(buf[4:12], value)
		return buf, nil
	default:
		return nil, fmt.Errorf("stats: unsupported width %d", width)
	}
}

// EncodeWire serializes one already-populated TLV (IANA or enterprise).
func (tlv TLV) EncodeWire() ([]byte, error) {
	if tlv.Enterprise {
		return EncodeEnterpriseTLV(tlv.PEN, tlv.Type, valueFromData(tlv.Data), Width(len(tlv.Data)))
	}
	return EncodeTLV(tlv.Type, valueFromData(tlv.Data), Width(len(tlv.Data)))
}
