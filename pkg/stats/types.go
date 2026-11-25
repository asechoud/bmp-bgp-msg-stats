// Package stats implements BGP session PDU counter Statistics Types
// defined in draft-choudhary-grow-bmp-bgp-msg-stats-00.
package stats

import (
	"encoding/binary"
	"fmt"
)

// Provisional Stat Type codes (TBD1–TBD10).
//
// IANA has not yet assigned values. These defaults use the next free
// Standards Action codes after RFC 9972 (44–53). Override via Registry
// or CLI flags when early allocation lands.
const (
	TypeUpdateReceived        Type = 44 // TBD1, 64-bit Counter
	TypeUpdateSent            Type = 45 // TBD2, 64-bit Counter
	TypeKeepaliveReceived     Type = 46 // TBD3, 32-bit Counter
	TypeKeepaliveSent         Type = 47 // TBD4, 32-bit Counter
	TypeNotificationReceived  Type = 48 // TBD5, 32-bit Counter
	TypeNotificationSent      Type = 49 // TBD6, 32-bit Counter
	TypeRouteRefreshReceived  Type = 50 // TBD7, 32-bit Counter
	TypeRouteRefreshSent      Type = 51 // TBD8, 32-bit Counter
	TypeTotalReceived         Type = 52 // TBD9, 64-bit Counter
	TypeTotalSent             Type = 53 // TBD10, 64-bit Counter
)

// Type is a BMP Statistics Type code.
type Type uint16

// Width is the on-wire Stat Len for a type.
type Width int

const (
	Width32 Width = 4
	Width64 Width = 8
)

// Spec describes one PDU counter type from the draft.
type Spec struct {
	Type        Type
	TBD         string
	Name        string
	Description string
	Width       Width
	Direction   string // "received" or "sent"
	MessageKind string // UPDATE, KEEPALIVE, NOTIFICATION, ROUTE-REFRESH, TOTAL
}

// DefaultSpecs returns the ten types from Section 5.2 / Table 1.
func DefaultSpecs() []Spec {
	return []Spec{
		{TypeUpdateReceived, "TBD1", "update_received", "BGP UPDATE messages received", Width64, "received", "UPDATE"},
		{TypeUpdateSent, "TBD2", "update_sent", "BGP UPDATE messages sent", Width64, "sent", "UPDATE"},
		{TypeKeepaliveReceived, "TBD3", "keepalive_received", "BGP KEEPALIVE messages received", Width32, "received", "KEEPALIVE"},
		{TypeKeepaliveSent, "TBD4", "keepalive_sent", "BGP KEEPALIVE messages sent", Width32, "sent", "KEEPALIVE"},
		{TypeNotificationReceived, "TBD5", "notification_received", "BGP NOTIFICATION messages received", Width32, "received", "NOTIFICATION"},
		{TypeNotificationSent, "TBD6", "notification_sent", "BGP NOTIFICATION messages sent", Width32, "sent", "NOTIFICATION"},
		{TypeRouteRefreshReceived, "TBD7", "route_refresh_received", "BGP ROUTE-REFRESH messages received", Width32, "received", "ROUTE-REFRESH"},
		{TypeRouteRefreshSent, "TBD8", "route_refresh_sent", "BGP ROUTE-REFRESH messages sent", Width32, "sent", "ROUTE-REFRESH"},
		{TypeTotalReceived, "TBD9", "total_received", "Total BGP messages received", Width64, "received", "TOTAL"},
		{TypeTotalSent, "TBD10", "total_sent", "Total BGP messages sent", Width64, "sent", "TOTAL"},
	}
}

// Registry maps Stat Type → Spec. Unknown types are ignored on decode.
type Registry map[Type]Spec

// DefaultRegistry builds a registry from DefaultSpecs.
func DefaultRegistry() Registry {
	r := make(Registry, 10)
	for _, s := range DefaultSpecs() {
		r[s.Type] = s
	}
	return r
}

// WithOverrides returns a copy of r with type codes remapped from old→new.
func (r Registry) WithOverrides(overrides map[Type]Type) Registry {
	if len(overrides) == 0 {
		return r
	}
	out := make(Registry, len(r))
	rev := make(map[Type]Type, len(overrides))
	for old, neu := range overrides {
		rev[neu] = old
	}
	for t, s := range r {
		if neu, ok := overrides[t]; ok {
			s.Type = neu
			out[neu] = s
			continue
		}
		if _, collided := rev[t]; collided {
			continue
		}
		out[t] = s
	}
	return out
}

// IsPDUCounter reports whether t is one of the draft PDU counter types.
func (r Registry) IsPDUCounter(t Type) bool {
	_, ok := r[t]
	return ok
}

// WidthOf returns the required Stat Len for t, or 0 if unknown.
func (r Registry) WidthOf(t Type) Width {
	if s, ok := r[t]; ok {
		return s.Width
	}
	return 0
}

// TLV is one statistic TLV as carried in a Statistics Report.
type TLV struct {
	Type Type
	Data []byte
}

