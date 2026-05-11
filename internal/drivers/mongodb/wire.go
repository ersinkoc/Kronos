package mongodb

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"time"
)

const (
	mongoWireVersion       = 17
	mongoOpMsg             = 2013
	mongoOpQuery           = 2004
	mongoOpReply           = 1
	mongoOpGetMore         = 2005
	mongoOpKillCursors     = 2007

	mongoReplyFlagCursorNotFound = 1 << 0
	mongoReplyFlagQueryFailure   = 1 << 1
	mongoReplyFlagShardConfigStale = 1 << 2
	mongoReplyFlagAwaitCapable   = 1 << 3

	mongoMsgFlagNone           = 0
	mongoMsgFlagCheckSumPresent = 1 << 0
	mongoMsgFlagMoreToCome     = 1 << 1
	mongoMsgFlagExhaustAllowed = 1 << 16
)

type mongoWireSession struct {
	conn       net.Conn
	reader     *bytes.Reader
	requestID  uint32
	database   string
	username   string
	password   string
	mechanism  string
	clusterTime *bsonValue
	nonce      string
	clientNonce  string
	saslPayload  []byte
	conversationID int32
	done         bool
}

type mongoQueryResult struct {
	Rows      []map[string]bsonValue
	NumFields int
	CursorID  int64
}

type bsonValue struct {
	Type byte
	Data interface{}
}

type mongoBinlogEvent struct {
	TS       time.Time
	Version  int
	Op       string
	Namespace string
	Document bsonValue
}

func mongoWireHandshake(ctx context.Context, conn net.Conn, username, password, database, mechanism string) (*mongoWireSession, error) {
	session := &mongoWireSession{
		conn:      conn,
		database:  database,
		username:  username,
		password:  password,
		mechanism: mechanism,
	}

	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, err
	}

	if err := mongoSCRAMAuth(ctx, session, username, password, database, mechanism); err != nil {
		return nil, err
	}

	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}

	return session, nil
}

func mongoSimpleQuery(ctx context.Context, conn net.Conn, username, password, database, mechanism string, query string) (mongoQueryResult, error) {
	session, err := mongoWireHandshake(ctx, conn, username, password, database, mechanism)
	if err != nil {
		return mongoQueryResult{}, err
	}
	defer session.Close()

	if err := session.ping(ctx); err != nil {
		return mongoQueryResult{}, err
	}

	return session.query(ctx, query)
}

func (s *mongoWireSession) ping(ctx context.Context) error {
	_, err := s.query(ctx, "ping")
	return err
}

func (s *mongoWireSession) query(ctx context.Context, query string) (mongoQueryResult, error) {
	ns := s.database + ".$cmd"
	body := bsonDocument{"ping": 1}

	req := mongoMessage{
		Header: mongoMessageHeader{
			MessageLength: 0,
			RequestID:     s.nextRequestID(),
			ResponseTo:    0,
			OpCode:        mongoOpQuery,
		},
		OpQuery: &opQuery{
			FullCollectionName: ns,
			QueryFlags:         0,
			NumberToSkip:       0,
			NumberToReturn:     1,
			Query:              body,
		},
	}

	var msgBytes bytes.Buffer
	if err := writeMongoMessage(&msgBytes, req); err != nil {
		return mongoQueryResult{}, err
	}

	if _, err := s.conn.Write(msgBytes.Bytes()); err != nil {
		return mongoQueryResult{}, err
	}

	reply, err := s.readReply(ctx)
	if err != nil {
		return mongoQueryResult{}, err
	}

	if len(reply.OpReply.Documents) == 0 {
		return mongoQueryResult{}, errors.New("query returned no documents")
	}

	doc := reply.OpReply.Documents[0]
	if errDoc, ok := doc["$err"]; ok {
		return mongoQueryResult{}, fmt.Errorf("query error: %v", errDoc)
	}

	return mongoQueryResult{Rows: []map[string]bsonValue{doc}}, nil
}

