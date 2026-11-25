// Package store keeps the latest PDU counters per BMP peer for the
// collector dashboard and JSON API.
package store

import (
	"net"
	"sync"
	"time"

	"github.com/asechoud/bmp-bgp-msg-stats/pkg/bmp"
	"github.com/asechoud/bmp-bgp-msg-stats/pkg/rate"
	"github.com/asechoud/bmp-bgp-msg-stats/pkg/stats"
)

// PeerSnapshot is the latest view of one monitored BGP peer.
type PeerSnapshot struct {
	Key              string            `json:"key"`
	Router           string            `json:"router"`
	PeerType         uint8             `json:"peer_type"`
	PeerAddress      string            `json:"peer_address"`
	PeerAS           uint32            `json:"peer_as"`
	PeerBGPID        string            `json:"peer_bgp_id"`
	Up               bool              `json:"up"`
	LastPeerEvent    string            `json:"last_peer_event"`
	Timestamp        time.Time         `json:"timestamp"`
	Counters         stats.Counters    `json:"counters"`
	Rates            map[string]float64 `json:"rates_per_sec"`
	Discontinuities  []string          `json:"discontinuities,omitempty"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// RouterInfo is metadata from Initiation.
type RouterInfo struct {
	Remote    string    `json:"remote"`
	SysName   string    `json:"sys_name"`
	SysDescr  string    `json:"sys_descr"`
	Connected time.Time `json:"connected_at"`
}

type peerState struct {
	snap     PeerSnapshot
	prev     stats.Counters
	prevAt   time.Time
	hasPrev  bool
}

// Store is a concurrency-safe in-memory collector state.
type Store struct {
	mu      sync.RWMutex
	routers map[string]RouterInfo
	peers   map[string]*peerState
	reg     stats.Registry
}

// New creates an empty Store.
func New(reg stats.Registry) *Store {
	return &Store{
		routers: make(map[string]RouterInfo),
		peers:   make(map[string]*peerState),
		reg:     reg,
	}
}

// OnInitiation records a new BMP session (Section 7 discontinuity).
func (s *Store) OnInitiation(remote, sysName, sysDescr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routers[remote] = RouterInfo{
		Remote:    remote,
		SysName:   sysName,
		SysDescr:  sysDescr,
		Connected: time.Now().UTC(),
	}
	for key, st := range s.peers {
		if st.snap.Router == remote {
			st.snap.Discontinuities = append(st.snap.Discontinuities, rate.SessionDiscontinuity())
			st.hasPrev = false
			s.peers[key] = st
		}
	}
}

// OnPeerUp marks the peer Established.
func (s *Store) OnPeerUp(remote string, peer bmp.PerPeerHeader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := remote + "|" + peer.PeerKey()
	st := s.peers[key]
	if st == nil {
		st = &peerState{snap: PeerSnapshot{Key: key, Counters: stats.NewCounters(), Rates: map[string]float64{}}}
		s.peers[key] = st
	}
	st.snap.Router = remote
	st.snap.PeerType = peer.PeerType
	st.snap.PeerAddress = peer.PeerAddress.String()
	st.snap.PeerAS = peer.PeerAS
	st.snap.PeerBGPID = peer.PeerBGPID.String()
	st.snap.Up = true
	st.snap.LastPeerEvent = "peer_up"
	st.snap.UpdatedAt = time.Now().UTC()
	// Peer Down → Peer Up is NOT by itself a counter discontinuity (Section 7).
}

// OnPeerDown marks the peer down without clearing counters (Section 7).
func (s *Store) OnPeerDown(remote string, peer bmp.PerPeerHeader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := remote + "|" + peer.PeerKey()
	st := s.peers[key]
	if st == nil {
		st = &peerState{snap: PeerSnapshot{Key: key, Counters: stats.NewCounters(), Rates: map[string]float64{}}}
		s.peers[key] = st
	}
	st.snap.Router = remote
	st.snap.PeerType = peer.PeerType
	st.snap.PeerAddress = peer.PeerAddress.String()
	st.snap.PeerAS = peer.PeerAS
	st.snap.PeerBGPID = peer.PeerBGPID.String()
	st.snap.Up = false
	st.snap.LastPeerEvent = "peer_down"
	st.snap.UpdatedAt = time.Now().UTC()
}

// OnStats applies a Statistics Report. Loc-RIB peers are ignored.
func (s *Store) OnStats(remote string, peer bmp.PerPeerHeader, c stats.Counters) {
	if peer.PeerType == bmp.PeerTypeLocRIB {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := remote + "|" + peer.PeerKey()
	st := s.peers[key]
	if st == nil {
		st = &peerState{snap: PeerSnapshot{Key: key, Rates: map[string]float64{}}}
		s.peers[key] = st
	}
	ts := peer.Timestamp
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	rates := map[string]float64{}
	var discs []string
	if st.hasPrev {
		for _, spec := range stats.DefaultSpecs() {
			mapped := spec
			if s2, ok := s.reg[spec.Type]; ok {
				mapped = s2
			} else {
				for _, cand := range s.reg {
					if cand.TBD == spec.TBD {
						mapped = cand
						break
					}
				}
			}
			prevS := sampleOf(st.prev, st.prevAt, mapped)
			currS := sampleOf(c, ts, mapped)
			d := rate.Diff(prevS, currS)
			if d.Discontinuity {
				discs = append(discs, mapped.Name+": "+d.Reason)
				continue
			}
			if currS.Present && prevS.Present {
				rates[mapped.Name] = d.RatePerSecond
			}
		}
	}
	st.prev = st.snap.Counters
	st.prevAt = st.snap.Timestamp
	if !st.hasPrev {
		st.prev = c
		st.prevAt = ts
	}
	st.hasPrev = true
	st.snap.Router = remote
	st.snap.PeerType = peer.PeerType
	st.snap.PeerAddress = peer.PeerAddress.String()
	st.snap.PeerAS = peer.PeerAS
	st.snap.PeerBGPID = peer.PeerBGPID.String()
	st.snap.Timestamp = ts
	st.snap.Counters = c
	st.snap.Rates = rates
	if len(discs) > 0 {
		st.snap.Discontinuities = append(st.snap.Discontinuities, discs...)
	}
	st.snap.UpdatedAt = time.Now().UTC()
}

func sampleOf(c stats.Counters, at time.Time, spec stats.Spec) rate.Sample {
	s := rate.Sample{At: at, Width: spec.Width, Present: c.Present[spec.Type]}
	if !s.Present {
		// Also accept Present keyed by default type when remapped.
		for t, ok := range c.Present {
			if !ok {
				continue
			}
			if t == spec.Type {
				s.Present = true
				break
			}
		}
	}
	switch spec.TBD {
	case "TBD1":
		s.Value = c.UpdateReceived
	case "TBD2":
		s.Value = c.UpdateSent
	case "TBD3":
		s.Value = uint64(c.KeepaliveReceived)
	case "TBD4":
		s.Value = uint64(c.KeepaliveSent)
	case "TBD5":
		s.Value = uint64(c.NotificationReceived)
	case "TBD6":
		s.Value = uint64(c.NotificationSent)
	case "TBD7":
		s.Value = uint64(c.RouteRefreshReceived)
	case "TBD8":
		s.Value = uint64(c.RouteRefreshSent)
	case "TBD9":
		s.Value = c.TotalReceived
	case "TBD10":
		s.Value = c.TotalSent
	}
	return s
}

// Snapshot returns a copy of all routers and peers.
func (s *Store) Snapshot() (routers []RouterInfo, peers []PeerSnapshot) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.routers {
		routers = append(routers, r)
	}
	for _, st := range s.peers {
		peers = append(peers, st.snap)
	}
	return routers, peers
}

// EnsureIP normalizes an IP for display.
func EnsureIP(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}
