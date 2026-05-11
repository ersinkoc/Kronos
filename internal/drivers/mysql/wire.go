package mysql

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
)

const (
	mysqlProtocolVersion    = 10
	mysqlMaxPacketSize       = 1 << 24
	mysqlDefaultServerPort   = 3306
)

const (
	mysqlOKPacket          = 0x00
	mysqlERRPacket         = 0xff
	mysqlEOFHello          = 0xfe
	mysqlLocalInFile       = 0xfb
)

const (
	mysqlClientLongPassword     = 1 << 0
	mysqlClientFoundRows         = 1 << 1
	mysqlClientLongFlag          = 1 << 2
	mysqlClientConnectWithDB     = 1 << 3
	mysqlClientNoSchema          = 1 << 4
	mysqlClientCompress          = 1 << 5
	mysqlClientLocalFiles        = 1 << 7
	mysqlClientIgnoreSIGPIPE     = 1 << 8
	mysqlClientTransactions       = 1 << 9
	mysqlClientSecureConnection  = 1 << 11
	mysqlClientMultiStatements    = 1 << 16
	mysqlClientMultiResults       = 1 << 18
	mysqlClientPluginAuth        = 1 << 19
	mysqlClientConnectAttrs       = 1 << 20
	mysqlClientPluginAuthLenEncClientData = 1 << 21
	mysqlClientDeprecateEOF      = 1 << 24
)

const defaultClientCap = mysqlClientLongPassword |
	mysqlClientFoundRows |
	mysqlClientLongFlag |
	mysqlClientLocalFiles |
	mysqlClientTransactions |
	mysqlClientSecureConnection |
	mysqlClientMultiStatements |
	mysqlClientMultiResults |
	mysqlClientPluginAuth |
	mysqlClientConnectAttrs |
	mysqlClientDeprecateEOF

const defaultCharset = 0x21 // utf8mb4_general_ci

type mysqlPacket struct {
	Length uint32
	Number byte
	Type   byte
	Body   []byte
}

type mysqlQueryResult struct {
	Fields  []mysqlField
	Rows    [][]*string
	Command string
}

type mysqlField struct {
	Catalog  string
	Table    string
	OrgTable string
	Name     string
	OrgName  string
	Charset  uint16
	Length   uint32
	Type     uint8
	Flags    uint16
	Scale    uint8
}

func mysqlHandshake(conn net.Conn, username, password, database string) error {
	capabilities := defaultClientCap

	// Read initial handshake
	packet, err := readPacket(conn)
	if err != nil {
		return fmt.Errorf("read handshake: %w", err)
	}
	if packet.Type != mysqlEOFHello {
		return fmt.Errorf("expected handshake packet 0xfe, got 0x%02x", packet.Type)
	}

	// Parse handshake
	serverVersion, pos := readNullString(packet.Body)
	_ = serverVersion
	serverCaps := binary.LittleEndian.Uint32(packet.Body[pos : pos+4])
	pos += 4
	_ = serverCaps
	serverCharset := packet.Body[pos]
	pos++
	serverStatus := binary.LittleEndian.Uint16(packet.Body[pos : pos+2])
	_ = serverStatus
	_ = serverCharset

	// Skip rest of plugin-provided data
	if len(packet.Body) > pos+12 {
		pos += 12
		if pos < len(packet.Body) {
			serverAuthPlugin := string(packet.Body[pos:])
			_ = serverAuthPlugin
		}
	}

	// Build auth response based on server capabilities
	authResp := buildAuthResponse(password, packet.Body)

	// Send handshake response
	var respBody bytes.Buffer
	binary.Write(&respBody, binary.LittleEndian, capabilities)
	respBody.WriteByte(0) // packet number 1
	binary.Write(&respBody, binary.LittleEndian, uint32(23)) // max packet size
	respBody.WriteByte(defaultCharset)
	respBody.Write(bytes.Repeat([]byte{0}, 23)) // reserved
	respBody.WriteString(username)
	respBody.WriteByte(0)

	if len(authResp) > 0 {
		respBody.WriteByte(byte(len(authResp)))
		respBody.Write(authResp)
	} else {
		respBody.WriteByte(0)
	}

	if database != "" {
		respBody.WriteString(database)
	}
	respBody.WriteByte(0)

	if err := writePacket(conn, 1, respBody.Bytes()); err != nil {
		return fmt.Errorf("send auth response: %w", err)
	}

	// Read auth result
	resultPacket, err := readPacket(conn)
	if err != nil {
		return fmt.Errorf("read auth result: %w", err)
	}
	switch resultPacket.Type {
	case mysqlOKPacket:
		return nil
	case mysqlERRPacket:
		return parseMySQLError(resultPacket.Body)
	case 0x01:
		// Auth switch request - server requesting different auth method
		return nil
	default:
		return fmt.Errorf("unexpected auth response packet type 0x%02x", resultPacket.Type)
	}
}

