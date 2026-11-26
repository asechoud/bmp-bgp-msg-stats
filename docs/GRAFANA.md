# Grafana integration

Two paths: **Prometheus** (Go `bmp-collector`) and **SQL** (OpenBMP → Kafka → DB).
See [ARCHITECTURE.md](ARCHITECTURE.md) for where Grafana sits in the overall pipeline.

## A. Go collector → Prometheus → Grafana

### 1. Run the collector

```bash
go run ./cmd/bmp-collector -listen 0.0.0.0:11019 -http 0.0.0.0:18080
go run ./cmd/bmp-simulator -collector 127.0.0.1:11019
```

Metrics: `http://127.0.0.1:18080/metrics`

| Metric | Type | Labels |
|--------|------|--------|
| `bmp_bgp_pdu_total` | counter | `router`, `peer_ip`, `peer_as`, `peer_bgp_id`, `message`, `direction` |
| `bmp_bgp_pdu_present` | gauge | same (1 if type present in last report) |
| `bmp_bgp_peer_up` | gauge | peer labels |
| `bmp_bgp_routers` | gauge | — |

`message` ∈ `update`, `keepalive`, `notification`, `route_refresh`, `total`  
`direction` ∈ `received`, `sent`

### 2. Start Prometheus + Grafana

```bash
docker compose -f grafana/docker-compose.yml up -d
```

- Grafana: http://127.0.0.1:13000 (`admin` / `admin`)
- Dashboard folder **BMP** → **BMP BGP PDU Stats**
- Prometheus: http://127.0.0.1:19090

If scrape fails on Linux, set the target in `grafana/prometheus.yml` to your host IP instead of `host.docker.internal`.

### 3. Useful PromQL

```promql
# UC2 churn
rate(bmp_bgp_pdu_total{message="update"}[5m])

# UC1 hold-timer context
rate(bmp_bgp_pdu_total{message="keepalive",direction="received"}[5m])

# UC3 notifications (cumulative across flaps)
bmp_bgp_pdu_total{message="notification"}

# UC4 refresh storms
rate(bmp_bgp_pdu_total{message="route_refresh"}[5m])

# Only chart types that were present
rate(bmp_bgp_pdu_total{message="update"}[5m])
  and on(peer_ip,direction) (bmp_bgp_pdu_present{message="update"} == 1)
```

Dashboard JSON: [`grafana/dashboards/bmp-bgp-pdu-stats.json`](../grafana/dashboards/bmp-bgp-pdu-stats.json)

---

## B. OpenBMP → Kafka → MySQL/Postgres → Grafana

```
Router → openbmpd → Kafka bmp_stat → DB consumer → Grafana SQL
```

Upstream dashboards: [OpenBMP/obmp-grafana](https://github.com/OpenBMP/obmp-grafana).

This fork adds `bmp_stat` fields 18–28 (see `openbmp-collector/docs/MESSAGE_BUS_API.md`). Consumers that only map fields 1–17 need a schema extension before these queries work.

### Example schema extension (MySQL-style)

```sql
ALTER TABLE bmp_stat_reports
  ADD COLUMN updates_received BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN updates_sent BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN keepalives_received INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN keepalives_sent INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN notifications_received INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN notifications_sent INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN route_refresh_received INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN route_refresh_sent INT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN total_received BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN total_sent BIGINT UNSIGNED NOT NULL DEFAULT 0,
  ADD COLUMN pdu_stats_present SMALLINT UNSIGNED NOT NULL DEFAULT 0;
```

Exact table/column names depend on your consumer; adjust to match.

### Example Grafana SQL panels

**UPDATE receive rate (approx per interval):**

```sql
SELECT
  $__timeGroup(timestamp, $__interval) AS time,
  peer_ip,
  MAX(updates_received) - MIN(updates_received) AS updates_delta
FROM bmp_stat_reports
WHERE $__timeFilter(timestamp)
  AND (pdu_stats_present & 1) = 1   -- TBD1 present
GROUP BY 1, peer_ip
ORDER BY 1;
```

**NOTIFICATION cumulative (UC3):**

```sql
SELECT
  timestamp AS time,
  peer_ip,
  notifications_received,
  notifications_sent
FROM bmp_stat_reports
WHERE $__timeFilter(timestamp)
  AND (pdu_stats_present & 16) = 16  -- TBD5 present (bit 4)
ORDER BY timestamp;
```

**Presence bitmask** (draft §5.1): bit 0 = TBD1 … bit 9 = TBD10. Always filter with `pdu_stats_present` so omitted types are not plotted as zero.

### Wire Grafana

1. Add MySQL/Postgres datasource to the OpenBMP DB.
2. Import or clone panels from `obmp-grafana`, then add PDU panels using the SQL above.
3. Prefer rate-from-delta over absolute counters for UPDATE/KA/RR/total; keep NOTIFICATION as a cumulative series.
