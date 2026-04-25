package s3

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

var testCredentials = Credentials{
	AccessKey: "AKIAIOSFODNN7EXAMPLE",
	SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
}

var testTime = time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)

func TestSignGETObjectAWSExample(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Range", "bytes=0-9")

	signature, err := (Signer{Region: "us-east-1"}).SignRequest(req, testCredentials, testTime, emptyPayloadSHA256)
	if err != nil {
		t.Fatalf("SignRequest() error = %v", err)
	}

	wantSignature := "f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if signature.Signature != wantSignature {
		t.Fatalf("Signature = %s, want %s\nCanonicalRequest:\n%s\nStringToSign:\n%s", signature.Signature, wantSignature, signature.CanonicalRequest, signature.StringToSign)
	}
	if signature.SignedHeaders != "host;range;x-amz-content-sha256;x-amz-date" {
		t.Fatalf("SignedHeaders = %q", signature.SignedHeaders)
	}
	if req.Header.Get("Authorization") != signature.Authorization {
		t.Fatalf("Authorization header was not applied")
	}
}

func TestSignPUTObjectAWSExample(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodPut, "https://examplebucket.s3.amazonaws.com/test$file.text", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Date", "Fri, 24 May 2013 00:00:00 GMT")
	req.Header.Set("x-amz-storage-class", "REDUCED_REDUNDANCY")

	payloadHash := "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072"
	signature, err := (Signer{Region: "us-east-1"}).SignRequest(req, testCredentials, testTime, payloadHash)
	if err != nil {
		t.Fatalf("SignRequest() error = %v", err)
	}

	wantSignature := "98ad721746da40c64f1a55b78f14c238d841ea1380cd77a1b5971af0ece108bd"
	if signature.Signature != wantSignature {
		t.Fatalf("Signature = %s, want %s\nCanonicalRequest:\n%s\nStringToSign:\n%s", signature.Signature, wantSignature, signature.CanonicalRequest, signature.StringToSign)
	}
	if !strings.Contains(signature.CanonicalRequest, "/test%24file.text") {
		t.Fatalf("CanonicalRequest path not encoded as expected:\n%s", signature.CanonicalRequest)
	}
}

func TestSignListObjectsAWSExample(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/?max-keys=2&prefix=J", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	signature, err := (Signer{Region: "us-east-1"}).SignRequest(req, testCredentials, testTime, emptyPayloadSHA256)
	if err != nil {
		t.Fatalf("SignRequest() error = %v", err)
	}

	wantSignature := "34b48302e7b5fa45bde8084f4b7868a86f0a534bc59db6670ed5711ef69dc6f7"
	if signature.Signature != wantSignature {
		t.Fatalf("Signature = %s, want %s\nCanonicalRequest:\n%s\nStringToSign:\n%s", signature.Signature, wantSignature, signature.CanonicalRequest, signature.StringToSign)
	}
}

func TestCanonicalQuerySortsAfterEncoding(t *testing.T) {
	t.Parallel()

	values := make(url.Values)
	query := canonicalQuery(values)
	if query != "" {
		t.Fatalf("empty canonical query = %q", query)
	}

	got := canonicalQuery(url.Values{
		"prefix":      {"some Prefix"},
		"max-keys":    {"20"},
		"marker":      {"some/Marker"},
		"subresource": {""},
	})
	want := "marker=some%2FMarker&max-keys=20&prefix=some%20Prefix&subresource="
	if got != want {
		t.Fatalf("canonicalQuery() = %q, want %q", got, want)
	}
}

func TestURIEncodePreservesObjectSlashes(t *testing.T) {
	t.Parallel()

	if got := uriEncode("/photos/Jan/sample 1.jpg", false); got != "/photos/Jan/sample%201.jpg" {
		t.Fatalf("uriEncode(path) = %q", got)
	}
	if got := uriEncode("a/b c", true); got != "a%2Fb%20c" {
		t.Fatalf("uriEncode(query) = %q", got)
	}
}