func buildAuthResponse(password string, handshakeBody []byte) []byte {
	if password == "" {
		return nil
	}

	// Extract auth data (plugin-provided data after server capabilities)
	// For mysql_native_password, it's the challenge
	// For caching_sha2_password, it's the challenge
	pos := 4 + 1 + 23 + 1 // skip caps, packet num, max packet size, charset, reserved (23 bytes)
	if len(handshakeBody) <= pos {
		return nil
	}

	authData := handshakeBody[pos:]
	if len(authData) < 20 {
		return nil
	}

	// MySQL 5.x uses mysql_native_password: SHA1(password) XOR SHA1(challenge)
	return mysqlNativePassword(password, authData[:20])
}

func mysqlNativePassword(password string, challenge []byte) []byte {
	// Stage 1: SHA1(password)
	p1 := sha1.Sum([]byte(password))
	// Stage 2: SHA1(p1)
	p2 := sha1.Sum(p1[:])
	// Stage 3: XOR(p1, SHA1(challenge XOR p2))
	stage3 := make([]byte, 20)
	for i := range challenge {
		if i < len(p2) {
			stage3[i] = p1[i] ^ sha1.Sum(append(p2[:], challenge[i]))[0]
		}
	}
	return stage3
}

func mysqlQuery(conn net.Conn, query string) (mysqlQueryResult, error) {
	if err := writeCommandPacket(conn, 0x03, []byte(query)); err != nil {
		return mysqlQueryResult{}, err
	}
	return readQueryResponse(conn)
}

func writeCommandPacket(w io.Writer, command byte, payload []byte) error {
	packet := make([]byte, 4+1+len(payload))
	packetLen := 1 + len(payload)
	packet[0] = byte(packetLen)
	packet[1] = byte(packetLen >> 8)
	packet[2] = byte(packetLen >> 16)
	packet[3] = 0
	packet[4] = command
	copy(packet[5:], payload)
	_, err := w.Write(packet)
	return err
}

func writePacket(w io.Writer, packetNum byte, body []byte) error {
	packet := make([]byte, 4+len(body))
	packetLen := len(body)
	packet[0] = byte(packetLen)
	packet[1] = byte(packetLen >> 8)
	packet[2] = byte(packetLen >> 16)
	packet[3] = packetNum
	copy(packet[4:], body)
	_, err := w.Write(packet)
	return err
}

func readPacket(r io.Reader) (mysqlPacket, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return mysqlPacket{}, err
	}
	length := binary.LittleEndian.Uint32(header[:3])
	if length == 0 {
		return mysqlPacket{Length: 0, Number: header[3]}, nil
	}
	if length > mysqlMaxPacketSize {
		return mysqlPacket{}, fmt.Errorf("mysql packet too large: %d", length)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return mysqlPacket{}, err
	}
	return mysqlPacket{
		Length: binary.LittleEndian.Uint32(header[:3]),
		Number: header[3],
		Type:   body[0],
		Body:   body[1:],
	}, nil
}

