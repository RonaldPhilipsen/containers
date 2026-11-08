// db-bridge: mTLS Postgres bridge.
//
// It is also the container entrypoint (PID 1 in a FROM scratch image): it
// starts and supervises spiffe-helper, waits for the first SVID, then
// serves.
//
// Listens on 127.0.0.1:5432 (plain TCP). For each app connection it opens
// a connection to the upstream Postgres, performs the Postgres SSL
// negotiation (SSLRequest / 'S'), upgrades that connection to TLS
// presenting the pod's SPIFFE SVID as the client certificate, then pipes
// bytes both ways.
//
// The app speaks the plain Postgres protocol to the bridge (no password);
// the server authenticates the SVID (CN=app) via pg_hba "cert". The SVID
// files are re-read on every new connection, so spiffe-helper rotation is
// absorbed without a restart: existing sessions keep their (still valid)
// old certificate, new connections pick up the renewed one.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// upstream is the Postgres rw service (host:port). The host part is also
// the TLS server name that the server certificate must match. Set by
// DB_BRIDGE_UPSTREAM, so one image serves every CNPG cluster.
var upstream, upstreamHost string

// listenAddr is where the app connects (plain Postgres). DB_BRIDGE_LISTEN
// lets several bridges share a pod on different ports; the -health probe
// reads the same variable, since exec probes inherit the container env.
var listenAddr = func() string {
	if v := os.Getenv("DB_BRIDGE_LISTEN"); v != "" {
		return v
	}
	return defaultListen
}()

func initUpstream() {
	upstream = os.Getenv("DB_BRIDGE_UPSTREAM")
	if upstream == "" {
		upstream = defaultUpstream
	}
	host, _, err := net.SplitHostPort(upstream)
	if err != nil || host == "" {
		log.Fatalf("DB_BRIDGE_UPSTREAM %q: want host:port", upstream)
	}
	upstreamHost = host
}

const (
	// defaultListen is used when DB_BRIDGE_LISTEN is unset.
	defaultListen = "127.0.0.1:5432"
	// defaultUpstream is used when DB_BRIDGE_UPSTREAM is unset.
	defaultUpstream = "postgres-rw.database.svc:5432"
	svidCert        = "/spiffe/svid.crt"
	svidKey         = "/spiffe/svid.key"
	caFile          = "/cnpg-ca/ca.crt"

	helperBinary = "/usr/local/bin/spiffe-helper"
	helperConfig = "/etc/spiffe-helper/helper.conf"
	svidTimeout  = 60 * time.Second
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)

	// db-bridge -health: liveness probe (is the listener up?)
	if len(os.Args) > 1 && os.Args[1] == "-health" {
		c, err := net.Dial("tcp", listenAddr)
		if err != nil {
			log.Printf("health: %v", err)
			os.Exit(1)
		}
		_ = c.Close()
		return
	}

	initUpstream()

	// Register for signals before starting the child, so an early SIGTERM
	// is not lost.
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)

	helper := startHelper()
	if err := waitForSVID(sigs, helper); err != nil {
		stopHelper(helper)
		log.Fatalf("%v", err)
	}

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		stopHelper(helper)
		log.Fatalf("listen: %v", err)
	}
	log.Printf("db-bridge listening on %s -> %s", listenAddr, upstream)

	go func() {
		select {
		case <-sigs:
			// helper first: closing the listener ends main()
			stopHelper(helper)
			_ = ln.Close()
		case err := <-helper.done:
			// Without spiffe-helper the SVID is never renewed: exit, and
			// let the kubelet restart the container.
			log.Fatalf("spiffe-helper exited: %v", err)
		}
	}()

	for {
		client, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			return
		}
		go handle(client)
	}
}

// helperProc is the supervised spiffe-helper child. done receives its exit
// status exactly once.
type helperProc struct {
	cmd  *exec.Cmd
	done chan error
}

// startHelper runs spiffe-helper, which fetches the pod's SVID from the
// SPIRE agent and renews it in /spiffe. The image is FROM scratch, so this
// binary is PID 1 and takes the place of a shell entrypoint.
func startHelper() *helperProc {
	cmd := exec.Command(helperBinary, "-config", helperConfig)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Fatalf("start spiffe-helper: %v", err)
	}
	h := &helperProc{cmd: cmd, done: make(chan error, 1)}
	go func() { h.done <- cmd.Wait() }()
	return h
}