func (s *mongoWireSession) queryMany(ctx context.Context, query string, batchSize int) (mongoQueryResult, error) {
	ns := s.database + ".$cmd"

	req := mongoMessage{
		Header: mongoMessageHeader{
			MessageLength: 0,
			RequestID:     s.nextRequestID(),
			ResponseTo:    0,
			OpCode:        mongoOpQuery,
		},
		OpQuery: &opQuery{
			FullCollectionName: ns,
			QueryFlags:        0,
			NumberToSkip:       0,
			NumberToReturn:     int32(-1 * batchSize),
			Query:              bsonDocument{"ping": 1},
		},
	}

	var msgBytes bytes.Buffer
	if err := writeMongoMessage(&msgBytes, req); err != nil {
		return mongoQueryResult{}, err
	}

	var allRows []map[string]bsonValue
	cursorID := int64(0)

	for {
		if _, err := s.conn.Write(msgBytes.Bytes()); err != nil {
			return mongoQueryResult{}, err
		}

		reply, err := s.readReply(ctx)
		if err != nil {
			return mongoQueryResult{}, err
		}

		if len(reply.OpReply.Documents) > 0 {
			allRows = append(allRows, reply.OpReply.Documents...)
		}

		cursorID = reply.OpReply.CursorID
		if cursorID == 0 {
			break
		}

		if len(reply.OpReply.Documents) < batchSize {
			break
		}
	}

	if cursorID != 0 {
		s.killCursors(ctx, []int64{cursorID})
	}

	return mongoQueryResult{Rows: allRows}, nil
}

func (s *mongoWireSession) killCursors(ctx context.Context, cursorIDs []int64) error {
	req := mongoMessage{
		Header: mongoMessageHeader{
			MessageLength: 0,
			RequestID:     s.nextRequestID(),
			ResponseTo:    0,
			OpCode:        mongoOpKillCursors,
		},
		OpKillCursors: &opKillCursors{
			Zero:          0,
			NumberOfCursors: int32(len(cursorIDs)),
			CursorIDs:     cursorIDs,
		},
	}

	var msgBytes bytes.Buffer
	if err := writeMongoMessage(&msgBytes, req); err != nil {
		return err
	}

	_, err := s.conn.Write(msgBytes.Bytes())
	return err
}

func (s *mongoWireSession) nextRequestID() uint32 {
	s.requestID++
	return s.requestID
}

type mongoMessageHeader struct {
	MessageLength int32
	RequestID     uint32
	ResponseTo    uint32
	OpCode        int32
}

type mongoMessage struct {
	Header     mongoMessageHeader
	OpQuery    *opQuery
	OpReply    *opReply
	OpMsg      *opMsg
	OpGetMore  *opGetMore
	OpKillCursors *opKillCursors
}

type opQuery struct {
	FullCollectionName string
	QueryFlags         int32
	NumberToSkip       int32
	NumberToReturn     int32
	Query              interface{}
}

type opReply struct {
	ResponseFlags   int32
	CursorID        int64
	StartingFrom    int32
	NumberReturned  int32
	Documents       []map[string]bsonValue
}

type opMsg struct {
	FlagBits    int32
	Sections    []opMsgSection
	Checksum    uint32
}

type opMsgSection struct {
	PayloadType byte
	Payload     interface{}
}

type opGetMore struct {
	Zero              int32
	FullCollectionName string
	CursorID          int64
	NumberToReturn    int32
}

type opKillCursors struct {
	Zero             int32
	NumberOfCursors int32
	CursorIDs       []int64
}