func readQueryResponse(conn net.Conn) (mysqlQueryResult, error) {
	var result mysqlQueryResult
	firstPacket, err := readPacket(conn)
	if err != nil {
		return mysqlQueryResult{}, err
	}

	if firstPacket.Type == mysqlOKPacket {
		result.Command = parseOKCommand(firstPacket.Body)
		return result, nil
	}
	if firstPacket.Type == mysqlERRPacket {
		return mysqlQueryResult{}, parseMySQLError(firstPacket.Body)
	}

	// Column count in first packet
	colCountReader := bytes.NewReader(firstPacket.Body)
	colCount, err := binary.ReadUvarint(colCountReader)
	if err != nil || colCount == 0 {
		return mysqlQueryResult{}, fmt.Errorf("invalid column count")
	}
	_ = colCount

	// Read columns
	for {
		colPacket, err := readPacket(conn)
		if err != nil {
			return mysqlQueryResult{}, err
		}
		if colPacket.Type == mysqlEOFHello && len(colPacket.Body) == 5 {
			break // End of columns
		}
		if colPacket.Type == mysqlERRPacket {
			return mysqlQueryResult{}, parseMySQLError(colPacket.Body)
		}
		field, err := parseFieldPacket(colPacket.Body)
		if err == nil {
			result.Fields = append(result.Fields, field)
		}
	}

	// Read rows
	for {
		rowPacket, err := readPacket(conn)
		if err != nil {
			return mysqlQueryResult{}, err
		}
		if rowPacket.Type == mysqlERRPacket {
			return mysqlQueryResult{}, parseMySQLError(rowPacket.Body)
		}
		if rowPacket.Type == mysqlEOFHello {
			break
		}
		row, err := parseRowPacket(rowPacket.Body, result.Fields)
		if err == nil {
			result.Rows = append(result.Rows, row)
		}
	}

	return result, nil
}

func parseFieldPacket(body []byte) (mysqlField, error) {
	var field mysqlField
	pos := 0

	// Catalog (length-encoded string)
	if len(body) > pos {
		clen := int(body[pos])
		pos++
		if clen > 0 && pos+clen <= len(body) {
			field.Catalog = string(body[pos : pos+clen])
			pos += clen
		}
	}

	// Schema, table, org table
	for _, name := range []*string{&field.Table, &field.OrgTable, &field.Name, &field.OrgName} {
		if len(body) > pos {
			slen := int(body[pos])
			pos++
			if slen > 0 && pos+slen <= len(body) {
				*name = string(body[pos : pos+slen])
				pos += slen
			}
		}
	}

	// Skip length (fixed 4 bytes)
	pos += 4

	// Type (1 byte)
	if len(body) > pos {
		field.Type = body[pos]
		pos++
	}

	// Flags (2 bytes)
	if len(body) > pos+1 {
		field.Flags = binary.LittleEndian.Uint16(body[pos : pos+2])
		pos += 2
	}

	// Scale (1 byte)
	if len(body) > pos {
		field.Scale = body[pos]
		pos++
	}

	// Skip default (rest of packet)
	return field, nil
}

func parseRowPacket(body []byte, fields []mysqlField) ([]*string, error) {
	row := make([]*string, len(fields))
	pos := 0
	for i := range fields {
		if pos >= len(body) {
			break
		}
		slen := int(body[pos])
		pos++
		if slen == 0xfb {
			row[i] = nil // NULL
			continue
		}
		if slen == 0xfc {
			if pos+2 > len(body) {
				break
			}
			slen = int(binary.LittleEndian.Uint16(body[pos : pos+2]))
			pos += 2
		} else if slen == 0xfd {
			if pos+3 > len(body) {
				break
			}
			slen = int(body[pos]) | (int(body[pos+1]) << 8) | (int(body[pos+2]) << 16)
			pos += 3
		}
		if pos+slen > len(body) {
			slen = len(body) - pos
		}
		if slen > 0 {
			s := string(body[pos : pos+slen])
			row[i] = &s
			pos += slen
		}
	}
	return row, nil
}

func parseOKCommand(body []byte) string {
	if len(body) >= 3 {
		return "OK"
	}
	return ""
}

func parseMySQLError(body []byte) error {
	if len(body) < 9 {
		return errors.New("mysql error")
	}
	return fmt.Errorf("mysql error %d: %s", binary.LittleEndian.Uint16(body[1:3]), string(body[9:]))
}

func readNullString(b []byte) (string, int) {
	for i := 0; i < len(b); i++ {
		if b[i] == 0 {
			return string(b[:i]), i + 1
		}
	}
	return string(b), len(b)
}
