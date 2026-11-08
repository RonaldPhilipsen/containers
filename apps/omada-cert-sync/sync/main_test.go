package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func makePair(t *testing.T, key any, public any, notAfter time.Time) (certPEM, keyPEM []byte) {
	t.Helper()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "omada.example"},
		NotBefore:    time.Unix(1000, 0),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

func TestLoadLeaf(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(5000, 0)
	valid := time.Unix(9000, 0)

	certPEM, keyPEM := makePair(t, rsaKey, &rsaKey.PublicKey, valid)
	if _, err := loadLeaf(certPEM, keyPEM, now); err != nil {
		t.Errorf("valid RSA pair: %v", err)
	}
	certPEM, keyPEM = makePair(t, ecKey, &ecKey.PublicKey, valid)
	if _, err := loadLeaf(certPEM, keyPEM, now); err == nil || !strings.Contains(err.Error(), "RSA") {
		t.Errorf("ECDSA pair: error = %v, want RSA error", err)
	}
	certPEM, keyPEM = makePair(t, rsaKey, &rsaKey.PublicKey, time.Unix(2000, 0))
	if _, err := loadLeaf(certPEM, keyPEM, now); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Errorf("expired pair: error = %v, want validity error", err)
	}
}

func TestDialTarget(t *testing.T) {
	address, name, err := dialTarget("https://omada.example")
	if err != nil || address != "omada.example:443" || name != "omada.example" {
		t.Errorf("got %q, %q, %v", address, name, err)
	}
	if _, _, err := dialTarget("http://omada.example"); err == nil {
		t.Error("http URL: want error")
	}
}

func TestServedLeafAndWait(t *testing.T) {
	srv := httptest.NewTLSServer(nil)
	defer srv.Close()
	want := srv.Certificate()
	address := strings.TrimPrefix(srv.URL, "https://")
	got, err := servedLeaf(context.Background(), address, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Error("served certificate differs from the server certificate")
	}
	if err := waitForLeaf(context.Background(), address, "example.com", want); err != nil {
		t.Errorf("waitForLeaf() error = %v", err)
	}
}
