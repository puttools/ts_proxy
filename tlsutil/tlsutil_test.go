package tlsutil

import (
	"crypto/x509"
	"testing"
)

func TestGenerateSelfSignedAndConfigs(t *testing.T) {
	certPEM, keyPEM, err := GenerateSelfSigned()
	if err != nil {
		t.Fatalf("GenerateSelfSigned() error = %v", err)
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Fatalf("empty cert/key")
	}
	if _, err := ServerConfig(certPEM, keyPEM); err != nil {
		t.Fatalf("ServerConfig() error = %v", err)
	}
	if _, err := ClientConfig(certPEM, false); err != nil {
		t.Fatalf("ClientConfig() error = %v", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		t.Fatalf("generated cert is not valid pem")
	}
}

