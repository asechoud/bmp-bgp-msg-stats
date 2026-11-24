package stats

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	reg := DefaultRegistry()
	c := NewCounters()
	c.UpdateReceived = 1000
	c.KeepaliveReceived = 50
	c.TotalReceived = 1060
	for _, s := range DefaultSpecs() {
		c.Present[s.Type] = true
	}
	// Only set a subset present for encode filter test — mark all present.
	tlvs, err := c.Encode(reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(tlvs) != 10 {
		t.Fatalf("expected 10 TLVs, got %d", len(tlvs))
	}
	got, err := reg.ParseTLVs(tlvs)
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdateReceived != 1000 || got.KeepaliveReceived != 50 || got.TotalReceived != 1060 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestStatLenEnforced(t *testing.T) {
	reg := DefaultRegistry()
	// 64-bit type with 4-byte data must fail.
	bad := TLV{Type: TypeUpdateReceived, Data: []byte{0, 0, 0, 1}}
	_, err := reg.ParseTLVs([]TLV{bad})
	if err == nil {
		t.Fatal("expected Stat Len error for TBD1")
	}
}

func TestDuplicateTypeRejected(t *testing.T) {
	reg := DefaultRegistry()
	raw, _ := EncodeTLV(TypeKeepaliveReceived, 1, Width32)
	tlv := TLV{Type: TypeKeepaliveReceived, Data: raw[4:]}
	_, err := reg.ParseTLVs([]TLV{tlv, tlv})
	if err == nil {
		t.Fatal("expected duplicate error")
	}
}

func TestOmittedTypeNotZero(t *testing.T) {
	reg := DefaultRegistry()
	raw, _ := EncodeTLV(TypeUpdateReceived, 42, Width64)
	tlv := TLV{Type: TypeUpdateReceived, Data: raw[4:]}
	got, err := reg.ParseTLVs([]TLV{tlv})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Present[TypeUpdateReceived] {
		t.Fatal("UPDATE received should be present")
	}
	if got.Present[TypeKeepaliveReceived] {
		t.Fatal("omitted KEEPALIVE must not be treated as present/zero")
	}
}

func Test64BitCounterEncoding(t *testing.T) {
	const v = uint64(0x0102030405060708)
	raw, err := EncodeTLV(TypeTotalReceived, v, Width64)
	if err != nil {
		t.Fatal(err)
	}
	if binary.BigEndian.Uint16(raw[2:4]) != 8 {
		t.Fatalf("Stat Len want 8 got %d", binary.BigEndian.Uint16(raw[2:4]))
	}
	if binary.BigEndian.Uint64(raw[4:12]) != v {
		t.Fatal("value mismatch")
	}
	if bytes.Equal(raw[4:8], []byte{0, 0, 0, 0}) {
		t.Fatal("high 32 bits unexpectedly zero for this fixture")
	}
}
