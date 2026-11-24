package bmp

import (
	"bytes"
	"net"
	"testing"
	"time"

	"github.com/asechoud/bmp-bgp-msg-stats/pkg/stats"
)

func TestStatsReportRoundTrip(t *testing.T) {
	reg := stats.DefaultRegistry()
	c := stats.NewCounters()
	c.UpdateReceived = 1000
	c.KeepaliveReceived = 50
	c.TotalReceived = 1060
	for _, s := range stats.DefaultSpecs() {
		c.Present[s.Type] = true
	}
	tlvs, err := c.Encode(reg)
	if err != nil {
		t.Fatal(err)
	}
	peer := PerPeerHeader{
		PeerType:    PeerTypeGlobal,
		PeerFlags:   FlagO, // must be cleared by EncodeStatsReport
		PeerAddress: net.IPv4(203, 0, 113, 1),
		PeerAS:      64500,
		PeerBGPID:   net.IPv4(203, 0, 113, 1),
		Timestamp:   time.Unix(1_700_000_000, 123_000_000),
	}
	msg, err := EncodeStatsReport(peer, tlvs)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadMessage(bytes.NewReader(msg))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Header.Type != MsgStatsReport {
		t.Fatalf("type %d", decoded.Header.Type)
	}
	if decoded.Peer.IsAdjRIBOut() {
		t.Fatal("O flag must be 0 on Statistics Report")
	}
	got, err := reg.ParseTLVs(decoded.Stats)
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdateReceived != 1000 || got.KeepaliveReceived != 50 || got.TotalReceived != 1060 {
		t.Fatalf("counters %+v", got)
	}
}

func TestLocRIBRejected(t *testing.T) {
	peer := PerPeerHeader{PeerType: PeerTypeLocRIB, PeerAddress: net.IPv4(1, 1, 1, 1)}
	_, err := EncodeStatsReport(peer, nil)
	if err == nil {
		t.Fatal("expected Loc-RIB rejection")
	}
}

func TestInitiationAndPeerUp(t *testing.T) {
	initMsg := EncodeInitiation("sysDescr", "router1")
	m, err := ReadMessage(bytes.NewReader(initMsg))
	if err != nil {
		t.Fatal(err)
	}
	if m.Header.Type != MsgInitiation || len(m.Info) < 2 {
		t.Fatalf("initiation decode failed: %+v", m)
	}
	peer := PerPeerHeader{
		PeerType:    PeerTypeGlobal,
		PeerAddress: net.IPv4(10, 0, 0, 2),
		PeerAS:      65001,
		PeerBGPID:   net.IPv4(10, 0, 0, 2),
		Timestamp:   time.Now().UTC(),
	}
	open := MinimalOPEN(65000, 90, net.IPv4(192, 0, 2, 1))
	up := EncodePeerUp(peer, net.IPv4(192, 0, 2, 1), 179, 50000, open, open)
	m2, err := ReadMessage(bytes.NewReader(up))
	if err != nil {
		t.Fatal(err)
	}
	if m2.Header.Type != MsgPeerUp || m2.Peer.PeerAS != 65001 {
		t.Fatalf("peer up decode failed: %+v", m2)
	}
}
