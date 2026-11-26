# Architecture

Layered view of how this repository turns BGP session activity into
observability signals for
[draft-choudhary-grow-bmp-bgp-msg-stats-00](https://datatracker.ietf.org/doc/html/draft-choudhary-grow-bmp-bgp-msg-stats-00).

Diagram style follows the same narrative used in large observability
platform write-ups: **ingestion → processing → storage / fan-out → query &
visualization**, with what this repo actually implements called out in each
layer.

## 1. Platform overview (two deployable paths)

Both paths share the same BMP Statistics Report encoding (provisional Stat
Types **44–53** / TBD1–TBD10). They diverge after the collector:

| Path | Ingest | Fan-out | Visualize |
|------|--------|---------|-----------|
| **OpenBMP** (primary) | `openbmpd` (BMPv3/v4) | Kafka `bmp_stat` → DB consumer | Grafana SQL ([docs/GRAFANA.md](GRAFANA.md)) |
| **Go reference** | `bmp-collector` (BMPv3/v4 + E-bit) | In-memory store + `/metrics` | Built-in UI + Prometheus → Grafana |

BMPv4 Stats Reports enclose Stats Count/data in Stats TLV type 1. Enterprise
process CPU/memory gauges use the E-bit + IANA PEN (see [IMPLEMENTATION.md](IMPLEMENTATION.md)).

```mermaid
flowchart TB
  subgraph Sources["Sources"]
    R1["BGP routers<br/>BMP speakers"]
    SIM["bmp-simulator<br/>(lab / demo)"]
  end

  subgraph Ingest["Layer 1 — Ingestion (BMP / TCP)"]
    BMP["BMP v3 session<br/>Initiation · Peer Up/Down · Statistics Report"]
  end

  subgraph Process["Layer 2 — Processing"]
    direction TB
    OBMP["openbmp-collector<br/>parseBMP · handleStatsReport<br/>types 44–53 + presence mask"]
    GO["bmp-collector (Go)<br/>pkg/bmp · pkg/stats · pkg/rate · pkg/store"]
  end

  subgraph Fanout["Layer 3 — Storage / fan-out"]
    KFK["Apache Kafka<br/>openbmp.parsed.bmp_stat<br/>fields 18–28"]
    DB["MySQL / Postgres<br/>consumer schema"]
    MEM["In-memory peer store"]
    PROM["Prometheus<br/>scrape /metrics"]
  end

  subgraph Viz["Layer 4 — Query & visualization"]
    GF_SQL["Grafana<br/>SQL panels"]
    GF_PROM["Grafana<br/>PromQL dashboard"]
    UI["Built-in dashboard<br/>:18080/"]
    API["JSON API<br/>/api/snapshot"]
  end

  R1 --> BMP
  SIM --> BMP
  BMP --> OBMP
  BMP --> GO

  OBMP --> KFK --> DB --> GF_SQL
  GO --> MEM
  MEM --> UI
  MEM --> API
  MEM --> PROM --> GF_PROM
```

## 2. OpenBMP production pipeline

Matches the classic OpenBMP collector flow, extended only on the **stats**
path for PDU counters (RIBs / Peer Up-Down continue unchanged).

```mermaid
flowchart LR
  subgraph Edge["Network edge"]
    RT["Router BMP station"]
  end

  subgraph Collector["openbmp-collector"]
    L["BMPListener"]
    R["BMPReader"]
    P["parseBMP"]
    S["handleStatsReport()"]
    M["MsgBusImpl_kafka<br/>add_StatReport()"]
    L --> R --> P
    P -->|TYPE_STATS_REPORT| S --> M
  end

  subgraph Bus["Message bus"]
    T["Kafka topic<br/>bmp_stat"]
  end

  subgraph Downstream["Consumers"]
    C["openbmp-*-consumer"]
    SQL[(Relational DB)]
    G["Grafana<br/>obmp-grafana + PDU panels"]
  end

  RT -->|TCP :11019-ish| L
  M --> T --> C --> SQL --> G
```

**PDU-specific processing inside `handleStatsReport`:**

```mermaid
flowchart TB
  SR["Statistics Report TLVs"]
  SR --> T0["RFC 7854 types 0–8<br/>existing RIB / policy stats"]
  SR --> T44["Types 44–53 (provisional)<br/>TBD1–TBD10 PDU counters"]
  T44 --> VAL{"Stat Len<br/>4 or 8 as required?"}
  VAL -->|yes| FILL["Fill obj_stats_report<br/>UPDATE / KA / NOTIF / RR / total"]
  VAL -->|no| SKIP["Ignore that type"]
  FILL --> BIT["Set bit in pdu_stats_present<br/>omitted ≠ zero §5.1"]
  BIT --> ROW["Kafka TSV row<br/>fields 1–17 + 18–28"]
```

## 3. Go reference pipeline (lab / light deploy)

No Kafka. Suitable for demos, unit testing the draft encoding, and a small
Prometheus scrape target.

```mermaid
flowchart TB
  subgraph Speakers["BMP speakers"]
    SIM["bmp-simulator"]
    RT["Router optional"]
  end

  subgraph Collector["cmd/bmp-collector"]
    TCP[":11019 BMP listen"]
    DEC["pkg/bmp ReadMessage"]
    ST["pkg/stats ParseTLVs"]
    RATE["pkg/rate Diff<br/>wrap / discontinuity"]
    STORE["pkg/store Snapshot"]
    MET["pkg/metrics<br/>Prometheus Collector"]
    HTTP[":18080 HTTP"]
    TCP --> DEC --> ST --> RATE --> STORE
    STORE --> MET
    STORE --> HTTP
  end

  subgraph Clients["Operators / scrapers"]
    UI["GET /"]
    SNAP["GET /api/snapshot"]
    M["GET /metrics"]
    P["Prometheus"]
    GF["Grafana<br/>grafana/dashboards/"]
  end

  SIM --> TCP
  RT --> TCP
  HTTP --> UI
  HTTP --> SNAP
  HTTP --> M --> P --> GF
  MET --> M
```

## 4. What is ingested vs what is not

Aligned with draft scope (session-layer PDU counters only):

```mermaid
flowchart LR
  subgraph In["In scope for this draft"]
    U["UPDATE PDUs in/out"]
    K["KEEPALIVE in/out"]
    N["NOTIFICATION in/out"]
    RR["ROUTE-REFRESH in/out"]
    T["Total BGP PDUs in/out"]
  end

  subgraph Out["Out of scope here"]
    NLRI["NLRI / prefix counts"]
    HOLD["Hold / keepalive intervals"]
    LAST["Last NOTIFICATION body"]
    CPU["CPU / platform metrics"]
    KA_RELAY["Relaying KEEPALIVE via Route Monitoring"]
  end

  BMP["BMP Statistics Report"] --> In
  BMP -.-> Out
```

Peer Down does **not** clear PDU counters (draft §7). Monitoring stations
derive rates from successive samples and treat decreases / new Initiation as
discontinuities (`pkg/rate`, OpenBMP consumers must do the same).

## 5. Component map (repository)

```mermaid
flowchart TB
  subgraph Repo["asechoud/bmp-bgp-msg-stats"]
    subgraph OB["openbmp-collector/"]
      OB1["Server/src/bmp/parseBMP.*"]
      OB2["Server/src/MsgBusInterface.hpp"]
      OB3["Server/src/kafka/MsgBusImpl_kafka.cpp"]
      OB4["docs/BGP_PDU_STATS.md"]
    end
    subgraph Go["Go module"]
      G1["pkg/bmp · pkg/stats · pkg/rate · pkg/store · pkg/metrics"]
      G2["cmd/bmp-collector · cmd/bmp-simulator"]
      G3["web/static dashboard"]
    end
    subgraph Obs["Observability packaging"]
      O1["grafana/ docker-compose + dashboard JSON"]
      O2["docs/GRAFANA.md"]
    end
  end

  OB1 --> OB2 --> OB3
  OB4 -.-> OB1
  G1 --> G2 --> G3
  G1 --> O1
  O2 -.-> O1
```

## Related docs

- [IMPLEMENTATION.md](IMPLEMENTATION.md) — draft section coverage
- [GRAFANA.md](GRAFANA.md) — Prometheus + OpenBMP SQL panels
- [openbmp-collector/docs/BGP_PDU_STATS.md](../openbmp-collector/docs/BGP_PDU_STATS.md) — collector behavior for types 44–53
- [openbmp-collector/docs/MESSAGE_BUS_API.md](../openbmp-collector/docs/MESSAGE_BUS_API.md) — `bmp_stat` field list
