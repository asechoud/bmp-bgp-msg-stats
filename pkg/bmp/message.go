// Package bmp implements BMP version 3 framing per RFC 7854, with
// support for Statistics Report TLVs from draft-choudhary-grow-bmp-bgp-msg-stats-00.
package bmp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/asechoud/bmp-bgp-msg-stats/pkg/stats"
)

const (
	Version = 3

	MsgRouteMonitoring = 0
	MsgStatsReport     = 1
	MsgPeerDown        = 2
	MsgPeerUp          = 3
	MsgInitiation      = 4
	MsgTermination     = 5
	MsgRouteMirroring  = 6

	PeerTypeGlobal  = 0
	PeerTypeRD      = 1
	PeerTypeLocal   = 2
	PeerTypeLocRIB  = 3 // RFC 9069 — draft PDU counters MUST NOT be sent for this peer

	FlagV = 0x80 // IPv6 peer address
	FlagL = 0x40 // post-policy
	FlagA = 0x20 // legacy 2-byte AS_PATH
	FlagO = 0x10 // Adj-RIB-Out (RFC 8671); MUST be 0 for Statistics Reports

	InfoString   = 0
	InfoSysDescr = 1
	InfoSysName  = 2

	CommonHeaderLen = 6
	PerPeerHeaderLen = 42
)

// CommonHeader is the BMP common header (RFC 7854 §4.1).
type CommonHeader struct {
	Version uint8
	Length  uint32
	Type    uint8
}

// PerPeerHeader is the BMP per-peer header (RFC 7854 §4.2).
type PerPeerHeader struct {
	PeerType         uint8
	PeerFlags        uint8
	PeerDistinguisher [8]byte
	PeerAddress      net.IP
	PeerAS           uint32
	PeerBGPID        net.IP
	Timestamp        time.Time
}

// IsIPv6 reports the V flag.
func (p PerPeerHeader) IsIPv6() bool { return p.PeerFlags&FlagV != 0 }

// IsAdjRIBOut reports the O flag (RFC 8671).
func (p PerPeerHeader) IsAdjRIBOut() bool { return p.PeerFlags&FlagO != 0 }

// PeerKey uniquely identifies a monitored peer on a BMP session.
func (p PerPeerHeader) PeerKey() string {
	return fmt.Sprintf("%d/%x/%s/%d/%s",
		p.PeerType, p.PeerDistinguisher, p.PeerAddress.String(), p.PeerAS, bgpIDString(p.PeerBGPID))
}

func bgpIDString(ip net.IP) string {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4.String()
	}
	return ip.String()
}

// InformationTLV is used by Initiation and Peer Up.
type InformationTLV struct {
	Type uint16
	Info []byte
}

// Message is a decoded BMP message.
type Message struct {
	Header CommonHeader
	Peer   *PerPeerHeader // nil for Initiation/Termination

	// Initiation / Termination
	Info []InformationTLV

	// Stats Report
	StatsCount uint32
	Stats      []stats.TLV

	// Peer Down
	PeerDownReason uint8
	PeerDownData   []byte

	// Peer Up
	LocalAddress net.IP
	LocalPort    uint16
	RemotePort   uint16
	SentOPEN     []byte
	ReceivedOPEN []byte
	PeerInfo     []InformationTLV

	// Route Monitoring / raw remainder
	Payload []byte
}

// ReadMessage reads one BMP message from r.
func ReadMessage(r io.Reader) (*Message, error) {
	hdr := make([]byte, CommonHeaderLen)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	ch := CommonHeader{
		Version: hdr[0],
		Length:  binary.BigEndian.Uint32(hdr[1:5]),
		Type:    hdr[5],
	}
	if ch.Version != Version {
		return nil, fmt.Errorf("bmp: unsupported version %d", ch.Version)
	}
	if ch.Length < CommonHeaderLen {
		return nil, fmt.Errorf("bmp: message length %d too small", ch.Length)
	}
	bodyLen := int(ch.Length) - CommonHeaderLen
	body := make([]byte, bodyLen)
	if bodyLen > 0 {
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, err
		}
	}
	msg := &Message{Header: ch}
	if err := msg.parseBody(body); err != nil {
		return nil, err
	}
	return msg, nil
}

func (m *Message) parseBody(body []byte) error {
	switch m.Header.Type {
	case MsgInitiation, MsgTermination:
		tlvs, err := parseInfoTLVs(body)
		if err != nil {
			return err
		}
		m.Info = tlvs
		return nil
	case MsgStatsReport, MsgPeerUp, MsgPeerDown, MsgRouteMonitoring, MsgRouteMirroring:
		if len(body) < PerPeerHeaderLen {
			return fmt.Errorf("bmp: per-peer header truncated")
		}
		peer, err := parsePerPeerHeader(body[:PerPeerHeaderLen])
		if err != nil {
			return err
		}
		m.Peer = &peer
		rest := body[PerPeerHeaderLen:]
		switch m.Header.Type {
		case MsgStatsReport:
			return m.parseStats(rest)
		case MsgPeerDown:
			if len(rest) < 1 {
				return fmt.Errorf("bmp: peer down truncated")
			}
			m.PeerDownReason = rest[0]
			m.PeerDownData = append([]byte(nil), rest[1:]...)
			return nil
		case MsgPeerUp:
			return m.parsePeerUp(rest)
		default:
			m.Payload = append([]byte(nil), rest...)
			return nil
		}
	default:
		m.Payload = append([]byte(nil), body...)
		return nil
	}
}

