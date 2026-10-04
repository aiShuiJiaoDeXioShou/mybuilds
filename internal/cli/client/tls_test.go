package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRemoteExplicitCAAndLocalIsolation(t *testing.T) {
	t.Setenv("MYBUILDS_CLIENT_TOKEN", strings.Repeat("SECRET", 8))
	t.Setenv("SSL_CERT_FILE", "")
	t.Setenv("SSL_CERT_DIR", "")
	t.Setenv("MYBUILDS_CA_FILE", "")
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "自有CA"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"projects":0,"queued":0,"skipped":0,"running":0,"nodes":0}`))
	}))
	api.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER, caDER}, PrivateKey: key}}}
	api.StartTLS()
	defer api.Close()
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := executeRemote(t, "--server-url", api.URL, "status"); err == nil {
		t.Fatal("未知CA不应信任")
	}
	if _, err := executeRemote(t, "--server-url", api.URL, "--ca-file", p, "status"); err != nil {
		t.Fatal("显式CA请求失败", err)
	}
	t.Setenv("SSL_CERT_FILE", p)
	if _, err := executeRemote(t, "--server-url", api.URL, "--ca-file", p, "status"); err == nil {
		t.Fatal("隐式TLS材料被接受")
	}
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"projects":0,"queued":0,"skipped":0,"running":0,"nodes":0}`))
	}))
	defer plain.Close()
	if _, err := executeRemote(t, "--server-url", plain.URL, "--ca-file", p+"-missing", "status"); err != nil {
		t.Fatal("HTTP读取无关CA", err)
	}
	for _, args := range [][]string{{"--ca-file", p + "-missing", "version"}, {"--ca-file", p + "-missing", "--help"}} {
		if _, err := executeRemote(t, args...); err != nil {
			t.Fatal("本地命令读TLS配置", err)
		}
	}
}