func writeMongoMessage(w *bytes.Buffer, msg mongoMessage) error {
	startPos := w.Len()

	binary.Write(w, binary.LittleEndian, msg.Header.RequestID)
	binary.Write(w, binary.LittleEndian, msg.Header.ResponseTo)
	binary.Write(w, binary.LittleEndian, msg.Header.OpCode)

	switch {
	case msg.OpQuery != nil:
		q := msg.OpQuery
		writeCString(w, q.FullCollectionName)
		binary.Write(w, binary.LittleEndian, q.QueryFlags)
		binary.Write(w, binary.LittleEndian, q.NumberToSkip)
		binary.Write(w, binary.LittleEndian, q.NumberToReturn)
		if err := writeBSON(w, q.Query); err != nil {
			return err
		}

	case msg.OpMsg != nil:
		m := msg.OpMsg
		binary.Write(w, binary.LittleEndian, m.FlagBits)
		for _, section := range m.Sections {
			w.WriteByte(section.PayloadType)
			if err := writeBSON(w, section.Payload); err != nil {
				return err
			}
		}
		if m.Checksum != 0 {
			binary.Write(w, binary.LittleEndian, m.Checksum)
		}

	case msg.OpKillCursors != nil:
		k := msg.OpKillCursors
		binary.Write(w, binary.LittleEndian, k.Zero)
		binary.Write(w, binary.LittleEndian, k.NumberOfCursors)
		for _, id := range k.CursorIDs {
			binary.Write(w, binary.LittleEndian, id)
		}
	}

	msgLen := int32(w.Len() - startPos)
	bs := w.Bytes()
	binary.LittleEndian.PutUint32(bs[startPos:], uint32(msgLen))

	return nil
}

func (s *mongoWireSession) readReply(ctx context.Context) (*mongoMessage, error) {
	headerBytes := make([]byte, 16)
	if err := s.readFull(ctx, headerBytes); err != nil {
		return nil, err
	}

	header := mongoMessageHeader{
		MessageLength: int32(binary.LittleEndian.Uint32(headerBytes[0:4])),
		RequestID:    binary.LittleEndian.Uint32(headerBytes[4:8]),
		ResponseTo:   binary.LittleEndian.Uint32(headerBytes[8:12]),
		OpCode:       int32(binary.LittleEndian.Uint32(headerBytes[12:16])),
	}

	if header.MessageLength < 16 || header.MessageLength > 16*1024*1024 {
		return nil, fmt.Errorf("invalid message length: %d", header.MessageLength)
	}

	msgBytes := make([]byte, header.MessageLength-16)
	if err := s.readFull(ctx, msgBytes); err != nil {
		return nil, err
	}

	msg := &mongoMessage{Header: header}

	switch header.OpCode {
	case mongoOpReply:
		msg.OpReply = s.parseOpReply(msgBytes)
	case mongoOpMsg:
		msg.OpMsg = s.parseOpMsg(msgBytes)
	default:
		return nil, fmt.Errorf("unexpected op code: %d", header.OpCode)
	}

	return msg, nil
}

func (s *mongoWireSession) parseOpReply(data []byte) *opReply {
	reply := &opReply{
		Documents: []map[string]bsonValue{},
	}

	if len(data) < 20 {
		return reply
	}

	pos := 0
	reply.ResponseFlags = int32(binary.LittleEndian.Uint32(data[pos:]))
	pos += 4
	reply.CursorID = int64(binary.LittleEndian.Uint64(data[pos:]))
	pos += 8
	reply.StartingFrom = int32(binary.LittleEndian.Uint32(data[pos:]))
	pos += 4
	reply.NumberReturned = int32(binary.LittleEndian.Uint32(data[pos:]))
	pos += 4

	for i := int32(0); i < reply.NumberReturned && pos < len(data); i++ {
		doc, consumed := parseBSONDocument(data[pos:])
		if consumed == 0 {
			break
		}
		reply.Documents = append(reply.Documents, doc)
		pos += consumed
	}

	return reply
}

func (s *mongoWireSession) parseOpMsg(data []byte) *opMsg {
	if len(data) < 4 {
		return nil
	}

	msg := &opMsg{
		Sections: []opMsgSection{},
	}
	pos := 0

	msg.FlagBits = int32(binary.LittleEndian.Uint32(data[pos:]))
	pos += 4

	for pos < len(data) {
		sectionType := data[pos]
		pos++
		doc, consumed := parseBSONDocument(data[pos:])
		if consumed == 0 {
			break
		}
		msg.Sections = append(msg.Sections, opMsgSection{
			PayloadType: sectionType,
			Payload:     doc,
		})
		pos += consumed
	}

	return msg
}

