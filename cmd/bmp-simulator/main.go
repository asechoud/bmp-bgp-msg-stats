// bmp-simulator is a demo BMP speaker that emits Initiation, Peer Up,
// and Statistics Reports carrying draft-choudhary-grow-bmp-bgp-msg-stats-00
// PDU counters. Useful for local testing of bmp-collector.
package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asechoud/bmp-bgp-msg-stats/pkg/bmp"
	"github.com/asechoud/bmp-bgp-msg-stats/pkg/stats"
)

func main() {
	collector := flag.String("collector", "127.0.0.1:11019", "BMP collector address")
	interval := flag.Duration("interval", 5*time.Second, "Statistics Report interval")
	peers := flag.Int("peers", 2, "number of simulated BGP peers")
	sysName := flag.String("sysname", "bmp-sim", "sysName in Initiation")
	flag.Parse()

	reg := stats.DefaultRegistry()

	conn, err := net.Dial("tcp", *collector)
	if err != nil {
		log.Fatalf("dial collector: %v", err)
	}
	defer conn.Close()
	log.Printf("connected to %s (%s)", *collector, stats.DraftReference)

	initMsg := bmp.EncodeInitiation(
		"bmp-bgp-msg-stats simulator; implements "+stats.DraftReference,
		*sysName,
	)
	if _, err := conn.Write(initMsg); err != nil {
		log.Fatal(err)
	}

	ps := make([]*simPeer, 0, *peers)
	for i := 0; i < *peers; i++ {
		addr := net.IPv4(10, 0, 0, byte(i+1))
		bgpID := net.IPv4(10, 0, 0, byte(i+1))
		h := bmp.PerPeerHeader{
			PeerType:    bmp.PeerTypeGlobal,
			PeerFlags:   0, // O flag MUST be 0
			PeerAddress: addr,
			PeerAS:      uint32(65000 + i),
			PeerBGPID:   bgpID,
			Timestamp:   time.Now().UTC(),
		}
		sent := bmp.MinimalOPEN(65000, 90, net.IPv4(192, 0, 2, 1))
		recv := bmp.MinimalOPEN(uint16(65000+i), 90, bgpID)
		up := bmp.EncodePeerUp(h, net.IPv4(192, 0, 2, 1), 179, 49152, sent, recv)
		if _, err := conn.Write(up); err != nil {
			log.Fatal(err)
		}
		c := stats.NewCounters()
		for _, s := range stats.DefaultSpecs() {
			c.Present[s.Type] = true
		}
		// Seed like the draft §5.5 example on peer 0.
		if i == 0 {
			c.UpdateReceived = 1000
			c.KeepaliveReceived = 50
			c.TotalReceived = 1060
			c.UpdateSent = 800
			c.KeepaliveSent = 50
			c.TotalSent = 860
		}
		ps = append(ps, &simPeer{header: h, c: c})
		log.Printf("Peer Up %s AS%d", addr, 65000+i)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			log.Printf("shutting down")
			return
		case <-ticker.C:
			for i, p := range ps {
				bump(p, i)
				p.header.Timestamp = time.Now().UTC()
				tlvs, err := p.c.Encode(reg)
				if err != nil {
					log.Printf("encode: %v", err)
					continue
				}
				// Rebuild TLV Data from Encode which already set Data.
				rawTLVs := make([]stats.TLV, 0, len(tlvs))
				for _, t := range tlvs {
					rawTLVs = append(rawTLVs, t)
				}
				msg, err := bmp.EncodeStatsReport(p.header, rawTLVs)
				if err != nil {
					log.Printf("stats report: %v", err)
					continue
				}
				if _, err := conn.Write(msg); err != nil {
					log.Printf("write: %v", err)
					return
				}
				log.Printf("Stats %s updates_in=%d ka_in=%d notif_in=%d total_in=%d",
					p.header.PeerAddress, p.c.UpdateReceived, p.c.KeepaliveReceived,
					p.c.NotificationReceived, p.c.TotalReceived)
			}
		}
	}
}

type simPeer struct {
	header bmp.PerPeerHeader
	c      stats.Counters
}

func bump(p *simPeer, idx int) {
	// Quiet peer mostly keepalives (UC1/UC5); busy peer churns UPDATEs (UC2).
	ka := uint32(1)
	upd := uint64(0)
	rr := uint32(0)
	notif := uint32(0)
	if idx%2 == 0 {
		upd = 40
		if p.c.UpdateReceived%500 == 0 && p.c.UpdateReceived > 0 {
			rr = 1 // occasional refresh storm signal (UC4)
		}
	} else {
		upd = 2
		ka = 2
	}
	if p.c.NotificationReceived == 0 && p.c.UpdateReceived > 1500 {
		notif = 1 // one historical NOTIFICATION for UC3
	}
	p.c.UpdateReceived += upd
	p.c.UpdateSent += upd / 2
	p.c.KeepaliveReceived += ka
	p.c.KeepaliveSent += ka
	p.c.RouteRefreshReceived += rr
	p.c.RouteRefreshSent += rr
	p.c.NotificationReceived += notif
	p.c.NotificationSent += 0
	// Totals include OPEN historically + typed PDUs; keep a small OPEN offset.
	p.c.TotalReceived = p.c.UpdateReceived + uint64(p.c.KeepaliveReceived) +
		uint64(p.c.NotificationReceived) + uint64(p.c.RouteRefreshReceived) + 2
	p.c.TotalSent = p.c.UpdateSent + uint64(p.c.KeepaliveSent) +
		uint64(p.c.NotificationSent) + uint64(p.c.RouteRefreshSent) + 2
}
