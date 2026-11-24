// bmp-collector receives BMP sessions and exposes BGP PDU counters
// from draft-choudhary-grow-bmp-bgp-msg-stats-00 via a dashboard and JSON API.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/asechoud/bmp-bgp-msg-stats/pkg/bmp"
	"github.com/asechoud/bmp-bgp-msg-stats/pkg/stats"
	"github.com/asechoud/bmp-bgp-msg-stats/pkg/store"
	"github.com/asechoud/bmp-bgp-msg-stats/web"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:11019", "BMP listen address")
	httpAddr := flag.String("http", "127.0.0.1:18080", "HTTP dashboard/API listen address")
	typeMap := flag.String("types", "", "optional remapping TBD1=N,TBD2=N,... for IANA-assigned codes")
	flag.Parse()

	reg := stats.DefaultRegistry()
	if *typeMap != "" {
		overrides, err := parseTypeMap(*typeMap)
		if err != nil {
			log.Fatalf("types: %v", err)
		}
		reg = remappedRegistry(overrides)
	}

	st := store.New(reg)

	go serveHTTP(*httpAddr, st, reg)
	log.Printf("bmp-collector: implementing %s", stats.DraftReference)
	log.Printf("BMP listening on %s", *listen)
	log.Printf("dashboard on http://%s/", *httpAddr)

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handleConn(conn, st, reg)
	}
}

func handleConn(conn net.Conn, st *store.Store, reg stats.Registry) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	log.Printf("BMP session from %s", remote)
	_ = conn.SetDeadline(time.Time{})

	for {
		msg, err := bmp.ReadMessage(conn)
		if err != nil {
			log.Printf("%s: %v", remote, err)
			return
		}
		switch msg.Header.Type {
		case bmp.MsgInitiation:
			sysName, sysDescr := "", ""
			for _, t := range msg.Info {
				switch t.Type {
				case bmp.InfoSysName:
					sysName = string(t.Info)
				case bmp.InfoSysDescr:
					sysDescr = string(t.Info)
				}
			}
			st.OnInitiation(remote, sysName, sysDescr)
			log.Printf("%s: Initiation sysName=%q", remote, sysName)
		case bmp.MsgPeerUp:
			if msg.Peer != nil {
				st.OnPeerUp(remote, *msg.Peer)
				log.Printf("%s: Peer Up %s AS%d", remote, msg.Peer.PeerAddress, msg.Peer.PeerAS)
			}
		case bmp.MsgPeerDown:
			if msg.Peer != nil {
				st.OnPeerDown(remote, *msg.Peer)
				log.Printf("%s: Peer Down %s reason=%d", remote, msg.Peer.PeerAddress, msg.PeerDownReason)
			}
		case bmp.MsgStatsReport:
			if msg.Peer == nil {
				continue
			}
			if msg.Peer.PeerType == bmp.PeerTypeLocRIB {
				log.Printf("%s: ignoring PDU stats for Loc-RIB peer", remote)
				continue
			}
			if msg.Peer.IsAdjRIBOut() {
				log.Printf("%s: Statistics Report O flag set; clearing for PDU counter view", remote)
			}
			c, err := reg.ParseTLVs(msg.Stats)
			if err != nil {
				log.Printf("%s: stats parse: %v", remote, err)
				continue
			}
			if len(c.Present) == 0 {
				continue
			}
			st.OnStats(remote, *msg.Peer, c)
			log.Printf("%s: Stats %s updates_in=%d ka_in=%d total_in=%d",
				remote, msg.Peer.PeerAddress, c.UpdateReceived, c.KeepaliveReceived, c.TotalReceived)
		case bmp.MsgTermination:
			log.Printf("%s: Termination", remote)
			return
		}
	}
}

func serveHTTP(addr string, st *store.Store, reg stats.Registry) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(web.DashboardHTML)
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"ok":    true,
			"draft": stats.DraftReference,
		})
	})
	mux.HandleFunc("/api/types", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, stats.DefaultSpecs())
	})
	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		routers, peers := st.Snapshot()
		writeJSON(w, map[string]any{
			"draft":   stats.DraftReference,
			"types":   typeTable(reg),
			"routers": routers,
			"peers":   peers,
		})
	})
	log.Fatal(http.ListenAndServe(addr, mux))
}

func typeTable(reg stats.Registry) []map[string]any {
	var out []map[string]any
	for _, s := range stats.DefaultSpecs() {
		spec := s
		for _, cand := range reg {
			if cand.TBD == s.TBD {
				spec = cand
				break
			}
		}
		out = append(out, map[string]any{
			"tbd":         spec.TBD,
			"type":        spec.Type,
			"name":        spec.Name,
			"description": spec.Description,
			"stat_len":    spec.Width,
		})
	}
	return out
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func parseTypeMap(s string) (map[string]uint16, error) {
	out := make(map[string]uint16)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("expected TBD1=44, got %q", part)
		}
		n, err := strconv.ParseUint(strings.TrimSpace(kv[1]), 10, 16)
		if err != nil {
			return nil, err
		}
		out[strings.ToUpper(strings.TrimSpace(kv[0]))] = uint16(n)
	}
	return out, nil
}

func remappedRegistry(overrides map[string]uint16) stats.Registry {
	reg := stats.DefaultRegistry()
	mapped := make(stats.Registry)
	for _, s := range stats.DefaultSpecs() {
		if n, ok := overrides[s.TBD]; ok {
			s.Type = stats.Type(n)
		}
		mapped[s.Type] = s
	}
	if len(mapped) == 0 {
		return reg
	}
	return mapped
}

func init() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}
