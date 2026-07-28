// SPDX-License-Identifier: Apache-2.0

package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
)

// ParseRSAPrivateKey decodes a PEM-encoded RSA private key in either PKCS#1
// ("RSA PRIVATE KEY") or PKCS#8 ("PRIVATE KEY") form.
func ParseRSAPrivateKey(pemString string) (*rsa.PrivateKey, error) {
	pemBytes, err := base64.StdEncoding.DecodeString(pemString)
	if err != nil {
		return nil, fmt.Errorf("JWT signing key is not valid base64: %w", err)
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found in key")
	}

	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse RSA private key: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("key is %T, want RSA", parsed)
	}
	return key, nil
}
