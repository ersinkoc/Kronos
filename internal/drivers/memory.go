package drivers

import "io"

// MemoryRecordStream stores records in memory for driver tests.
type MemoryRecordStream struct {
	records []Record
	index   int
}

// WriteRecord appends one logical record.
func (s *MemoryRecordStream) WriteRecord(obj ObjectRef, payload []byte) error {
	s.records = append(s.records, Record{Object: obj, Payload: append([]byte(nil), payload...)})
	return nil
}

// FinishObject marks one logical object as complete.
func (s *MemoryRecordStream) FinishObject(obj ObjectRef, rows int64) error {
	s.records = append(s.records, Record{Object: obj, Rows: rows, Done: true})
	return nil
}

// NextRecord returns the next record or io.EOF.
func (s *MemoryRecordStream) NextRecord() (Record, error) {
	if s.index >= len(s.records) {
		return Record{}, io.EOF
	}
	record := s.records[s.index]
	s.index++
	return record, nil
}

// Records returns a copy of all records.
func (s *MemoryRecordStream) Records() []Record {
	out := make([]Record, len(s.records))
	copy(out, s.records)
	return out
}
