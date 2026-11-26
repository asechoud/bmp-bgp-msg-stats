# BGP Session PDU Counters

Support for [draft-choudhary-grow-bmp-bgp-msg-stats-00](https://datatracker.ietf.org/doc/html/draft-choudhary-grow-bmp-bgp-msg-stats-00).

End-to-end diagrams (OpenBMP path and PDU TLV processing):
[../../docs/ARCHITECTURE.md](../../docs/ARCHITECTURE.md).

## Collector behavior

- Accepts BMP **version 3 and 4** common headers
- BMPv4: unwraps mandatory Stats TLV (type 1) before Stats Count / stats data
- Parses Statistics Report TLVs with provisional Stat Types **44–53**
- Enforces expected Stat Len (8 for TBD1/2/9/10, 4 for TBD3–8); wrong lengths are ignored for that type
- Sets `pdu_stats_present` bits when a draft type was present (omitted ≠ zero)
- **E-bit** enterprise stats: when Stat Type MSB is set, reads 4-byte PEN then Stat Data
- Enterprise process resource gauges (PEN **9**, types **1** / **2**): process CPU and memory utilization in hundredths of a percent → `process_cpu_hundredths`, `process_memory_hundredths`, `resource_pen`, `resource_stats_present`
- Does not change handling of RFC 7854 types 0–8
- Loc-RIB peer filtering remains the speaker’s responsibility; the collector records what it receives

## Kafka `bmp_stat` fields

See [MESSAGE_BUS_API.md](MESSAGE_BUS_API.md) fields 18–28 (PDU counters) and
29–32 (process CPU / memory / PEN / presence).

## Remapping IANA codes

When IANA assigns TBD1–TBD10, update the constants in
`Server/src/bmp/parseBMP.h` (`STATS_UPDATE_RECEIVED` … `STATS_TOTAL_SENT`).
