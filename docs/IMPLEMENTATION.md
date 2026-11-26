# Implementation notes for draft-choudhary-grow-bmp-bgp-msg-stats-00

## Scope shipped

| Draft section | Status |
|---------------|--------|
| §5.1 Format (Stat Type / Len / Data) | Done |
| §5.2 Per-peer PDU counters TBD1–TBD10 | Done |
| §5.3 Loc-RIB exclusion, O flag 0, after Peer Up | Done (encoder + collector) |
| §5.4 BMPv3 Stats Report encoding | Done (`EncodeStatsReportV3` / reader) |
| §5.4 BMPv4 Stats TLV enclosure | Done (default encode = BMPv4) |
| §7 Discontinuity handling | Done in `pkg/rate` + collector store |
| §7 Configurable type inclusion | Simulator sends all; speakers may omit |
| Appendix A open issues | Out of scope (documented only) |

## BMPv4 and E-bit (draft-ietf-grow-bmp-tlv)

| Feature | Status |
|---------|--------|
| Common Header Version = 4 | Done (default encode; reader accepts 3 and 4) |
| Stats Report mandatory Stats TLV (type 1) | Done |
| E-bit on Stat Types (PEN before Stat Data) | Done |
| E-bit on Information TLVs | Done (parse/encode) |
| Optional trailing Stats Report TLVs | Parsed into `OptionalStatsTLVs` |

## Enterprise process resource gauges

Not part of the PDU-counter draft (that draft lists CPU as a non-goal for
*IANA* types). This repo carries them as **enterprise Stat Types** via the
E-bit, using IANA PEN **9** (Cisco) by default:

| Ent type | Name | Encoding |
|----------|------|----------|
| 1 | Process CPU utilization | 32-bit gauge, hundredths of a percent |
| 2 | Process memory utilization | 32-bit gauge, hundredths of a percent |

Wire: `Stat Type = 0x8000 \| ent`, `Stat Len = 4 + 4`, then PEN, then value.

Prometheus: `bmp_process_cpu_percent`, `bmp_process_memory_percent`.

## Increment rules (speaker side)

This repo’s **simulator** increments counters to exercise the collector. A
production BGP speaker must follow §5.2:

- **Received**: after BGP header validated (marker + length) and Message Type
  identified; framing errors are not counted; unrecognized types count in TBD9 only.
- **Sent**: when the complete PDU is passed to the transport.
- Enhanced Route Refresh begin/end markers count as ROUTE-REFRESH (TBD7/TBD8).

## Non-goals (draft §4)

Still out of scope as *IANA* PDU types: last NOTIFICATION content, hold/keepalive
intervals, NLRI counts, KEEPALIVE relay via Route Monitoring. Process CPU/memory
are implemented only as enterprise (E-bit) gauges as above.
