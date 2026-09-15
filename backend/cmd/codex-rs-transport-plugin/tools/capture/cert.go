package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

type generatedPKI struct {
	Certificate tls.Certificate
	CACertPEM   []byte
	LeafCertPEM []byte
	LeafKeyPEM  []byte
}

func loadOrMakeCertificate(certFile, keyFile, serverName string) (generatedPKI, error) {
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return generatedPKI{}, fmt.Errorf("-cert and -key must be set together")
		}
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return generatedPKI{}, err
		}
		return generatedPKI{Certificate: cert}, nil
	}
	return generateLocalCA(serverName)
}

func generateLocalCA(serverName string) (generatedPKI, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return generatedPKI{}, err
	}
	caSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return generatedPKI{}, err
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          caSerial,
		Subject:               pkix.Name{CommonName: "sub2api-codex-capture-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return generatedPKI{}, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return generatedPKI{}, err
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return generatedPKI{}, err
	}
	leafSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return generatedPKI{}, err
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: leafSerial,
		Subject:      pkix.Name{CommonName: serverName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{serverName},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return generatedPKI{}, err
	}
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return generatedPKI{}, err
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	leafPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER})
	cert, err := tls.X509KeyPair(append(append([]byte{}, leafPEM...), caPEM...), leafKeyPEM)
	if err != nil {
		return generatedPKI{}, err
	}
	return generatedPKI{
		Certificate: cert,
		CACertPEM:   caPEM,
		LeafCertPEM: leafPEM,
		LeafKeyPEM:  leafKeyPEM,
	}, nil
}

func writeGeneratedPKI(pki generatedPKI, dir string) error {
	if len(pki.CACertPEM) == 0 {
		return nil
	}
	if err := os.WriteFile(filepath.Join(dir, "inspector-ca.pem"), pki.CACertPEM, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "inspector.pem"), pki.LeafCertPEM, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "inspector-key.pem"), pki.LeafKeyPEM, 0o600)
}
