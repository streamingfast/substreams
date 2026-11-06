package clickhouse

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"io"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/ClickHouse/ch-go"
	"github.com/stretchr/testify/require"
)

func TestWireCounterCountsEveryConnectionItDials(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = conn.Write([]byte("pong"))
				_, _ = conn.Read(make([]byte, 16))
			}()
		}
	}()

	counter := &wireCounter{}

	// Two connections, because a retry replaces the cached client and the count has to
	// survive that rather than restart with the new socket.
	for range 2 {
		conn, err := counter.DialContext(context.Background(), "tcp", listener.Addr().String())
		require.NoError(t, err)

		_, err = conn.Write([]byte("hello"))
		require.NoError(t, err)

		_, err = conn.Read(make([]byte, 4))
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	}

	require.Equal(t, int64(10), counter.BytesWritten(), "5 bytes on each of two connections")
}

func TestWireCounterIsZeroBeforeAnythingIsDialed(t *testing.T) {
	require.Zero(t, (&wireCounter{}).BytesWritten())
}

func TestDialTLSCountsBytesUnderTheHandshake(t *testing.T) {
	cert, pool := testCertificate(t)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	require.NoError(t, err)
	defer listener.Close()

	// Completes the TLS handshake, then hangs up before any ClickHouse hello.
	served := make(chan struct{})
	go func() {
		defer close(served)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_ = conn.(*tls.Conn).Handshake()
	}()

	counter := &wireCounter{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = dialTLS(ctx, ch.Options{
		Address:          listener.Addr().String(),
		DialTimeout:      2 * time.Second,
		HandshakeTimeout: 2 * time.Second,
	}, &tls.Config{RootCAs: pool}, counter)
	<-served

	// Past the TLS handshake, so it failed in ch.Connect, not on the certificate.
	require.ErrorContains(t, err, "connect")
	require.False(t, isPermanentTLSError(err))
	require.Greater(t, counter.BytesWritten(), int64(0))
}

func TestDialTLSRejectsUntrustedCertificateAsPermanent(t *testing.T) {
	cert, _ := testCertificate(t)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	require.NoError(t, err)
	defer listener.Close()

	served := make(chan struct{})
	go func() {
		defer close(served)
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_ = conn.(*tls.Conn).Handshake()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Empty RootCAs: the self-signed certificate is not trusted.
	_, err = dialTLS(ctx, ch.Options{Address: listener.Addr().String(), DialTimeout: 2 * time.Second}, &tls.Config{}, &wireCounter{})
	<-served

	require.Error(t, err)
	require.True(t, isPermanentTLSError(err), "got %v", err)
}

func TestIsPermanentTLSErrorIgnoresNetworkErrors(t *testing.T) {
	require.False(t, isPermanentTLSError(&net.OpError{Op: "dial", Err: io.EOF}))
}

func testCertificate(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	leaf, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	pool := x509.NewCertPool()
	pool.AddCert(leaf)

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}
