//go:build darwin || linux

package store

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testCertificate(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, client bool) ([]byte, []byte, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: "store006 自有测试证书"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true}
	parent, signer := ca, caKey
	if ca == nil {
		cert.IsCA = true
		cert.KeyUsage |= x509.KeyUsageCertSign
		parent = cert
		signer = key
	} else if client {
		cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		cert.DNSNames = []string{"localhost"}
		cert.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	data, err := x509.CreateCertificate(rand.Reader, cert, parent, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(data)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: data}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), parsed, key
}

func TestPostgresTLSMemoryAndActualHandshake(t *testing.T) {
	caPEM, _, ca, caKey := testCertificate(t, nil, nil, false)
	serverPEM, serverKey, _, _ := testCertificate(t, ca, caKey, false)
	clientPEM, clientKey, _, _ := testCertificate(t, ca, caKey, true)
	dir := t.TempDir()
	caFile, certFile, keyFile := filepath.Join(dir, "ca.crt"), filepath.Join(dir, "client.crt"), filepath.Join(dir, "client.key")
	for name, data := range map[string][]byte{caFile: caPEM, certFile: clientPEM, keyFile: clientKey} {
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	config, err := postgresConfig(testContext, "host=127.0.0.1 user=test dbname=control sslmode=verify-full sslrootcert='"+caFile+"' sslcert='"+certFile+"' sslkey='"+keyFile+"'")
	if err != nil {
		t.Fatal(err)
	}
	caConfig, err := postgresConfig(testContext, "host=127.0.0.1 user=test dbname=control sslmode=require sslrootcert='"+caFile+"' sslcert='"+certFile+"' sslkey='"+keyFile+"'")
	if err != nil {
		t.Fatal(err)
	}
	// 解析完删除源文件，真实握手仍使用此前限额读取的同一批证据。
	for _, name := range []string{caFile, certFile, keyFile} {
		if err := os.Remove(name); err != nil {
			t.Fatal(err)
		}
	}
	serverCertificate, err := tls.X509KeyPair(serverPEM, serverKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	handshake := func(client *tls.Config) error {
		listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{serverCertificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				done <- err
				return
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			done <- conn.(*tls.Conn).Handshake()
		}()
		conn, clientErr := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", listener.Addr().String(), client)
		listener.Close()
		serverErr := <-done
		if conn != nil {
			conn.Close()
		}
		if clientErr == nil && serverErr != nil {
			t.Fatal("server rejected actual client certificate")
		}
		return clientErr
	}
	if err = handshake(config.TLSConfig.Clone()); err != nil {
		t.Fatal("verified TLS handshake failed")
	}
	// require 配合显式 CA 遵循 pgx 的 verify-ca 语义，验证链但不核对名称。
	chainOnly := caConfig.TLSConfig.Clone()
	chainOnly.ServerName = "unmatched-but-chain-valid"
	if err = handshake(chainOnly); err != nil {
		t.Fatal("explicit CA chain verification failed")
	}
	untrustedChain := caConfig.TLSConfig.Clone()
	untrustedChain.VerifyPeerCertificate = func(raw [][]byte, _ [][]*x509.Certificate) error { return verifyPostgresChain(raw, x509.NewCertPool()) }
	if err = handshake(untrustedChain); err == nil {
		t.Fatal("verify-ca accepted unknown CA")
	}
	wrongHost := config.TLSConfig.Clone()
	wrongHost.ServerName = "wrong-host-marker"
	if err = handshake(wrongHost); err == nil {
		t.Fatal("verify-full accepted wrong hostname")
	}
	wrongCA := config.TLSConfig.Clone()
	wrongCA.RootCAs = x509.NewCertPool()
	if err = handshake(wrongCA); err == nil {
		t.Fatal("verify-full accepted unknown CA")
	}
}

func TestPostgresTLSUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "invalid-marker.crt")
	if err := os.WriteFile(regular, []byte("invalid-secret-material-marker"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "linked.crt")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(dir, "large.crt")
	if err := os.WriteFile(large, []byte(strings.Repeat("x", (1<<20)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{dir, link, large, regular} {
		_, err := postgresConfig(testContext, "host=127.0.0.1 user=test dbname=control sslmode=verify-full sslrootcert='"+file+"'")
		if !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), "marker") {
			t.Fatal("unsafe TLS material was accepted or leaked")
		}
	}
}
