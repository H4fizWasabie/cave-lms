package storage

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestR2PutGetDelete runs against a real, dedicated test bucket, like the app tests run against a test database.
func TestR2PutGetDelete(t *testing.T) {
	account, bucket, key, secret := os.Getenv("TEST_R2_ACCOUNT_ID"), os.Getenv("TEST_R2_BUCKET"), os.Getenv("TEST_R2_ACCESS_KEY_ID"), os.Getenv("TEST_R2_SECRET_ACCESS_KEY")
	if account == "" || bucket == "" || key == "" || secret == "" {
		t.Skip("set TEST_R2_ACCOUNT_ID, TEST_R2_BUCKET, TEST_R2_ACCESS_KEY_ID and TEST_R2_SECRET_ACCESS_KEY to a dedicated test bucket")
	}
	var s Store = NewR2(account, bucket, key, secret)
	name := "contract-test-" + time.Now().UTC().Format("20060102T150405.000000000")
	data := []byte(`{"rows":[["Kopi",4321.5]]}`)

	ctx := context.Background()
	if err := s.Put(ctx, name, data); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, name)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("Get after Put = %q, %v; want %q", got, err, data)
	}
	if err := s.Delete(ctx, name); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, name); err == nil {
		t.Fatal("Get after Delete found the object")
	}
	if err := s.Delete(ctx, name); err != nil {
		t.Fatalf("deleting a deleted object: %v", err)
	}
}

// TestSignMatchesAWSExample checks the signer against the GET Object example in AWS's Signature Version 4 docs,
// the only check of it that runs without a bucket.
func TestSignMatchesAWSExample(t *testing.T) {
	req, err := http.NewRequest("GET", "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-9")
	sign(req, nil, "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "us-east-1", time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC))
	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization =\n%s\nwant\n%s", got, want)
	}
}

// TestURIEncodeKeepsOnlyUnreservedBytesAndSlashes checks the canonical path escaping Signature Version 4 asks for,
// which differs from Go's path escaping on characters such as + and $.
func TestURIEncodeKeepsOnlyUnreservedBytesAndSlashes(t *testing.T) {
	if got, want := uriEncode("/bucket/a b+c$=~é/x_y-z.json"), "/bucket/a%20b%2Bc%24%3D~%C3%A9/x_y-z.json"; got != want {
		t.Fatalf("uriEncode = %s, want %s", got, want)
	}
}
