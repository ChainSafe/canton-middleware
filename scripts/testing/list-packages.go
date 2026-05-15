//go:build ignore

// list-packages.go — List all packages registered on the participant.

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
	"strings"
	"time"

	cfgpkg "github.com/chainsafe/canton-middleware/pkg/config"
	lapiv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Config")

func main() {
	flag.Parse()
	cfg, err := cfgpkg.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("config: %v", err)
	}
	ctx := context.Background()
	token, err := fetchOAuthToken(cfg.Canton.Ledger.Auth.TokenURL, cfg.Canton.Ledger.Auth.ClientID, cfg.Canton.Ledger.Auth.ClientSecret, cfg.Canton.Ledger.Auth.Audience)
	if err != nil {
		fatalf("oauth: %v", err)
	}
	target := cfg.Canton.Ledger.RPCURL
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}
	var dialOpts []grpc.DialOption
	if cfg.Canton.Ledger.TLS != nil && cfg.Canton.Ledger.TLS.Enabled {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(expcreds.NewTLSWithALPNDisabled(&tls.Config{MinVersion: tls.VersionTLS12})))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		fatalf("dial: %v", err)
	}
	defer conn.Close()
	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	c := lapiv2.NewPackageServiceClient(conn)
	listCtx, cancel := context.WithTimeout(authCtx, 30*time.Second)
	defer cancel()
	resp, err := c.ListPackages(listCtx, &lapiv2.ListPackagesRequest{})
	if err != nil {
		fatalf("list: %v", err)
	}
	fmt.Printf(">>> %d packages\n", len(resp.PackageIds))
	for _, p := range resp.PackageIds {
		fmt.Println(p)
	}
}

func fetchOAuthToken(tokenURL, clientID, clientSecret, audience string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id": clientID, "client_secret": clientSecret, "audience": audience, "grant_type": "client_credentials",
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
	json.Unmarshal(respBody, &tr)
	return tr.AccessToken, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