// EncodeTLV encodes a single Stat Type / Stat Len / Stat Data TLV.
func EncodeTLV(t Type, value uint64, width Width) ([]byte, error) {
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

// DecodeTLV parses one TLV from b and returns the remainder.
func DecodeTLV(b []byte) (TLV, []byte, error) {
	if len(b) < 4 {
		return TLV{}, nil, fmt.Errorf("stats: TLV header truncated")
	}
	t := Type(binary.BigEndian.Uint16(b[0:2]))
	n := int(binary.BigEndian.Uint16(b[2:4]))
	if len(b) < 4+n {
		return TLV{}, nil, fmt.Errorf("stats: TLV data truncated (type=%d len=%d)", t, n)
	}
	data := make([]byte, n)
	copy(data, b[4:4+n])
	return TLV{Type: t, Data: data}, b[4+n:], nil
}

// Value interprets Stat Data according to width. Absent/unknown → ok=false.
func (tlv TLV) Value(width Width) (uint64, error) {
	switch width {
	case Width32:
		if len(tlv.Data) != 4 {
			return 0, fmt.Errorf("stats: type %d expected Stat Len 4, got %d", tlv.Type, len(tlv.Data))
		}
		return uint64(binary.BigEndian.Uint32(tlv.Data)), nil
	case Width64:
		if len(tlv.Data) != 8 {
			return 0, fmt.Errorf("stats: type %d expected Stat Len 8, got %d", tlv.Type, len(tlv.Data))
		}
		return binary.BigEndian.Uint64(tlv.Data), nil
	default:
		return 0, fmt.Errorf("stats: unknown width %d", width)
	}
}

// Counters holds the ten PDU counters for one peer (draft Section 5.2).
// A zero value means the counter was observed as zero, not that it was
// omitted. Use Present to distinguish omission (Section 5.1).
type Counters struct {
	UpdateReceived       uint64 `json:"update_received"`
	UpdateSent           uint64 `json:"update_sent"`
	KeepaliveReceived    uint32 `json:"keepalive_received"`
	KeepaliveSent        uint32 `json:"keepalive_sent"`
	NotificationReceived uint32 `json:"notification_received"`
	NotificationSent     uint32 `json:"notification_sent"`
	RouteRefreshReceived uint32 `json:"route_refresh_received"`
	RouteRefreshSent     uint32 `json:"route_refresh_sent"`
	TotalReceived        uint64 `json:"total_received"`
	TotalSent            uint64 `json:"total_sent"`

	Present map[Type]bool `json:"present"`
}

// NewCounters returns an empty Counters with Present initialized.
func NewCounters() Counters {
	return Counters{Present: make(map[Type]bool)}
}

// ParseTLVs extracts draft PDU counters from a Statistics Report TLV list.
// Unknown types are skipped. Duplicate draft types return an error
// (Section 5.1: each type MUST appear at most once).
func (r Registry) ParseTLVs(tlvs []TLV) (Counters, error) {
	c := NewCounters()
	seen := make(map[Type]bool)
	for _, tlv := range tlvs {
		spec, ok := r[tlv.Type]
		if !ok {
			continue
		}
		if seen[tlv.Type] {
			return Counters{}, fmt.Errorf("stats: duplicate type %d (%s) in report", tlv.Type, spec.TBD)
		}
		seen[tlv.Type] = true
		v, err := tlv.Value(spec.Width)
		if err != nil {
			return Counters{}, err
		}
		c.Present[tlv.Type] = true
		switch spec.TBD {
		case "TBD1":
			c.UpdateReceived = v
		case "TBD2":
			c.UpdateSent = v
		case "TBD3":
			c.KeepaliveReceived = uint32(v)
		case "TBD4":
			c.KeepaliveSent = uint32(v)
		case "TBD5":
			c.NotificationReceived = uint32(v)
		case "TBD6":
			c.NotificationSent = uint32(v)
		case "TBD7":
			c.RouteRefreshReceived = uint32(v)
		case "TBD8":
			c.RouteRefreshSent = uint32(v)
		case "TBD9":
			c.TotalReceived = v
		case "TBD10":
			c.TotalSent = v
		}
	}
	return c, nil
}

// Encode builds Stat TLVs for every Present counter (or all, if Present is nil).
func (c Counters) Encode(reg Registry) ([]TLV, error) {
	specs := DefaultSpecs()
	out := make([]TLV, 0, len(specs))
	for _, s := range specs {
		spec, ok := reg[s.Type]
		if !ok {
			// Type remapped: find by TBD name.
			for _, cand := range reg {
				if cand.TBD == s.TBD {
					spec = cand
					ok = true
					break
				}
			}
		}
		if !ok {
			continue
		}
		if c.Present != nil && !c.Present[spec.Type] && !c.Present[s.Type] {
			continue
		}
		var v uint64
		switch s.TBD {
		case "TBD1":
			v = c.UpdateReceived
		case "TBD2":
			v = c.UpdateSent
		case "TBD3":
			v = uint64(c.KeepaliveReceived)
		case "TBD4":
			v = uint64(c.KeepaliveSent)
		case "TBD5":
			v = uint64(c.NotificationReceived)
		case "TBD6":
			v = uint64(c.NotificationSent)
		case "TBD7":
			v = uint64(c.RouteRefreshReceived)
		case "TBD8":
			v = uint64(c.RouteRefreshSent)
		case "TBD9":
			v = c.TotalReceived
		case "TBD10":
			v = c.TotalSent
		}
		raw, err := EncodeTLV(spec.Type, v, spec.Width)
		if err != nil {
			return nil, err
		}
		out = append(out, TLV{Type: spec.Type, Data: raw[4:]})
	}
	return out, nil
}

// EncodeBytes serializes TLVs into the Stats Data portion (without Stats Count).
func EncodeBytes(tlvs []TLV) ([]byte, error) {
	var buf []byte
	for _, tlv := range tlvs {
		chunk, err := EncodeTLV(tlv.Type, valueFromData(tlv.Data), Width(len(tlv.Data)))
		if err != nil {
			return nil, err
		}
		buf = append(buf, chunk...)
	}
	return buf, nil
}

func valueFromData(data []byte) uint64 {
	switch len(data) {
	case 4:
		return uint64(binary.BigEndian.Uint32(data))
	case 8:
		return binary.BigEndian.Uint64(data)
	default:
		return 0
	}
}

// DraftReference is the Internet-Draft this package implements.
const DraftReference = "draft-choudhary-grow-bmp-bgp-msg-stats-00"
