// omada-cert-sync pushes a mounted TLS certificate to an Omada controller.
//
// It uploads only when the controller serves a different certificate, so it
// is safe to run often. Credentials are sent only over a verified TLS
// connection: a controller with an untrusted certificate is an error.
package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/RonaldPhilipsen/omada/controller"
)

const (
	dialTimeout   = 10 * time.Second
	runTimeout    = 2 * time.Minute
	verifyTimeout = 45 * time.Second
	verifyPause   = 3 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("certificate sync failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := map[string]string{}
	for _, name := range []string{"OMADA_URL", "OMADA_USER", "OMADA_PASS"} {
		value := os.Getenv(name)
		if value == "" {
			return fmt.Errorf("%s is not set", name)
		}
		cfg[name] = value
	}
	tlsDir := os.Getenv("TLS_DIR")
	if tlsDir == "" {
		tlsDir = "/tls"
	}
	certPEM, err := os.ReadFile(filepath.Join(tlsDir, "tls.crt"))
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(filepath.Join(tlsDir, "tls.key"))
	if err != nil {
		return err
	}
	want, err := loadLeaf(certPEM, keyPEM, time.Now())
	if err != nil {
		return err
	}
	address, serverName, err := dialTarget(cfg["OMADA_URL"])
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	served, err := servedLeaf(ctx, address, serverName)
	if err != nil {
		return fmt.Errorf("read served certificate: %w", err)
	}
	if served.Equal(want) {
		slog.Info("certificate is current", "serial", want.SerialNumber.Text(16), "notAfter", want.NotAfter)
		return nil
	}

	slog.Info("uploading certificate", "serial", want.SerialNumber.Text(16), "servedSerial", served.SerialNumber.Text(16))
	client, err := controller.New(cfg["OMADA_URL"], cfg["OMADA_USER"], cfg["OMADA_PASS"], controller.WithTimeout(time.Minute))
	if err != nil {
		return fmt.Errorf("log in: %w", err)
	}
	defer client.Close()
	if err := client.SetCertificate(ctx, string(keyPEM), string(certPEM)); err != nil {
		return err
	}
	if err := waitForLeaf(ctx, address, serverName, want); err != nil {
		return err
	}
	slog.Info("certificate updated", "serial", want.SerialNumber.Text(16), "notAfter", want.NotAfter)
	return nil
}

// loadLeaf returns the leaf certificate after it checks that the controller
// can use the pair: a matching RSA key and a currently valid certificate.
func loadLeaf(certPEM, keyPEM []byte, now time.Time) (*x509.Certificate, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse key pair: %w", err)
	}
	if _, ok := pair.PrivateKey.(*rsa.PrivateKey); !ok {
		return nil, errors.New("private key must be RSA")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}
	if now.Before(leaf.NotBefore) || now.After(leaf.NotAfter) {
		return nil, fmt.Errorf("certificate is not valid now: valid from %s to %s", leaf.NotBefore, leaf.NotAfter)
	}
	return leaf, nil
}

// dialTarget returns the host:port to dial and the TLS server name for rawURL.
func dialTarget(rawURL string) (address, serverName string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("parse OMADA_URL: %w", err)
	}
	if u.Scheme != "https" || u.Hostname() == "" {
		return "", "", errors.New("OMADA_URL must be an https URL with a host")
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	return net.JoinHostPort(u.Hostname(), port), u.Hostname(), nil
}

// servedLeaf reads the certificate that the controller presents. The
// connection skips verification because it sends no data and the result is
// only compared with the wanted certificate.
func servedLeaf(ctx context.Context, address, serverName string) (*x509.Certificate, error) {
	dialer := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: dialTimeout},
		Config:    &tls.Config{ServerName: serverName, InsecureSkipVerify: true}, //nolint:gosec // See the function comment.
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, errors.New("controller presented no certificate")
	}
	return certs[0], nil
}

// waitForLeaf waits until the controller serves want.
func waitForLeaf(ctx context.Context, address, serverName string, want *x509.Certificate) error {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()
	var last error
	for {
		served, err := servedLeaf(ctx, address, serverName)
		switch {
		case err != nil:
			last = err
		case served.Equal(want):
			return nil
		default:
			last = fmt.Errorf("controller serves serial %s", served.SerialNumber.Text(16))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("controller did not apply the new certificate: %w", last)
		case <-time.After(verifyPause):
		}
	}
}
