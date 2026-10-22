package storage

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// R2 keeps objects in a private Cloudflare R2 bucket through its S3-compatible API (ADR 0008).
// Requests are signed here with AWS Signature Version 4: three object calls do not justify an SDK.
type R2 struct {
	base              string // https://<account>.r2.cloudflarestorage.com/<bucket>/
	accessKey, secret string
	client            *http.Client
}

// NewR2 returns the store for bucket in account, reached with an R2 API token's access key and secret.
func NewR2(account, bucket, accessKey, secret string) *R2 {
	return &R2{
		base:      "https://" + account + ".r2.cloudflarestorage.com/" + url.PathEscape(bucket) + "/",
		accessKey: accessKey,
		secret:    secret,
		client:    &http.Client{Timeout: time.Minute},
	}
}

// Put writes data under key, replacing any object there.
func (r *R2) Put(key string, data []byte) error {
	_, err := r.do(http.MethodPut, key, data)
	return err
}

// Get returns the object stored under key.
func (r *R2) Get(key string) ([]byte, error) {
	return r.do(http.MethodGet, key, nil)
}

// Delete removes the object under key; R2 answers a missing key with success, as Disk does.
func (r *R2) Delete(key string) error {
	_, err := r.do(http.MethodDelete, key, nil)
	return err
}

func (r *R2) do(method, key string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, r.base+url.PathEscape(key), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	sign(req, body, r.accessKey, r.secret, "auto", time.Now())
	res, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("R2 %s %s: %w", method, key, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("R2 %s %s: %w", method, key, err)
	}
	if res.StatusCode/100 != 2 {
		if len(data) > 300 {
			data = data[:300]
		}
		return nil, fmt.Errorf("R2 %s %s: %s: %s", method, key, res.Status, data)
	}
	return data, nil
}

// sign adds the x-amz-date, x-amz-content-sha256 and Authorization headers, signing the host and every header set on req.
func sign(req *http.Request, body []byte, accessKey, secret, region string, now time.Time) {
	now = now.UTC()
	day, stamp := now.Format("20060102"), now.Format("20060102T150405Z")
	payload := sha256Hex(body)
	req.Header.Set("X-Amz-Date", stamp)
	req.Header.Set("X-Amz-Content-Sha256", payload)

	values := map[string]string{"host": req.URL.Host}
	for name, v := range req.Header {
		values[strings.ToLower(name)] = strings.TrimSpace(strings.Join(v, ","))
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var headers strings.Builder
	for _, name := range names {
		headers.WriteString(name + ":" + values[name] + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{req.Method, req.URL.EscapedPath(), req.URL.RawQuery, headers.String(), signed, payload}, "\n")

	scope := day + "/" + region + "/s3/aws4_request"
	key := []byte("AWS4" + secret)
	for _, part := range []string{day, region, "s3", "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	signature := hex.EncodeToString(hmacSHA256(key, "AWS4-HMAC-SHA256\n"+stamp+"\n"+scope+"\n"+sha256Hex([]byte(canonical))))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+signature)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}
