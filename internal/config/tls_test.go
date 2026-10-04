package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTLSRootsExplicitCA(t *testing.T) {
	t.Setenv("SSL_CERT_FILE", "")
	t.Setenv("SSL_CERT_DIR", "")
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "自有测试CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	roots, err := TLSRoots(p)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots}); err != nil {
		t.Fatal("显式CA未受信任", err)
	}
	if _, err := TLSRoots(""); err != nil {
		t.Fatal("系统根不可用", err)
	}
	for _, content := range []string{"SECRET", string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("SECRET")}))} {
		os.WriteFile(p, []byte(content), 0600)
		if _, err := TLSRoots(p); err == nil || err.Error() != "tls_configuration_error" {
			t.Fatal("非法CA未固定失败", err)
		}
	}
	ca.IsCA = false
	der, _ = x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	if _, err := TLSRoots(p); err == nil {
		t.Fatal("非CA被当CA")
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(p, link)
	if _, err := TLSRoots(link); err == nil {
		t.Fatal("CA叶子链接被读取")
	}
	t.Setenv("SSL_CERT_FILE", p)
	if _, err := TLSRoots(""); err == nil || err.Error() != "unsupported_tls_environment" {
		t.Fatal("隐式TLS材料未拒绝", err)
	}
}