func (m *Message) parseStats(rest []byte) error {
	if len(rest) < 4 {
		return fmt.Errorf("bmp: stats count truncated")
	}
	m.StatsCount = binary.BigEndian.Uint32(rest[0:4])
	rest = rest[4:]
	var tlvs []stats.TLV
	for i := uint32(0); i < m.StatsCount; i++ {
		tlv, next, err := stats.DecodeTLV(rest)
		if err != nil {
			return err
		}
		tlvs = append(tlvs, tlv)
		rest = next
	}
	m.Stats = tlvs
	return nil
}

func (m *Message) parsePeerUp(rest []byte) error {
	if len(rest) < 20 {
		return fmt.Errorf("bmp: peer up truncated")
	}
	m.LocalAddress = decodePeerIP(rest[0:16], m.Peer.IsIPv6())
	m.LocalPort = binary.BigEndian.Uint16(rest[16:18])
	m.RemotePort = binary.BigEndian.Uint16(rest[18:20])
	rest = rest[20:]
	sent, rest, err := readBGPMessage(rest)
	if err != nil {
		return fmt.Errorf("bmp: peer up sent OPEN: %w", err)
	}
	recv, rest, err := readBGPMessage(rest)
	if err != nil {
		return fmt.Errorf("bmp: peer up received OPEN: %w", err)
	}
	m.SentOPEN = sent
	m.ReceivedOPEN = recv
	if len(rest) > 0 {
		tlvs, err := parseInfoTLVs(rest)
		if err != nil {
			return err
		}
		m.PeerInfo = tlvs
	}
	return nil
}

func readBGPMessage(b []byte) ([]byte, []byte, error) {
	if len(b) < 19 {
		return nil, nil, fmt.Errorf("bgp header truncated")
	}
	length := binary.BigEndian.Uint16(b[16:18])
	if length < 19 || int(length) > len(b) {
		return nil, nil, fmt.Errorf("bgp length %d invalid", length)
	}
	msg := make([]byte, length)
	copy(msg, b[:length])
	return msg, b[length:], nil
}

func parsePerPeerHeader(b []byte) (PerPeerHeader, error) {
	p := PerPeerHeader{
		PeerType:  b[0],
		PeerFlags: b[1],
	}
	copy(p.PeerDistinguisher[:], b[2:10])
	p.PeerAddress = decodePeerIP(b[10:26], p.IsIPv6())
	p.PeerAS = binary.BigEndian.Uint32(b[26:30])
	p.PeerBGPID = net.IPv4(b[30], b[31], b[32], b[33])
	sec := binary.BigEndian.Uint32(b[34:38])
	usec := binary.BigEndian.Uint32(b[38:42])
	if sec != 0 || usec != 0 {
		p.Timestamp = time.Unix(int64(sec), int64(usec)*1000)
	}
	return p, nil
}

func decodePeerIP(b []byte, v6 bool) net.IP {
	if v6 {
		ip := make(net.IP, 16)
		copy(ip, b)
		return ip
	}
	return net.IPv4(b[12], b[13], b[14], b[15])
}

func encodePeerIP(ip net.IP, v6 bool) [16]byte {
	var out [16]byte
	if v6 {
		copy(out[:], ip.To16())
	} else {
		ip4 := ip.To4()
		if ip4 == nil {
			ip4 = net.IPv4zero
		}
		copy(out[12:], ip4)
	}
	return out
}

func parseInfoTLVs(b []byte) ([]InformationTLV, error) {
	var out []InformationTLV
	for len(b) > 0 {
		if len(b) < 4 {
			return nil, fmt.Errorf("bmp: info TLV truncated")
		}
		t := binary.BigEndian.Uint16(b[0:2])
		n := int(binary.BigEndian.Uint16(b[2:4]))
		if len(b) < 4+n {
			return nil, fmt.Errorf("bmp: info TLV data truncated")
		}
		info := make([]byte, n)
		copy(info, b[4:4+n])
		out = append(out, InformationTLV{Type: t, Info: info})
		b = b[4+n:]
	}
	return out, nil
}

func encodeInfoTLVs(tlvs []InformationTLV) []byte {
	var buf []byte
	for _, t := range tlvs {
		hdr := make([]byte, 4)
		binary.BigEndian.PutUint16(hdr[0:2], t.Type)
		binary.BigEndian.PutUint16(hdr[2:4], uint16(len(t.Info)))
		buf = append(buf, hdr...)
		buf = append(buf, t.Info...)
	}
	return buf
}

