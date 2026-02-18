package ledger

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func dialOptions(cfg *Config, extra []grpc.DialOption) ([]grpc.DialOption, error) {
	var opts []grpc.DialOption

	if cfg.TLS.Enabled {
		tlsCfg, err := loadTLSConfig(cfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("load TLS config: %w", err)
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if cfg.MaxMessageSize > 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(cfg.MaxMessageSize)))
	}

	opts = append(opts, extra...)
	return opts, nil
}

func loadTLSConfig(c *TLSConfig) (*tls.Config, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: c.InsecureSkipVerify, //nolint:gosec // for testing only this flag is true
		NextProtos:         []string{"h2"},
	}

	if c.CertFile != "" && c.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client cert/key: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	if c.CAFile != "" {
		b, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("append CA certs from PEM failed")
		}
		tlsCfg.RootCAs = pool
	}

	return tlsCfg, nil
}
