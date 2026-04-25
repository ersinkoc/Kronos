package s3

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStaticCredentialsProvider(t *testing.T) {
	t.Parallel()

	creds, err := (StaticCredentialsProvider{Credentials: testCredentials}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if creds.AccessKey != testCredentials.AccessKey || creds.SecretKey != testCredentials.SecretKey {
		t.Fatalf("Resolve() = %#v", creds)
	}
}

func TestEnvCredentialsProvider(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "env-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "env-secret")
	t.Setenv("AWS_SESSION_TOKEN", "env-token")

	creds, err := EnvCredentialsProvider{}.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if creds.AccessKey != "env-access" || creds.SecretKey != "env-secret" || creds.SessionToken != "env-token" {
		t.Fatalf("Resolve() = %#v", creds)
	}
}

func TestWebIdentityProvider(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("jwt-token\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method must be POST", http.StatusMethodNotAllowed)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Form.Get("Action") != "AssumeRoleWithWebIdentity" || r.Form.Get("WebIdentityToken") != "jwt-token" {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <AssumeRoleWithWebIdentityResult>
    <Credentials>
      <AccessKeyId>sts-access</AccessKeyId>
      <SecretAccessKey>sts-secret</SecretAccessKey>
      <SessionToken>sts-token</SessionToken>
      <Expiration>2026-04-24T12:00:00Z</Expiration>
    </Credentials>
  </AssumeRoleWithWebIdentityResult>
</AssumeRoleWithWebIdentityResponse>`)
	}))
	defer server.Close()

	creds, err := (WebIdentityProvider{
		Endpoint:        server.URL,
		RoleARN:         "arn:aws:iam::123456789012:role/kronos",
		RoleSessionName: "kronos-test",
		TokenFile:       tokenPath,
		HTTPClient:      server.Client(),
	}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if creds.AccessKey != "sts-access" || creds.SecretKey != "sts-secret" || creds.SessionToken != "sts-token" {
		t.Fatalf("Resolve() = %#v", creds)
	}
}

func TestIMDSProvider(t *testing.T) {
	t.Parallel()

	var sawTokenHeader atomic.Bool
	expires := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token":
			if r.Header.Get("X-aws-ec2-metadata-token-ttl-seconds") == "" {
				http.Error(w, "missing token ttl header", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, "imdsv2-token")
		case r.Method == http.MethodGet && r.URL.Path == "/latest/meta-data/iam/security-credentials/":
			if r.Header.Get("X-aws-ec2-metadata-token") == "imdsv2-token" {
				sawTokenHeader.Store(true)
			}
			fmt.Fprint(w, "kronos-role\n")
		case r.Method == http.MethodGet && r.URL.Path == "/latest/meta-data/iam/security-credentials/kronos-role":
			if r.Header.Get("X-aws-ec2-metadata-token") != "imdsv2-token" {
				http.Error(w, "missing imds token header", http.StatusBadRequest)
				return
			}
			fmt.Fprintf(w, `{"Code":"Success","AccessKeyId":"imds-access","SecretAccessKey":"imds-secret","Token":"imds-token","Expiration":%q}`, expires.Format(time.RFC3339))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	creds, err := (IMDSProvider{Endpoint: server.URL, HTTPClient: server.Client()}).Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if !sawTokenHeader.Load() {
		t.Fatal("role request did not include IMDS token")
	}
	if creds.AccessKey != "imds-access" || creds.SecretKey != "imds-secret" || creds.SessionToken != "imds-token" {
		t.Fatalf("Resolve() = %#v", creds)
	}
}

func TestBackendUsesCredentialsProvider(t *testing.T) {
	t.Parallel()

	endpoint := &url.URL{Scheme: "http", Host: "127.0.0.1"}
	backend, err := New(Config{
		Endpoint: endpoint.String(),
		Region:   "us-east-1",
		Bucket:   "bucket",
		CredentialsProvider: CredentialsProviderFunc(func(context.Context) (Credentials, error) {
			return Credentials{AccessKey: "provider-access", SecretKey: "provider-secret"}, nil
		}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if !strings.Contains(backend.creds.AccessKey, "provider") {
		t.Fatalf("backend creds = %#v", backend.creds)
	}
}

func TestCredentialProvidersRejectInvalidInputs(t *testing.T) {
	t.Parallel()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (StaticCredentialsProvider{Credentials: testCredentials}).Resolve(canceled); err == nil {
		t.Fatal("StaticCredentialsProvider canceled error = nil, want error")
	}
	if _, err := (EnvCredentialsProvider{}).Resolve(canceled); err == nil {
		t.Fatal("EnvCredentialsProvider canceled error = nil, want error")
	}
	if _, err := validateCredentials(Credentials{AccessKey: "only-access"}); err == nil {
		t.Fatal("validateCredentials(partial) error = nil, want error")
	}
	if _, err := (CredentialsProviderFunc(func(context.Context) (Credentials, error) {
		return testCredentials, nil
	})).Resolve(context.Background()); err != nil {
		t.Fatalf("CredentialsProviderFunc.Resolve() error = %v", err)
	}
}

func TestWebIdentityProviderRejectsBadResponses(t *testing.T) {
	t.Parallel()

	if _, err := (WebIdentityProvider{}).Resolve(context.Background()); err == nil {
		t.Fatal("Resolve(missing role ARN) error = nil, want error")
	}
	if _, err := (WebIdentityProvider{RoleARN: "arn"}).Resolve(context.Background()); err == nil {
		t.Fatal("Resolve(missing token file) error = nil, want error")
	}
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte("jwt-token"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer server.Close()
	if _, err := (WebIdentityProvider{Endpoint: server.URL, RoleARN: "arn", TokenFile: tokenPath, HTTPClient: server.Client()}).Resolve(context.Background()); err == nil {
		t.Fatal("Resolve(forbidden) error = nil, want error")
	}
}

func TestIMDSProviderRejectsBadResponses(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/api/token":
			fmt.Fprint(w, "token")
		case "/latest/meta-data/iam/security-credentials/":
			fmt.Fprint(w, "\n")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if _, err := (IMDSProvider{Endpoint: server.URL, HTTPClient: server.Client()}).Resolve(context.Background()); err == nil {
		t.Fatal("Resolve(empty role name) error = nil, want error")
	}

	badCode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/api/token":
			fmt.Fprint(w, "token")
		case "/latest/meta-data/iam/security-credentials/":
			fmt.Fprint(w, "role")
		case "/latest/meta-data/iam/security-credentials/role":
			fmt.Fprint(w, `{"Code":"Denied"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer badCode.Close()
	if _, err := (IMDSProvider{Endpoint: badCode.URL, HTTPClient: badCode.Client()}).Resolve(context.Background()); err == nil {
		t.Fatal("Resolve(bad code) error = nil, want error")
	}
}