// EncodeInitiation builds an Initiation message.
func EncodeInitiation(sysDescr, sysName string, extra ...InformationTLV) []byte {
	tlvs := []InformationTLV{
		{Type: InfoSysDescr, Info: []byte(sysDescr)},
		{Type: InfoSysName, Info: []byte(sysName)},
	}
	tlvs = append(tlvs, extra...)
	body := encodeInfoTLVs(tlvs)
	return wrapCommon(MsgInitiation, body)
}

// EncodePeerUp builds a minimal Peer Up message.
func EncodePeerUp(peer PerPeerHeader, local net.IP, localPort, remotePort uint16, sentOPEN, recvOPEN []byte) []byte {
	body := encodePerPeerHeader(peer)
	addr := encodePeerIP(local, peer.IsIPv6())
	body = append(body, addr[:]...)
	ports := make([]byte, 4)
	binary.BigEndian.PutUint16(ports[0:2], localPort)
	binary.BigEndian.PutUint16(ports[2:4], remotePort)
	body = append(body, ports...)
	body = append(body, sentOPEN...)
	body = append(body, recvOPEN...)
	return wrapCommon(MsgPeerUp, body)
}

// EncodePeerDown builds a Peer Down with reason and optional data.
func EncodePeerDown(peer PerPeerHeader, reason uint8, data []byte) []byte {
	body := encodePerPeerHeader(peer)
	body = append(body, reason)
	body = append(body, data...)
	return wrapCommon(MsgPeerDown, body)
}

// EncodeStatsReport builds a Statistics Report carrying the given TLVs.
// Peer Flags O bit MUST be 0 (cleared here). Loc-RIB peers are rejected.
func EncodeStatsReport(peer PerPeerHeader, tlvs []stats.TLV) ([]byte, error) {
	if peer.PeerType == PeerTypeLocRIB {
		return nil, fmt.Errorf("bmp: PDU counters MUST NOT be sent for Loc-RIB peer (type 3)")
	}
	peer.PeerFlags &^= FlagO
	body := encodePerPeerHeader(peer)
	count := make([]byte, 4)
	binary.BigEndian.PutUint32(count, uint32(len(tlvs)))
	body = append(body, count...)
	for _, tlv := range tlvs {
		chunk, err := stats.EncodeTLV(tlv.Type, valueOf(tlv.Data), stats.Width(len(tlv.Data)))
		if err != nil {
			return nil, err
		}
		body = append(body, chunk...)
	}
	return wrapCommon(MsgStatsReport, body), nil
}

func valueOf(data []byte) uint64 {
	switch len(data) {
	case 4:
		return uint64(binary.BigEndian.Uint32(data))
	case 8:
		return binary.BigEndian.Uint64(data)
	default:
		return 0
	}
}

func encodePerPeerHeader(p PerPeerHeader) []byte {
	b := make([]byte, PerPeerHeaderLen)
	b[0] = p.PeerType
	b[1] = p.PeerFlags
	copy(b[2:10], p.PeerDistinguisher[:])
	addr := encodePeerIP(p.PeerAddress, p.IsIPv6())
	copy(b[10:26], addr[:])
	binary.BigEndian.PutUint32(b[26:30], p.PeerAS)
	bgp := p.PeerBGPID.To4()
	if bgp == nil {
		bgp = net.IPv4zero
	}
	copy(b[30:34], bgp)
	if !p.Timestamp.IsZero() {
		binary.BigEndian.PutUint32(b[34:38], uint32(p.Timestamp.Unix()))
		binary.BigEndian.PutUint32(b[38:42], uint32(p.Timestamp.Nanosecond()/1000))
	}
	return b
}

func wrapCommon(msgType uint8, body []byte) []byte {
	out := make([]byte, CommonHeaderLen+len(body))
	out[0] = Version
	binary.BigEndian.PutUint32(out[1:5], uint32(len(out)))
	out[5] = msgType
	copy(out[CommonHeaderLen:], body)
	return out
}

// MinimalOPEN builds a minimal BGP OPEN PDU for Peer Up demos.
func MinimalOPEN(myAS uint16, holdTime uint16, bgpID net.IP) []byte {
	// Marker (16) + Length (2) + Type (1) + Version (1) + MyAS (2) + Hold (2) + BGP ID (4) + OptLen (1)
	msg := make([]byte, 29)
	for i := 0; i < 16; i++ {
		msg[i] = 0xff
	}
	binary.BigEndian.PutUint16(msg[16:18], 29)
	msg[18] = 1 // OPEN
	msg[19] = 4 // version
	binary.BigEndian.PutUint16(msg[20:22], myAS)
	binary.BigEndian.PutUint16(msg[22:24], holdTime)
	ip4 := bgpID.To4()
	if ip4 == nil {
		ip4 = net.IPv4(1, 1, 1, 1)
	}
	copy(msg[24:28], ip4)
	msg[28] = 0 // opt param len
	return msg
}
