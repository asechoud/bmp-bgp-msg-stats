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
// When Enterprise is true, Type is the 15-bit enterprise code and PEN is set
// (draft-ietf-grow-bmp-tlv E-bit).
type TLV struct {
	Type       Type
	Data       []byte
	Enterprise bool
	PEN        uint32
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

	// Enterprise process resource gauges (E-bit + PEN). Values are
	// hundredths of a percent (0–10000). Present keyed by EntProcess*.
	ProcessCPUHundredths    uint32 `json:"process_cpu_hundredths,omitempty"`
	ProcessMemoryHundredths uint32 `json:"process_memory_hundredths,omitempty"`
	ResourcePEN             uint32 `json:"resource_pen,omitempty"`

	Present map[Type]bool `json:"present"`
}

// NewCounters returns an empty Counters with Present initialized.
func NewCounters() Counters {
	return Counters{Present: make(map[Type]bool)}
}

// ParseTLVs extracts draft PDU counters and enterprise process resource
// gauges from a Statistics Report TLV list. Unknown types are skipped.
// Duplicate draft types return an error (Section 5.1: each type MUST appear
// at most once).
func (r Registry) ParseTLVs(tlvs []TLV) (Counters, error) {
	c := NewCounters()
	seen := make(map[Type]bool)
	seenEnt := make(map[Type]bool)
	for _, tlv := range tlvs {
		if tlv.Enterprise {
			if err := c.applyEnterprise(tlv, seenEnt); err != nil {
				return Counters{}, err
			}
			continue
		}
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

func (c *Counters) applyEnterprise(tlv TLV, seen map[Type]bool) error {
	if tlv.PEN != DefaultPEN {
		// Other PENs are ignored (forward-compatible).
		return nil
	}
	if seen[tlv.Type] {
		return fmt.Errorf("stats: duplicate enterprise type %d PEN %d", tlv.Type, tlv.PEN)
	}
	switch tlv.Type {
	case EntProcessCPU, EntProcessMemory:
		v, err := tlv.Value(Width32)
		if err != nil {
			return err
		}
		seen[tlv.Type] = true
		c.Present[tlv.Type] = true
		c.ResourcePEN = tlv.PEN
		if tlv.Type == EntProcessCPU {
			c.ProcessCPUHundredths = uint32(v)
		} else {
			c.ProcessMemoryHundredths = uint32(v)
		}
	}
	return nil
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
	out = append(out, c.encodeResourceTLVs()...)
	return out, nil
}

func (c Counters) encodeResourceTLVs() []TLV {
	pen := c.ResourcePEN
	if pen == 0 {
		pen = DefaultPEN
	}
	var out []TLV
	for _, rs := range DefaultResourceSpecs() {
		if c.Present != nil && !c.Present[rs.EntType] {
			continue
		}
		var v uint64
		switch rs.EntType {
		case EntProcessCPU:
			v = uint64(c.ProcessCPUHundredths)
		case EntProcessMemory:
			v = uint64(c.ProcessMemoryHundredths)
		}
		raw, err := EncodeEnterpriseTLV(pen, rs.EntType, v, rs.Width)
		if err != nil {
			continue
		}
		out = append(out, TLV{
			Type:       rs.EntType,
			Enterprise: true,
			PEN:        pen,
			Data:       raw[8:], // skip hdr(4)+PEN(4)
		})
	}
	return out
}

// EncodeBytes serializes TLVs into the Stats Data portion (without Stats Count).
func EncodeBytes(tlvs []TLV) ([]byte, error) {
	var buf []byte
	for _, tlv := range tlvs {
		chunk, err := tlv.EncodeWire()
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
