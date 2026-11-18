// Package dns provides lightweight UDP DNS health checks with response validation.
package dns

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Target represents a named DNS endpoint in host:port form.
type Target struct {
	Name string
	Addr string
}

// MakeQuery builds a recursive A-record DNS query packet for domain with the given ID.
func MakeQuery(domain string, id uint16) ([]byte, error) {
	domain = strings.TrimSuffix(domain, ".")
	if domain == "" || len(domain) > 253 {
		return nil, fmt.Errorf("invalid DNS name length: %d bytes", len(domain))
	}
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[0:2], id)
	binary.BigEndian.PutUint16(packet[2:4], 0x0100)
	binary.BigEndian.PutUint16(packet[4:6], 1)
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 {
			return nil, fmt.Errorf("invalid DNS label %q", label)
		}
		packet = append(packet, byte(len(label)))
		packet = append(packet, label...)
	}
	packet = append(packet, 0, 0, 1, 0, 1)
	return packet, nil
}

// parseDNSName decodes a DNS name, including compression pointers. nextOffset
// is the position immediately after the encoded name in the original stream.
func parseDNSName(packet []byte, offset int) (string, int, error) {
	if offset < 0 || offset >= len(packet) {
		return "", 0, errors.New("DNS name exceeds packet")
	}
	cursor, nextOffset := offset, -1
	labels := make([]string, 0, 4)
	visited := make(map[int]bool)
	expandedLength := 1
	for {
		if cursor >= len(packet) {
			return "", 0, errors.New("DNS name exceeds packet")
		}
		length := int(packet[cursor])
		switch length & 0xC0 {
		case 0x00:
			cursor++
			if length == 0 {
				if nextOffset < 0 {
					nextOffset = cursor
				}
				return strings.ToLower(strings.Join(labels, ".")), nextOffset, nil
			}
			if length > 63 || cursor+length > len(packet) {
				return "", 0, errors.New("invalid DNS label length")
			}
			expandedLength += length + 1
			if expandedLength > 255 {
				return "", 0, errors.New("expanded DNS name exceeds 255 bytes")
			}
			labels = append(labels, string(packet[cursor:cursor+length]))
			cursor += length
		case 0xC0:
			if cursor+1 >= len(packet) {
				return "", 0, errors.New("truncated DNS compression pointer")
			}
			pointer := ((length & 0x3F) << 8) | int(packet[cursor+1])
			if pointer >= len(packet) {
				return "", 0, errors.New("DNS compression pointer out of range")
			}
			if nextOffset < 0 {
				nextOffset = cursor + 2
			}
			if visited[pointer] {
				return "", 0, errors.New("DNS compression pointer loop")
			}
			visited[pointer] = true
			cursor = pointer
		default:
			return "", 0, errors.New("reserved DNS label encoding")
		}
	}
}

