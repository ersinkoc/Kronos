package chunk

import (
	"bytes"
	"testing"
)

func TestHashBytesKnownAnswers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "empty",
			data: nil,
			want: "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262",
		},
		{
			name: "abc",
			data: []byte("abc"),
			want: "6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := HashBytes(tc.data).String(); got != tc.want {
				t.Fatalf("HashBytes() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestHashReaderMatchesHashBytes(t *testing.T) {
	t.Parallel()

	data := deterministicData(128 * 1024)
	got, err := HashReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("HashReader() error = %v", err)
	}
	want := HashBytes(data)
	if got != want {
		t.Fatalf("HashReader() = %s, want %s", got, want)
	}
}

func TestParseHashRoundTrip(t *testing.T) {
	t.Parallel()

	want := HashBytes([]byte("kronos"))
	got, err := ParseHash(want.String())
	if err != nil {
		t.Fatalf("ParseHash() error = %v", err)
	}
	if got != want {
		t.Fatalf("ParseHash() = %s, want %s", got, want)
	}
}

func TestParseHashRejectsWrongSize(t *testing.T) {
	t.Parallel()

	if _, err := ParseHash("abcd"); err == nil {
		t.Fatal("ParseHash(short) error = nil, want error")
	}
}
