package s3

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultSTSEndpoint  = "https://sts.amazonaws.com/"
	defaultIMDSEndpoint = "http://169.254.169.254"
)

// CredentialsProvider resolves S3 credentials at startup or refresh time.
type CredentialsProvider interface {
	Resolve(context.Context) (Credentials, error)
}

// CredentialsProviderFunc adapts a function to CredentialsProvider.
type CredentialsProviderFunc func(context.Context) (Credentials, error)

// Resolve calls fn(ctx).
func (fn CredentialsProviderFunc) Resolve(ctx context.Context) (Credentials, error) {
	return fn(ctx)
}

// StaticCredentialsProvider returns a fixed credential set.
type StaticCredentialsProvider struct {
	Credentials Credentials
}

// Resolve returns the configured credentials.
func (p StaticCredentialsProvider) Resolve(ctx context.Context) (Credentials, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, err
	}
	return validateCredentials(p.Credentials)
}

// EnvCredentialsProvider resolves AWS-compatible environment variables.
type EnvCredentialsProvider struct{}

// Resolve reads AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, and optional AWS_SESSION_TOKEN.
func (EnvCredentialsProvider) Resolve(ctx context.Context) (Credentials, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, err
	}
	return validateCredentials(Credentials{
		AccessKey:    os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretKey:    os.Getenv("AWS_SECRET_ACCESS_KEY"),
		SessionToken: os.Getenv("AWS_SESSION_TOKEN"),
	})
}

// WebIdentityProvider resolves credentials with STS AssumeRoleWithWebIdentity.
type WebIdentityProvider struct {
	Endpoint        string
	RoleARN         string
	RoleSessionName string
	TokenFile       string
	DurationSeconds int
	HTTPClient      *http.Client
}

// Resolve exchanges the web identity token for temporary credentials.
func (p WebIdentityProvider) Resolve(ctx context.Context) (Credentials, error) {
	if p.RoleARN == "" {
		return Credentials{}, fmt.Errorf("web identity role ARN is required")
	}
	if p.TokenFile == "" {
		return Credentials{}, fmt.Errorf("web identity token file is required")
	}
	token, err := os.ReadFile(p.TokenFile)
	if err != nil {
		return Credentials{}, fmt.Errorf("read web identity token: %w", err)
	}
	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = defaultSTSEndpoint
	}
	sessionName := p.RoleSessionName
	if sessionName == "" {
		sessionName = "kronos"
	}
	values := url.Values{}
	values.Set("Action", "AssumeRoleWithWebIdentity")
	values.Set("Version", "2011-06-15")
	values.Set("RoleArn", p.RoleARN)
	values.Set("RoleSessionName", sessionName)
	values.Set("WebIdentityToken", strings.TrimSpace(string(token)))
	if p.DurationSeconds > 0 {
		values.Set("DurationSeconds", fmt.Sprintf("%d", p.DurationSeconds))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return Credentials{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Credentials{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Credentials{}, fmt.Errorf("assume role with web identity failed: %s", resp.Status)
	}

	var out assumeRoleWithWebIdentityResponse
	if err := xml.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Credentials{}, fmt.Errorf("decode assume role with web identity response: %w", err)
	}
	return validateCredentials(Credentials{
		AccessKey:    out.Result.Credentials.AccessKeyID,
		SecretKey:    out.Result.Credentials.SecretAccessKey,
		SessionToken: out.Result.Credentials.SessionToken,
		ExpiresAt:    out.Result.Credentials.Expiration,
	})
}

// IMDSProvider resolves EC2 instance profile credentials through IMDSv2.
type IMDSProvider struct {
	Endpoint   string
	HTTPClient *http.Client
	TTL        time.Duration
}

// Resolve obtains an IMDSv2 token, role name, and temporary credentials.
func (p IMDSProvider) Resolve(ctx context.Context) (Credentials, error) {
	endpoint := strings.TrimRight(p.Endpoint, "/")
	if endpoint == "" {
		endpoint = defaultIMDSEndpoint
	}
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	ttl := p.TTL
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint+"/latest/api/token", nil)
	if err != nil {
		return Credentials{}, err
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", fmt.Sprintf("%d", int(ttl.Seconds())))
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return Credentials{}, err
	}
	token, err := readSuccessfulText(tokenResp)
	if err != nil {
		return Credentials{}, fmt.Errorf("get imdsv2 token: %w", err)
	}

	roleReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/latest/meta-data/iam/security-credentials/", nil)
	if err != nil {
		return Credentials{}, err
	}
	roleReq.Header.Set("X-aws-ec2-metadata-token", token)
	roleResp, err := client.Do(roleReq)
	if err != nil {
		return Credentials{}, err
	}
	roleName, err := readSuccessfulText(roleResp)
	if err != nil {
		return Credentials{}, fmt.Errorf("get imds role name: %w", err)
	}
	roleName = strings.TrimSpace(strings.Split(roleName, "\n")[0])
	if roleName == "" {
		return Credentials{}, fmt.Errorf("imds returned empty role name")
	}

	credsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/latest/meta-data/iam/security-credentials/"+url.PathEscape(roleName), nil)
	if err != nil {
		return Credentials{}, err
	}
	credsReq.Header.Set("X-aws-ec2-metadata-token", token)
	credsResp, err := client.Do(credsReq)
	if err != nil {
		return Credentials{}, err
	}
	defer credsResp.Body.Close()
	if credsResp.StatusCode < 200 || credsResp.StatusCode > 299 {
		return Credentials{}, fmt.Errorf("get imds credentials failed: %s", credsResp.Status)
	}
	var doc imdsCredentials
	if err := json.NewDecoder(credsResp.Body).Decode(&doc); err != nil {
		return Credentials{}, fmt.Errorf("decode imds credentials: %w", err)
	}
	if doc.Code != "Success" {
		return Credentials{}, fmt.Errorf("imds credentials response code %q", doc.Code)
	}
	return validateCredentials(Credentials{
		AccessKey:    doc.AccessKeyID,
		SecretKey:    doc.SecretAccessKey,
		SessionToken: doc.Token,
		ExpiresAt:    doc.Expiration,
	})
}

func validateCredentials(creds Credentials) (Credentials, error) {
	if creds.AccessKey == "" || creds.SecretKey == "" {
		return Credentials{}, fmt.Errorf("access key and secret key are required")
	}
	return creds, nil
}

func readSuccessfulText(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("request failed: %s", resp.Status)
	}
	var out strings.Builder
	if _, err := io.Copy(&out, io.LimitReader(resp.Body, 1024*1024)); err != nil {
		return "", err
	}
	return out.String(), nil
}

type assumeRoleWithWebIdentityResponse struct {
	Result assumeRoleWithWebIdentityResult `xml:"AssumeRoleWithWebIdentityResult"`
}

type assumeRoleWithWebIdentityResult struct {
	Credentials stsCredentials `xml:"Credentials"`
}

type stsCredentials struct {
	AccessKeyID     string    `xml:"AccessKeyId"`
	SecretAccessKey string    `xml:"SecretAccessKey"`
	SessionToken    string    `xml:"SessionToken"`
	Expiration      time.Time `xml:"Expiration"`
}

type imdsCredentials struct {
	Code            string    `json:"Code"`
	AccessKeyID     string    `json:"AccessKeyId"`
	SecretAccessKey string    `json:"SecretAccessKey"`
	Token           string    `json:"Token"`
	Expiration      time.Time `json:"Expiration"`
}