// ValidateResponse verifies that response is a valid, non-truncated NOERROR A-record
// answer (directly or via CNAME chain) matching query.
func ValidateResponse(query, response []byte) error {
	if len(query) < 17 || len(response) < 12 {
		return errors.New("DNS packet too short")
	}
	if binary.BigEndian.Uint16(response[0:2]) != binary.BigEndian.Uint16(query[0:2]) {
		return errors.New("DNS response ID mismatch")
	}
	flags := binary.BigEndian.Uint16(response[2:4])
	if flags&0x8000 == 0 {
		return errors.New("packet is not a DNS response")
	}
	if flags&0x7800 != 0 {
		return errors.New("DNS response has unexpected opcode")
	}
	if flags&0x0200 != 0 {
		return errors.New("truncated DNS response")
	}
	if rcode := flags & 0x000f; rcode != 0 {
		return fmt.Errorf("DNS response code: %d", rcode)
	}
	if flags&0x0040 != 0 {
		return errors.New("DNS response has reserved Z flag set")
	}
	qdCount := int(binary.BigEndian.Uint16(response[4:6]))
	anCount := int(binary.BigEndian.Uint16(response[6:8]))
	if qdCount != 1 {
		return fmt.Errorf("DNS response question count = %d, want 1", qdCount)
	}
	queryName, _, err := parseDNSName(query, 12)
	if err != nil {
		return fmt.Errorf("invalid query name: %w", err)
	}
	responseName, offset, err := parseDNSName(response, 12)
	if err != nil {
		return fmt.Errorf("invalid response question: %w", err)
	}
	if responseName != queryName || offset+4 > len(response) {
		return errors.New("DNS response question does not match query")
	}
	qtype := binary.BigEndian.Uint16(response[offset : offset+2])
	qclass := binary.BigEndian.Uint16(response[offset+2 : offset+4])
	if qtype != binary.BigEndian.Uint16(query[len(query)-4:len(query)-2]) || qclass != binary.BigEndian.Uint16(query[len(query)-2:]) {
		return errors.New("DNS response question type/class does not match query")
	}
	if qtype != 1 || qclass != 1 {
		return errors.New("DNS response query is not an A/IN question")
	}
	if anCount == 0 {
		return errors.New("DNS response contains no answer")
	}

	// Follow the CNAME chain and require an A record for the queried name or
	// one of its aliases; unrelated A records do not make the query healthy.
	cursor := offset + 4
	aOwners := make(map[string]bool)
	cnameTargets := make(map[string]string)
	for i := 0; i < anCount; i++ {
		owner, next, err := parseDNSName(response, cursor)
		if err != nil {
			return fmt.Errorf("invalid answer owner name: %w", err)
		}
		if next+10 > len(response) {
			return errors.New("truncated DNS answer header")
		}
		typ := binary.BigEndian.Uint16(response[next : next+2])
		class := binary.BigEndian.Uint16(response[next+2 : next+4])
		rdLength := int(binary.BigEndian.Uint16(response[next+8 : next+10]))
		rdata := next + 10
		if rdata+rdLength > len(response) {
			return errors.New("truncated DNS answer data")
		}
		if class == 1 && typ == 1 {
			if rdLength != net.IPv4len {
				return errors.New("invalid A record length")
			}
			aOwners[owner] = true
		} else if class == 1 && typ == 5 {
			target, consumed, err := parseDNSName(response, rdata)
			if err != nil || consumed != rdata+rdLength {
				return errors.New("invalid CNAME answer data")
			}
			if previous, exists := cnameTargets[owner]; exists && previous != target {
				return errors.New("conflicting CNAME records for answer owner")
			}
			cnameTargets[owner] = target
		}
		cursor = rdata + rdLength
	}

	current := queryName
	visited := make(map[string]bool)
	for {
		if aOwners[current] {
			return nil
		}
		if visited[current] {
			return errors.New("DNS CNAME loop")
		}
		visited[current] = true
		target, ok := cnameTargets[current]
		if !ok {
			return errors.New("DNS response contains no A record for the queried name or its CNAME chain")
		}
		current = target
	}
}

func randomID() (uint16, error) {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b[:]), nil
}

// Checker performs UDP DNS health checks against a Target.
type Checker struct{}

// Check sends a recursive A-record query to target and validates the response.
func (Checker) Check(ctx context.Context, target Target, domain string, timeout time.Duration) (bool, string) {
	id, err := randomID()
	if err != nil {
		return false, fmt.Sprintf("generate query ID: %v", err)
	}
	query, err := MakeQuery(domain, id)
	if err != nil {
		return false, err.Error()
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "udp", target.Addr)
	if err != nil {
		return false, err.Error()
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return false, err.Error()
	}
	if _, err := conn.Write(query); err != nil {
		return false, err.Error()
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return false, err.Error()
	}
	if err := ValidateResponse(query, buf[:n]); err != nil {
		return false, err.Error()
	}
	return true, "DNS query successful"
}
