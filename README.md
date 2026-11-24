# bmp-bgp-msg-stats

Open-source BMP tooling that implements
[draft-choudhary-grow-bmp-bgp-msg-stats-00](https://datatracker.ietf.org/doc/html/draft-choudhary-grow-bmp-bgp-msg-stats-00):
**BGP session PDU counters as BMP Statistics Types**.

The draft adds ten Statistics Report types so a monitoring station that already
receives RIBs and Peer Up/Down can also see session-layer load on the same BMP
feed — UPDATE, KEEPALIVE, NOTIFICATION, ROUTE-REFRESH, and total BGP PDUs in
each direction.

| TBD | Description | Stat Len |
|-----|-------------|----------|
| TBD1 / TBD2 | UPDATE received / sent | 8 (64-bit Counter) |
| TBD3 / TBD4 | KEEPALIVE received / sent | 4 (32-bit Counter) |
| TBD5 / TBD6 | NOTIFICATION received / sent | 4 (32-bit Counter) |
| TBD7 / TBD8 | ROUTE-REFRESH received / sent | 4 (32-bit Counter) |
| TBD9 / TBD10 | Total BGP PDUs received / sent | 8 (64-bit Counter) |

## What this repo provides

- **`pkg/stats`** — encode/decode for TBD1–TBD10, presence vs zero, 32/64-bit widths
- **`pkg/bmp`** — BMP v3 framing (RFC 7854): Initiation, Peer Up/Down, Statistics Report
- **`pkg/rate`** — rate derivation and discontinuity detection (draft §7–8)
- **`bmp-collector`** — TCP BMP receiver + HTTP dashboard/JSON API
- **`bmp-simulator`** — demo BMP speaker that emits the new counters

## Provisional Stat Type codes

IANA has not assigned TBD1–TBD10 yet. This implementation uses **provisional**
codes **44–53** (next free Standards Action values after RFC 9972). Remap when
early allocation lands:

```bash
bmp-collector -types TBD1=44,TBD2=45,TBD3=46,TBD4=47,TBD5=48,TBD6=49,TBD7=50,TBD8=51,TBD9=52,TBD10=53
```

## Requirements

- Go 1.22+

## Quick start

```bash
# Terminal 1 — collector (BMP :11019, dashboard :18080)
go run ./cmd/bmp-collector -listen 0.0.0.0:11019 -http 127.0.0.1:18080

# Terminal 2 — simulator
go run ./cmd/bmp-simulator -collector 127.0.0.1:11019 -interval 5s

# Open the dashboard
open http://127.0.0.1:18080/
```

JSON snapshot:

```bash
curl -s http://127.0.0.1:18080/api/snapshot | jq .
```

## Draft rules enforced here

- Counters are **per-peer**, session-global (not per-AFI/SAFI)
- **MUST NOT** send these types for Loc-RIB peers (Peer Type 3)
- Statistics Report **O flag MUST be 0**
- Each type appears **at most once** per report; omitted ≠ zero
- **Peer Down does not clear** counters (cumulative across establishments)
- Monitoring station tracks discontinuities (decreases, new Initiation, …)
- TBD9/TBD10 need not equal the sum of per-type counters (OPEN and other types)

## Point a real router at the collector

Configure the router’s BMP station toward the collector listen address.
When the speaker supports this draft, Statistics Reports will carry TBD1–TBD10
(or your remapped codes). Speakers that omit unsupported types are fine — the
collector treats absence as “not present,” not zero.

## Tests

```bash
go test ./...
```

## API

| Path | Description |
|------|-------------|
| `GET /` | Dashboard |
| `GET /api/health` | Liveness + draft id |
| `GET /api/types` | TBD1–TBD10 catalog |
| `GET /api/snapshot` | Routers, peers, counters, rates |

## Reference

- Draft: https://datatracker.ietf.org/doc/html/draft-choudhary-grow-bmp-bgp-msg-stats-00
- BMP: [RFC 7854](https://www.rfc-editor.org/rfc/rfc7854.html)
- Authors: Aseem Choudhary, Narasimha Prasad S N (Cisco Systems)

## License

BSD 3-Clause — see [LICENSE](LICENSE). Code components extracted from the
Internet-Draft include Revised BSD License text as described in the IETF Trust
Legal Provisions.
