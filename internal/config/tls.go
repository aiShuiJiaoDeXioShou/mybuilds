package config

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
)

// TLSRoots仅供明确HTTPS消费者调用，不修改宿主环境或系统信任。
func TLSRoots(caFile string) (*x509.CertPool, error) {
	if os.Getenv("SSL_CERT_FILE") != "" || os.Getenv("SSL_CERT_DIR") != "" {
		return nil, errors.New("unsupported_tls_environment")
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		return nil, errors.New("tls_configuration_error")
	}
	roots = roots.Clone()
	if caFile == "" {
		return roots, nil
	}
	data, _, err := readConfiguration(caFile)
	if err != nil {
		return nil, errors.New("tls_configuration_error")
	}
	count := 0
	for len(bytes.TrimSpace(data)) > 0 {
		data = bytes.TrimSpace(data)
		if !bytes.HasPrefix(data, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, errors.New("tls_configuration_error")
		}
		block, rest := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, errors.New("tls_configuration_error")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !cert.IsCA || !cert.BasicConstraintsValid {
			return nil, errors.New("tls_configuration_error")
		}
		roots.AddCert(cert)
		count++
		data = rest
	}
	if count == 0 {
		return nil, errors.New("tls_configuration_error")
	}
	return roots, nil
}
