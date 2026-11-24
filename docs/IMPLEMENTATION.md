# Implementation notes for draft-choudhary-grow-bmp-bgp-msg-stats-00

## Scope shipped

| Draft section | Status |
|---------------|--------|
| §5.1 Format (Stat Type / Len / Data) | Done |
| §5.2 Per-peer PDU counters TBD1–TBD10 | Done |
| §5.3 Loc-RIB exclusion, O flag 0, after Peer Up | Done (encoder + collector) |
| §5.4 BMPv3 Stats Report encoding | Done |
| §5.4 BMPv4 Stats TLV enclosure | Not implemented (BMPv4 optional) |
| §7 Discontinuity handling | Done in `pkg/rate` + collector store |
| §7 Configurable type inclusion | Simulator sends all; speakers may omit |
| Appendix A open issues | Out of scope (documented only) |

## Increment rules (speaker side)

This repo’s **simulator** increments counters to exercise the collector. A
production BGP speaker must follow §5.2:

- **Received**: after BGP header validated (marker + length) and Message Type
  identified; framing errors are not counted; unrecognized types count in TBD9 only.
- **Sent**: when the complete PDU is passed to the transport.
- Enhanced Route Refresh begin/end markers count as ROUTE-REFRESH (TBD7/TBD8).

## Non-goals (draft §4)

Not implemented by design: last NOTIFICATION content, hold/keepalive intervals,
NLRI counts, CPU/platform metrics, KEEPALIVE relay via Route Monitoring.
