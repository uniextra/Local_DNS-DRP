package dns

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func TestMakeQueryValidAndInvalid(t *testing.T) {
	pkt, err := MakeQuery("google.com.", 0x1234)
	if err != nil {
		t.Fatalf("MakeQuery(google.com.) error: %v", err)
	}
	if binary.BigEndian.Uint16(pkt[0:2]) != 0x1234 {
		t.Fatalf("unexpected query ID: %x", pkt[0:2])
	}

	if _, err := MakeQuery("", 1); err == nil {
		t.Fatal("expected error for empty domain")
	}
	if _, err := MakeQuery("a..b", 1); err == nil {
		t.Fatal("expected error for empty label")
	}
}

func appendEncodedName(buf []byte, labels ...string) []byte {
	for _, l := range labels {
		buf = append(buf, byte(len(l)))
		buf = append(buf, l...)
	}
	return append(buf, 0)
}

func buildValidAResponse(query []byte) []byte {
	resp := make([]byte, len(query))
	copy(resp, query)
	binary.BigEndian.PutUint16(resp[2:4], 0x8180)
	binary.BigEndian.PutUint16(resp[6:8], 1) // ANCOUNT = 1

	// Answer: pointer to question name (0xC00C), TYPE=A, CLASS=IN, TTL=60, RDLEN=4, 1.2.3.4
	resp = append(resp, 0xC0, 0x0C, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 1, 2, 3, 4)
	return resp
}

func TestValidateResponse(t *testing.T) {
	query, err := MakeQuery("example.com", 0x4242)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("valid direct A record", func(t *testing.T) {
		if err := ValidateResponse(query, buildValidAResponse(query)); err != nil {
			t.Fatalf("ValidateResponse() error: %v", err)
		}
	})

	t.Run("valid CNAME chain", func(t *testing.T) {
		resp := make([]byte, len(query))
		copy(resp, query)
		binary.BigEndian.PutUint16(resp[2:4], 0x8180)
		binary.BigEndian.PutUint16(resp[6:8], 2)

		cnameTarget := appendEncodedName(nil, "alias", "example", "com")
		resp = append(resp, 0xC0, 0x0C, 0, 5, 0, 1, 0, 0, 0, 60, 0, byte(len(cnameTarget)))
		resp = append(resp, cnameTarget...)

		resp = appendEncodedName(resp, "alias", "example", "com")
		resp = append(resp, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 1, 2, 3, 4)

		if err := ValidateResponse(query, resp); err != nil {
			t.Fatalf("ValidateResponse() CNAME error: %v", err)
		}
	})

	t.Run("unrelated A record rejected", func(t *testing.T) {
		resp := make([]byte, len(query))
		copy(resp, query)
		binary.BigEndian.PutUint16(resp[2:4], 0x8180)
		binary.BigEndian.PutUint16(resp[6:8], 1)
		resp = appendEncodedName(resp, "other", "com")
		resp = append(resp, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 1, 2, 3, 4)

		if err := ValidateResponse(query, resp); err == nil {
			t.Fatal("expected error for unrelated A record")
		}
	})

	t.Run("compression pointer loop rejected", func(t *testing.T) {
		resp := make([]byte, 16)
		binary.BigEndian.PutUint16(resp[0:2], 0x4242)
		binary.BigEndian.PutUint16(resp[2:4], 0x8180)
		binary.BigEndian.PutUint16(resp[4:6], 1)
		binary.BigEndian.PutUint16(resp[6:8], 1)
		resp[12] = 0xC0
		resp[13] = 12
		if err := ValidateResponse(query, resp); err == nil {
			t.Fatal("expected error for compression pointer loop")
		}
	})
}

func TestCheckerCheck(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ListenPacket: %v", err)
	}
	defer pc.Close()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = pc.WriteTo(buildValidAResponse(buf[:n]), addr)
		}
	}()

	ok, detail := (Checker{}).Check(context.Background(), Target{Name: "test", Addr: pc.LocalAddr().String()}, "google.com", time.Second)
	if !ok {
		t.Fatalf("Check() = false (%s), want true", detail)
	}
}
