//go:build ignore

// inspect-package.go — Download a Canton package archive and extract printable strings
// to discover template/choice names. Hacky but effective for choice discovery.
//
// Usage:
//   go run scripts/testing/inspect-package.go \
//     -config config.api-server.devnet-test.yaml \
//     -pkg "7a75ef6e69f69395a4e60919e228528bb8f3881150ccfde3f31bcc73864b18ab" \
//     -filter "TransferOffer"

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	cfgpkg "github.com/chainsafe/canton-middleware/pkg/config"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Config")
	pkgID      = flag.String("pkg", "", "Package ID to download (required)")
	filter     = flag.String("filter", "", "Substring filter on extracted strings")
	minLen     = flag.Int("min-len", 5, "Min printable string length")
)

func main() {
	flag.Parse()
	if *pkgID == "" {
		fatalf("-pkg flag is required")
	}

	cfg, err := cfgpkg.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("config: %v", err)
	}

	ctx := context.Background()
	token, err := fetchOAuthToken(
		cfg.Canton.Ledger.Auth.TokenURL,
		cfg.Canton.Ledger.Auth.ClientID,
		cfg.Canton.Ledger.Auth.ClientSecret,
		cfg.Canton.Ledger.Auth.Audience,
	)
	if err != nil {
		fatalf("oauth: %v", err)
	}

	target := cfg.Canton.Ledger.RPCURL
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}
	var dialOpts []grpc.DialOption
	if cfg.Canton.Ledger.TLS != nil && cfg.Canton.Ledger.TLS.Enabled {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(
			expcreds.NewTLSWithALPNDisabled(&tls.Config{MinVersion: tls.VersionTLS12}),
		))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		fatalf("dial: %v", err)
	}
	defer conn.Close()

	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	pkgClient := lapiv2.NewPackageServiceClient(conn)
	getCtx, cancel := context.WithTimeout(authCtx, 60*time.Second)
	defer cancel()
	resp, err := pkgClient.GetPackage(getCtx, &lapiv2.GetPackageRequest{
		PackageId: *pkgID,
	})
	if err != nil {
		fatalf("GetPackage: %v", err)
	}

	fmt.Printf(">>> Got package %s (%d bytes)\n", *pkgID, len(resp.ArchivePayload))
	fmt.Println()

	strs := extractStrings(resp.ArchivePayload, *minLen)
	uniq := make(map[string]struct{}, len(strs))
	var ordered []string
	for _, s := range strs {
		if _, ok := uniq[s]; ok {
			continue
		}
		if *filter != "" && !strings.Contains(s, *filter) {
			continue
		}
		uniq[s] = struct{}{}
		ordered = append(ordered, s)
	}
	sort.Strings(ordered)

	fmt.Printf(">>> %d unique printable strings (min len=%d, filter=%q):\n\n", len(ordered), *minLen, *filter)
	for _, s := range ordered {
		fmt.Println(s)
	}
}

// extractStrings returns all runs of printable ASCII bytes of length >= minLen.
func extractStrings(data []byte, minLen int) []string {
	var out []string
	cur := make([]byte, 0, 64)
	flush := func() {
		if len(cur) >= minLen {
			out = append(out, string(cur))
		}
		cur = cur[:0]
	}
	for _, b := range data {
		if b >= 0x20 && b < 0x7f {
			cur = append(cur, b)
		} else {
			flush()
		}
	}
	flush()
	return out
}

func fetchOAuthToken(tokenURL, clientID, clientSecret, audience string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"audience":      audience,
		"grant_type":    "client_credentials",
	})
	resp, err := http.Post(tokenURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token %d: %s", resp.StatusCode, string(respBody))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return "", err
	}
	return tr.AccessToken, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