func (s *mongoWireSession) readFull(ctx context.Context, buf []byte) error {
	for len(buf) > 0 {
		n, err := s.conn.Read(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}

func (s *mongoWireSession) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

type opMsgSectionDocument struct {
	PayloadType byte
	Payload     bsonDocument
}

func writeCString(w *bytes.Buffer, s string) {
	w.WriteString(s)
	w.WriteByte(0)
}

func readCString(data []byte, pos *int) string {
	start := *pos
	for *pos < len(data) && data[*pos] != 0 {
		(*pos)++
	}
	result := string(data[start:*pos])
	if *pos < len(data) {
		(*pos)++
	}
	return result
}

type bsonDocument map[string]interface{}

func writeBSON(w *bytes.Buffer, val interface{}) error {
	switch v := val.(type) {
	case bsonDocument:
		return writeBSONDocument(w, v)
	case map[string]interface{}:
		return writeBSONDocument(w, bsonDocument(v))
	default:
		return writeBSONValue(w, v)
	}
}

func writeBSONDocument(w *bytes.Buffer, doc bsonDocument) error {
	startLen := w.Len()
	w.Write([]byte{0, 0, 0, 0})

	for key, val := range doc {
		writeCString(w, key)
		if err := writeBSONValue(w, val); err != nil {
			return err
		}
	}
	w.WriteByte(0)

	docLen := int32(w.Len() - startLen)
	bs := w.Bytes()
	binary.LittleEndian.PutUint32(bs[startLen:], uint32(docLen))

	return nil
}

func writeBSONValue(w *bytes.Buffer, val interface{}) error {
	if val == nil {
		w.WriteByte(0)
		return nil
	}

	switch v := val.(type) {
	case float64:
		w.WriteByte(1)
		binary.Write(w, binary.LittleEndian, v)

	case string:
		w.WriteByte(2)
		strBytes := []byte(v)
		binary.Write(w, binary.LittleEndian, int32(len(strBytes)+1))
		w.Write(strBytes)
		w.WriteByte(0)

	case bsonDocument:
		w.WriteByte(3)
		if err := writeBSONDocument(w, v); err != nil {
			return err
		}

	case []interface{}:
		w.WriteByte(4)
		arrDoc := bsonDocument{}
		for i, item := range v {
			arrDoc[fmt.Sprintf("%d", i)] = item
		}
		if err := writeBSONDocument(w, arrDoc); err != nil {
			return err
		}

	case bsonValue:
		w.WriteByte(v.Type)
		return writeBSONValue(w, v.Data)

	case map[string]interface{}:
		w.WriteByte(3)
		return writeBSONDocument(w, bsonDocument(v))

	case int32:
		w.WriteByte(16)
		binary.Write(w, binary.LittleEndian, v)

	case int64:
		w.WriteByte(18)

	case bool:
		w.WriteByte(8)
		if v {
			w.WriteByte(1)
		} else {
			w.WriteByte(0)
		}

	case int:
		w.WriteByte(16)
		binary.Write(w, binary.LittleEndian, int32(v))

	case time.Time:
		w.WriteByte(9)
		binary.Write(w, binary.LittleEndian, int64(v.Unix()*1000))

	case []byte:
		w.WriteByte(5)
		binary.Write(w, binary.LittleEndian, int32(len(v)))
		w.Write(v)

	default:
		w.WriteByte(10)
	}

	return nil
}

func parseBSONDocument(data []byte) (map[string]bsonValue, int) {
	if len(data) < 4 {
		return nil, 0
	}

	docLen := int(binary.LittleEndian.Uint32(data[0:4]))
	if docLen > len(data) || docLen < 5 {
		return nil, 0
	}

	doc := make(map[string]bsonValue)
	pos := 4

	for pos < docLen-1 {
		tp := data[pos]
		pos++
		if tp == 0 {
			break
		}

		key := readCString(data, &pos)
		val, consumed := parseBSONValue(data, pos, tp)
		if consumed == 0 {
			break
		}
		doc[key] = bsonValue{Type: tp, Data: val}
		pos += consumed
	}

	return doc, docLen
}

func parseBSONValue(data []byte, pos int, tp byte) (interface{}, int) {
	switch tp {
	case 1: // double
		if pos+8 > len(data) {
			return nil, 0
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(data[pos:])), 8

	case 2: // string
		if pos+4 > len(data) {
			return nil, 0
		}
		strLen := int(binary.LittleEndian.Uint32(data[pos:]))
		pos += 4
		if pos+strLen > len(data) {
			return nil, 0
		}
		return string(data[pos : pos+strLen-1]), strLen + 4

	case 3: // document
		if pos+4 > len(data) {
			return nil, 0
		}
		subDoc, consumed := parseBSONDocument(data[pos:])
		if consumed == 0 {
			return nil, 0
		}
		return subDoc, consumed

	case 4: // array
		if pos+4 > len(data) {
			return nil, 0
		}
		subDoc, consumed := parseBSONDocument(data[pos:])
		if consumed == 0 {
			return nil, 0
		}
		return subDoc, consumed

	case 5: // binary
		if pos+4 > len(data) {
			return nil, 0
		}
		subLen := int(binary.LittleEndian.Uint32(data[pos:]))
		pos += 4
		if pos+subLen+1 > len(data) {
			return nil, 0
		}
		return data[pos : pos+subLen], subLen + 5

	case 7: // object id (12 bytes)
		if pos+12 > len(data) {
			return nil, 0
		}
		return data[pos : pos+12], 12

	case 8: // bool
		if pos+1 > len(data) {
			return nil, 0
		}
		return data[pos] != 0, 1

	case 9: // timestamp (int64)
		if pos+8 > len(data) {
			return nil, 0
		}
		return int64(binary.LittleEndian.Uint64(data[pos:])), 8

	case 10: // null
		return nil, 1

	case 16: // int32
		if pos+4 > len(data) {
			return nil, 0
		}
		return int32(binary.LittleEndian.Uint32(data[pos:])), 4

	case 17: // timestamp
		if pos+8 > len(data) {
			return nil, 0
		}
		return int64(binary.LittleEndian.Uint64(data[pos:])), 8

	case 18: // int64
		if pos+8 > len(data) {
			return nil, 0
		}
		return int64(binary.LittleEndian.Uint64(data[pos:])), 8

	default:
		return nil, 0
	}
}

func mongoSCRAMAuth(ctx context.Context, s *mongoWireSession, username, password, database, mechanism string) error {
	if mechanism == "" {
		mechanism = "SCRAM-SHA-256"
	}

	s.clientNonce = generateNonce()

	sendDoc := bsonDocument{
		"saslStart":              1,
		"mechanism":              mechanism,
		"payload":                []byte{},
		"options": bsonDocument{
			"skipAuthenticationCheck": false,
		},
	}

	if err := s.doSaslStart(ctx, sendDoc); err != nil {
		return err
	}

	serverFirst := s.saslPayload
	if err := s.processServerFirst(string(serverFirst), password); err != nil {
		return err
	}

	clientProof := s.buildClientProof(username, password)

	for attempts := 0; attempts < 10; attempts++ {
		sendDoc = bsonDocument{
			"saslContinue":     1,
			"conversationId":  s.conversationID,
			"payload":          clientProof,
		}

		if err := s.doSaslContinue(ctx, sendDoc); err != nil {
			return err
		}

		if s.done {
			break
		}

		if err := s.processServerSecond(string(s.saslPayload), password); err != nil {
			return err
		}
	}

	return nil
}

func (s *mongoWireSession) processServerFirst(serverFirst string, password string) error {
	return nil
}

func (s *mongoWireSession) buildClientProof(username, password string) []byte {
	return nil
}

func (s *mongoWireSession) processServerSecond(serverSecond string, password string) error {
	return nil
}

func (s *mongoWireSession) doSaslStart(ctx context.Context, doc bsonDocument) error {
	req := mongoMessage{
		Header: mongoMessageHeader{
			MessageLength: 0,
			RequestID:     s.nextRequestID(),
			ResponseTo:    0,
			OpCode:        mongoOpQuery,
		},
		OpQuery: &opQuery{
			FullCollectionName: s.database + ".$cmd",
			QueryFlags:        0,
			NumberToSkip:       0,
			NumberToReturn:     1,
			Query:              doc,
		},
	}

	var msgBytes bytes.Buffer
	if err := writeMongoMessage(&msgBytes, req); err != nil {
		return err
	}

	if _, err := s.conn.Write(msgBytes.Bytes()); err != nil {
		return err
	}

	reply, err := s.readReply(ctx)
	if err != nil {
		return err
	}

	if len(reply.OpReply.Documents) == 0 {
		return errors.New("saslStart returned no documents")
	}

	docResult := reply.OpReply.Documents[0]

	if okVal, ok := docResult["ok"]; ok {
		if fl, ok := okVal.Data.(float64); ok && fl == 0 {
			if errmsg, ok := docResult["errmsg"]; ok {
				return fmt.Errorf("saslStart failed: %v", errmsg)
			}
			return errors.New("saslStart failed")
		}
	}

	if convID, ok := docResult["conversationId"]; ok {
		if id, ok := convID.Data.(int32); ok {
			s.conversationID = id
		}
	}

	if payload, ok := docResult["payload"]; ok {
		s.saslPayload = extractBinary(payload)
	}

	return nil
}

func (s *mongoWireSession) doSaslContinue(ctx context.Context, doc bsonDocument) error {
	req := mongoMessage{
		Header: mongoMessageHeader{
			MessageLength: 0,
			RequestID:     s.nextRequestID(),
			ResponseTo:    0,
			OpCode:        mongoOpQuery,
		},
		OpQuery: &opQuery{
			FullCollectionName: s.database + ".$cmd",
			QueryFlags:        0,
			NumberToSkip:       0,
			NumberToReturn:     1,
			Query:              doc,
		},
	}

	var msgBytes bytes.Buffer
	if err := writeMongoMessage(&msgBytes, req); err != nil {
		return err
	}

	if _, err := s.conn.Write(msgBytes.Bytes()); err != nil {
		return err
	}

	reply, err := s.readReply(ctx)
	if err != nil {
		return err
	}

	if len(reply.OpReply.Documents) == 0 {
		return errors.New("saslContinue returned no documents")
	}

	docResult := reply.OpReply.Documents[0]

	if okVal, ok := docResult["ok"]; ok {
		if fl, ok := okVal.Data.(float64); ok && fl == 0 {
			if errmsg, ok := docResult["errmsg"]; ok {
				return fmt.Errorf("saslContinue failed: %v", errmsg)
			}
			return errors.New("saslContinue failed")
		}
	}

	if convID, ok := docResult["conversationId"]; ok {
		if id, ok := convID.Data.(int32); ok {
			s.conversationID = id
		}
	}

	if done, ok := docResult["done"]; ok {
		if b, ok := done.Data.(bool); ok {
			s.done = b
		}
	}

	if payload, ok := docResult["payload"]; ok {
		s.saslPayload = extractBinary(payload)
	}

	return nil
}

func generateNonce() string {
	b := make([]byte, 24)
	for i := range b {
		b[i] = byte(i % 256)
	}
	return fmt.Sprintf("%x", b)
}

func extractBinary(val bsonValue) []byte {
	switch v := val.Data.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	default:
		return nil
	}
}
