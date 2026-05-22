// This file is part of Vassago.
// See LICENSE-AGPLv3 for license information.

package daemon

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	pb "github.com/aj-nt/vassago-sdk/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

// TestTLSWithAuthIntegration verifies the full TLS + API key auth chain:
// daemon with TLS + API key → authenticated client succeeds, unauthenticated fails.
func TestTLSWithAuthIntegration(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath := generateSelfSignedCert(t, dir)

	cfg := DefaultConfig()
	cfg.Daemon.APIKey = "test-api-key"
	cfg.Daemon.TLS.Cert = certPath
	cfg.Daemon.TLS.Key = keyPath
	cfg.Store.Path = filepath.Join(dir, "vassago.db")

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	addr := lis.Addr().String()

	// Serve in background goroutine
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- d.Server().Serve(lis)
	}()

	// Wait for server to be ready (retry dial)
	var caCreds credentials.TransportCredentials
	for i := 0; i < 50; i++ {
		time.Sleep(50 * time.Millisecond)
		caCreds, err = credentials.NewClientTLSFromFile(caPath, "localhost")
		if err != nil {
			continue
		}
		conn, dialErr := grpc.NewClient(addr, grpc.WithTransportCredentials(caCreds))
		if dialErr != nil {
			continue
		}
		conn.Close()
		break
	}
	if err != nil {
		t.Fatalf("load CA cert: %v", err)
	}

	// Test 1: valid API key — should succeed
	t.Run("valid-credentials", func(t *testing.T) {
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(caCreds),
		)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()

		client := pb.NewVassagoClient(conn)
		md := metadata.Pairs("authorization", "Bearer test-api-key")
		authCtx := metadata.NewOutgoingContext(context.Background(), md)

		_, err = client.GetHotBlock(authCtx, &pb.HotBlockRequest{
			Target:    "memory",
			AgentId:   "test",
			CharLimit: 1,
		})
		if err != nil {
			t.Fatalf("authenticated request should pass, got: %v", err)
		}
	})

	// Test 2: no API key — should fail
	t.Run("no-credentials", func(t *testing.T) {
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(caCreds),
		)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()

		client := pb.NewVassagoClient(conn)
		_, err = client.GetHotBlock(context.Background(), &pb.HotBlockRequest{
			Target:    "memory",
			AgentId:   "test",
			CharLimit: 1,
		})
		if err == nil {
			t.Fatal("unauthenticated request should fail")
		}
	})

	d.Server().GracefulStop()
	<-serveDone
}

// generateSelfSignedCert creates self-signed CA + server cert. Returns (certPath, keyPath, caPath).
func generateSelfSignedCert(t *testing.T, dir string) (string, string, string) {
	t.Helper()

	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey CA: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate CA: %v", err)
	}
	caPath := filepath.Join(dir, "ca.pem")
	writePEMCert(t, caPath, "CERTIFICATE", caDER)

	srvKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey server: %v", err)
	}
	srvTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	srvDER, err := x509.CreateCertificate(rand.Reader, srvTmpl, caTmpl, &srvKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("CreateCertificate server: %v", err)
	}
	srvKeyDER, err := x509.MarshalPKCS8PrivateKey(srvKey)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}

	certPath := filepath.Join(dir, "server.pem")
	keyPath := filepath.Join(dir, "server-key.pem")
	writePEMCert(t, certPath, "CERTIFICATE", srvDER)
	writePEMCert(t, keyPath, "PRIVATE KEY", srvKeyDER)

	return certPath, keyPath, caPath
}

func writePEMCert(t *testing.T, path string, blockType string, der []byte) {
	t.Helper()
	block := &pem.Block{Type: blockType, Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatalf("writePEM %s: %v", path, err)
	}
}
