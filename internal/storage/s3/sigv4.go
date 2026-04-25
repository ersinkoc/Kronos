package s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	algorithm          = "AWS4-HMAC-SHA256"
	awsRequest         = "aws4_request"
	emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// Credentials are static AWS-compatible credentials.
type Credentials struct {
	AccessKey    string
	SecretKey    string
	SessionToken string
	ExpiresAt    time.Time
}

// Signer calculates AWS Signature Version 4 request signatures.
type Signer struct {
	Region  string
	Service string
}

// Signature contains the debug artefacts produced while signing.
type Signature struct {
	Authorization    string
	CanonicalRequest string
	StringToSign     string
	SignedHeaders    string
	Signature        string
	PayloadHash      string
}

// SignRequest adds SigV4 headers to req and returns the signature artefacts.
func (s Signer) SignRequest(req *http.Request, creds Credentials, now time.Time, payloadHash string) (Signature, error) {
	if req == nil {
		return Signature{}, fmt.Errorf("request is required")
	}
	if creds.AccessKey == "" || creds.SecretKey == "" {
		return Signature{}, fmt.Errorf("access key and secret key are required")
	}
	if s.Region == "" {
		return Signature{}, fmt.Errorf("region is required")
	}
	service := s.Service
	if service == "" {
		service = "s3"
	}
	if payloadHash == "" {
		payloadHash = emptyPayloadSHA256
	}

	timestamp := now.UTC().Format("20060102T150405Z")
	shortDate := now.UTC().Format("20060102")
	req.Header.Set("x-amz-date", timestamp)
	req.Header.Set("x-amz-content-sha256", payloadHash)
	if creds.SessionToken != "" {
		req.Header.Set("x-amz-security-token", creds.SessionToken)
	}

	canonicalRequest, signedHeaders := canonicalRequest(req, payloadHash)
	scope := credentialScope(shortDate, s.Region, service)
	stringToSign := strings.Join([]string{
		algorithm,
		timestamp,
		scope,
		hexSHA256([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(signingKey(creds.SecretKey, shortDate, s.Region, service), []byte(stringToSign)))
	authorization := fmt.Sprintf("%s Credential=%s/%s,SignedHeaders=%s,Signature=%s", algorithm, creds.AccessKey, scope, signedHeaders, signature)
	req.Header.Set("Authorization", authorization)

	return Signature{
		Authorization:    authorization,
		CanonicalRequest: canonicalRequest,
		StringToSign:     stringToSign,
		SignedHeaders:    signedHeaders,
		Signature:        signature,
		PayloadHash:      payloadHash,
	}, nil
}

func canonicalRequest(req *http.Request, payloadHash string) (string, string) {
	canonicalHeaders, signedHeaders := canonicalHeaders(req)
	return strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQuery(req.URL.Query()),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n"), signedHeaders
}

func canonicalURI(u *url.URL) string {
	if u == nil || u.Path == "" {
		return "/"
	}
	return uriEncode(u.Path, false)
}

func canonicalQuery(values url.Values) string {
	type pair struct {
		key   string
		value string
	}

	pairs := make([]pair, 0)
	for key, vals := range values {
		encodedKey := uriEncode(key, true)
		if len(vals) == 0 {
			pairs = append(pairs, pair{key: encodedKey})
			continue
		}
		for _, value := range vals {
			pairs = append(pairs, pair{key: encodedKey, value: uriEncode(value, true)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return pairs[i].value < pairs[j].value
		}
		return pairs[i].key < pairs[j].key
	})

	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		parts = append(parts, pair.key+"="+pair.value)
	}
	return strings.Join(parts, "&")
}

func canonicalHeaders(req *http.Request) (string, string) {
	headers := make(map[string][]string, len(req.Header)+1)
	for name, values := range req.Header {
		lower := strings.ToLower(name)
		if lower == "authorization" {
			continue
		}
		headers[lower] = append(headers[lower], values...)
	}
	headers["host"] = []string{host(req)}

	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	var canonical strings.Builder
	for _, name := range names {
		canonical.WriteString(name)
		canonical.WriteByte(':')
		canonical.WriteString(canonicalHeaderValue(headers[name]))
		canonical.WriteByte('\n')
	}
	return canonical.String(), strings.Join(names, ";")
}

func canonicalHeaderValue(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		cleaned = append(cleaned, strings.Join(strings.Fields(value), " "))
	}
	return strings.Join(cleaned, ",")
}

func host(req *http.Request) string {
	if req.Host != "" {
		return req.Host
	}
	if req.URL != nil {
		return req.URL.Host
	}
	return ""
}

func credentialScope(date string, region string, service string) string {
	return strings.Join([]string{date, region, service, awsRequest}, "/")
}

func signingKey(secret string, date string, region string, service string) []byte {
	dateKey := hmacSHA256([]byte("AWS4"+secret), []byte(date))
	regionKey := hmacSHA256(dateKey, []byte(region))
	serviceKey := hmacSHA256(regionKey, []byte(service))
	return hmacSHA256(serviceKey, []byte(awsRequest))
}

func hmacSHA256(key []byte, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func hexSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func uriEncode(value string, encodeSlash bool) string {
	var out strings.Builder
	const hexDigits = "0123456789ABCDEF"
	for i := 0; i < len(value); i++ {
		c := value[i]
		if isUnreserved(c) || (c == '/' && !encodeSlash) {
			out.WriteByte(c)
			continue
		}
		out.WriteByte('%')
		out.WriteByte(hexDigits[c>>4])
		out.WriteByte(hexDigits[c&0x0f])
	}
	return out.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '.' || c == '_' || c == '~'
}
