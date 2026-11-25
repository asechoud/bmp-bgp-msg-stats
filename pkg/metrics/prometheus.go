// Package metrics exposes BGP PDU counters for Prometheus / Grafana scraping.
package metrics

import (
	"strconv"

	"github.com/asechoud/bmp-bgp-msg-stats/pkg/stats"
	"github.com/asechoud/bmp-bgp-msg-stats/pkg/store"
	"github.com/prometheus/client_golang/prometheus"
)

// Collector implements prometheus.Collector for the in-memory BMP store.
type Collector struct {
	store *store.Store

	pdu     *prometheus.Desc
	present *prometheus.Desc
	peerUp  *prometheus.Desc
	routers *prometheus.Desc
}

// New returns a Collector bound to st.
func New(st *store.Store) *Collector {
	labels := []string{"router", "peer_ip", "peer_as", "peer_bgp_id", "message", "direction"}
	return &Collector{
		store: st,
		pdu: prometheus.NewDesc(
			"bmp_bgp_pdu_total",
			"Cumulative BGP PDU counter from draft-choudhary-grow-bmp-bgp-msg-stats-00. Use rate()/increase() in Grafana.",
			labels, nil,
		),
		present: prometheus.NewDesc(
			"bmp_bgp_pdu_present",
			"1 if the PDU counter type was present in the last Statistics Report (draft §5.1: omitted ≠ zero).",
			labels, nil,
		),
		peerUp: prometheus.NewDesc(
			"bmp_bgp_peer_up",
			"1 if the monitored BGP peer is Established according to Peer Up/Down.",
			[]string{"router", "peer_ip", "peer_as", "peer_bgp_id"}, nil,
		),
		routers: prometheus.NewDesc(
			"bmp_bgp_routers",
			"Number of connected BMP routers (speakers).",
			nil, nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.pdu
	ch <- c.present
	ch <- c.peerUp
	ch <- c.routers
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	routers, peers := c.store.Snapshot()
	ch <- prometheus.MustNewConstMetric(c.routers, prometheus.GaugeValue, float64(len(routers)))

	for _, p := range peers {
		as := strconv.FormatUint(uint64(p.PeerAS), 10)
		base := []string{p.Router, p.PeerAddress, as, p.PeerBGPID}
		up := 0.0
		if p.Up {
			up = 1
		}
		ch <- prometheus.MustNewConstMetric(c.peerUp, prometheus.GaugeValue, up, base...)

		ctr := p.Counters
		emit := func(message, direction string, value float64, typeCode stats.Type) {
			lbls := append(append([]string{}, base...), message, direction)
			ch <- prometheus.MustNewConstMetric(c.pdu, prometheus.CounterValue, value, lbls...)
			pres := 0.0
			if ctr.Present[typeCode] {
				pres = 1
			}
			ch <- prometheus.MustNewConstMetric(c.present, prometheus.GaugeValue, pres, lbls...)
		}

		emit("update", "received", float64(ctr.UpdateReceived), stats.TypeUpdateReceived)
		emit("update", "sent", float64(ctr.UpdateSent), stats.TypeUpdateSent)
		emit("keepalive", "received", float64(ctr.KeepaliveReceived), stats.TypeKeepaliveReceived)
		emit("keepalive", "sent", float64(ctr.KeepaliveSent), stats.TypeKeepaliveSent)
		emit("notification", "received", float64(ctr.NotificationReceived), stats.TypeNotificationReceived)
		emit("notification", "sent", float64(ctr.NotificationSent), stats.TypeNotificationSent)
		emit("route_refresh", "received", float64(ctr.RouteRefreshReceived), stats.TypeRouteRefreshReceived)
		emit("route_refresh", "sent", float64(ctr.RouteRefreshSent), stats.TypeRouteRefreshSent)
		emit("total", "received", float64(ctr.TotalReceived), stats.TypeTotalReceived)
		emit("total", "sent", float64(ctr.TotalSent), stats.TypeTotalSent)
	}
}