func stopHelper(h *helperProc) {
	_ = h.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-h.done:
	case <-time.After(5 * time.Second):
		_ = h.cmd.Process.Kill()
	}
}

// waitForSVID blocks until spiffe-helper has written a non-empty SVID and
// key, for at most svidTimeout.
func waitForSVID(sigs <-chan os.Signal, h *helperProc) error {
	deadline := time.After(svidTimeout)
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if nonEmpty(svidCert) && nonEmpty(svidKey) {
			return nil
		}
		select {
		case <-tick.C:
		case <-deadline:
			return fmt.Errorf("SVID never appeared in /spiffe after %s", svidTimeout)
		case err := <-h.done:
			return fmt.Errorf("spiffe-helper exited before the SVID appeared: %v", err)
		case s := <-sigs:
			return fmt.Errorf("received %v while waiting for the SVID", s)
		}
	}
}

func nonEmpty(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 0
}

// Postgres pre-startup request codes (sent by the client in an 8-byte
// message before its StartupMessage).
const (
	codeSSLRequest    = 80877103
	codeGSSENCRequest = 80877104
)

// readStartup answers the client's own SSLRequest/GSSENCRequest with 'N'
// (loopback stays plaintext) and returns the first real startup-phase
// message (StartupMessage or CancelRequest), to be replayed upstream inside
// the TLS session. Without this, the client's SSLRequest would be forwarded
// into the already-encrypted upstream session and Postgres would answer 'E'.
func readStartup(client net.Conn) ([]byte, error) {
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(client, hdr[:]); err != nil {
			return nil, err
		}
		length := binary.BigEndian.Uint32(hdr[0:4])
		code := binary.BigEndian.Uint32(hdr[4:8])
		if length == 8 && (code == codeSSLRequest || code == codeGSSENCRequest) {
			if _, err := client.Write([]byte{'N'}); err != nil {
				return nil, err
			}
			continue
		}
		if length < 8 || length > 10000 {
			return nil, fmt.Errorf("invalid startup message length %d", length)
		}
		msg := make([]byte, length)
		copy(msg, hdr[:])
		if _, err := io.ReadFull(client, msg[8:]); err != nil {
			return nil, err
		}
		return msg, nil
	}
}

func handle(client net.Conn) {
	defer client.Close()

	startup, err := readStartup(client)
	if err != nil {
		if err != io.EOF { // EOF: the -health probe connects and closes
			log.Printf("client startup: %v", err)
		}
		return
	}

	server, err := net.Dial("tcp", upstream)
	if err != nil {
		log.Printf("dial upstream: %v", err)
		return
	}
	defer server.Close()

	// Postgres SSLRequest: 8-byte message, code 80877103 (0x04D2162F).
	sslRequest := []byte{0, 0, 0, 8, 0x04, 0xD2, 0x16, 0x2F}
	if _, err := server.Write(sslRequest); err != nil {
		log.Printf("ssl request write: %v", err)
		return
	}
	var b [1]byte
	if _, err := io.ReadFull(server, b[:]); err != nil || b[0] != 'S' {
		log.Printf("ssl negotiation: %v (reply %q)", err, string(b[:]))
		return
	}

	tlsCfg, err := tlsConfig()
	if err != nil {
		log.Printf("tls config: %v", err)
		return
	}
	tlsConn := tls.Client(server, tlsCfg)
	if err := tlsConn.Handshake(); err != nil {
		log.Printf("tls handshake: %v", err)
		return
	}
	if _, err := tlsConn.Write(startup); err != nil {
		log.Printf("startup replay: %v", err)
		return
	}
	log.Printf("upstream connection established (%s)", server.RemoteAddr())

	// Pipe both ways; when either direction ends, tear both down.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(tlsConn, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, tlsConn); done <- struct{}{} }()
	<-done
}

// tlsConfig loads the SVID (client cert + key) and the server CA bundle
// fresh on every call, so SVID rotation is picked up per connection.
func tlsConfig() (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(svidCert, svidKey)
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(caFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, io.ErrUnexpectedEOF
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		ServerName:   upstreamHost,
		MinVersion:   tls.VersionTLS12,
	}, nil
}
