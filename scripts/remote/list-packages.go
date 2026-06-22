//go:build ignore

// list-packages.go — List every DAR (package) uploaded on the connected Canton
// participant, with optional name-substring filtering. Use it to verify the
// Wayfinder bridge / cip56 / utility-registry DARs are present (and at which
// version hash) across environments.
//
// Uses admin.PackageManagementService.ListKnownPackages, which returns
// {package_id, name, version} for every uploaded package. Requires the OAuth
// user to have participant_admin (both devnet and prod1 OAuth users have it).
//
// Usage:
//
//	# All packages on devnet:
//	go run scripts/remote/list-packages.go -config config.api-server.devnet-test.yaml
//
//	# Just the wayfinder/bridge/cip56 ones (substring match, case-insensitive):
//	go run scripts/remote/list-packages.go \
//	  -config config.api-server.devnet-test.yaml \
//	  -filter "bridge,cip56,common,utility-registry,splice"

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	cantonclient "github.com/chainsafe/canton-middleware/pkg/cantonsdk/client"
	adminv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/admin"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	configPath = flag.String("config", "", "Path to API server config file (required)")
	filterCSV  = flag.String("filter", "", "Comma-separated case-insensitive substrings to match against the package name (default: no filter — list all)")
)

func main() {
	flag.Parse()
	if *configPath == "" {
		fmt.Println("ERROR: -config is required")
		os.Exit(1)
	}
	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}

	var filters []string
	if *filterCSV != "" {
		for _, s := range strings.Split(*filterCSV, ",") {
			if t := strings.TrimSpace(strings.ToLower(s)); t != "" {
				filters = append(filters, t)
			}
		}
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  List known DARs (packages) on the connected participant")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Printf("  Canton:  %s\n", cfg.Canton.Ledger.RPCURL)
	if len(filters) > 0 {
		fmt.Printf("  Filter:  name contains any of %v (case-insensitive)\n", filters)
	} else {
		fmt.Printf("  Filter:  (none — listing all)\n")
	}
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	conn, err := dialCanton(cfg)
	if err != nil {
		fatalf("dial Canton: %v", err)
	}
	defer conn.Close()

	ctx, _, err = authContext(ctx, cfg.Canton)
	if err != nil {
		fatalf("auth: %v", err)
	}

	client := adminv2.NewPackageManagementServiceClient(conn)
	resp, err := client.ListKnownPackages(ctx, &adminv2.ListKnownPackagesRequest{})
	if err != nil {
		fatalf("ListKnownPackages: %v (need participant_admin rights)", err)
	}

	type row struct{ name, version, id string }
	var rows []row
	for _, p := range resp.PackageDetails {
		if len(filters) > 0 && !nameMatchesAny(p.Name, filters) {
			continue
		}
		rows = append(rows, row{name: p.Name, version: p.Version, id: p.PackageId})
	}

	// Group by name, sort by version within each name, then by name across.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].name != rows[j].name {
			return rows[i].name < rows[j].name
		}
		return rows[i].version < rows[j].version
	})

	if len(rows) == 0 {
		fmt.Println(">>> 0 matching packages.")
		return
	}

	maxName, maxVer := 4, 7
	for _, r := range rows {
		if len(r.name) > maxName {
			maxName = len(r.name)
		}
		if len(r.version) > maxVer {
			maxVer = len(r.version)
		}
	}
	fmt.Printf("  %-*s  %-*s  package_id\n", maxName, "name", maxVer, "version")
	fmt.Printf("  %s  %s  %s\n", strings.Repeat("─", maxName), strings.Repeat("─", maxVer), strings.Repeat("─", 64))
	prev := ""
	for _, r := range rows {
		if prev != "" && prev != r.name {
			fmt.Println()
		}
		fmt.Printf("  %-*s  %-*s  %s\n", maxName, r.name, maxVer, r.version, r.id)
		prev = r.name
	}
	fmt.Println()
	fmt.Printf(">>> %d matching package(s).\n", len(rows))
}

func nameMatchesAny(name string, needles []string) bool {
	lower := strings.ToLower(name)
	for _, n := range needles {
		if strings.Contains(lower, n) {
			return true
		}
	}
	return false
}

func dialCanton(cfg *config.APIServer) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption
	if cfg.Canton.Ledger.TLS != nil && cfg.Canton.Ledger.TLS.Enabled {
		opts = append(opts, grpc.WithTransportCredentials(expcreds.NewTLSWithALPNDisabled(&tls.Config{InsecureSkipVerify: true}))) //nolint:gosec
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if cfg.Canton.Ledger.MaxMessageSize > 0 {
		opts = append(opts, grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(cfg.Canton.Ledger.MaxMessageSize)))
	}
	target := cfg.Canton.Ledger.RPCURL
	if !strings.Contains(target, "://") {
		target = "dns:///" + target
	}
	return grpc.NewClient(target, opts...)
}

func authContext(ctx context.Context, canton *cantonclient.Config) (context.Context, string, error) {
	if canton.Ledger.Auth == nil || canton.Ledger.Auth.ClientID == "" {
		return ctx, "", nil
	}
	body, _ := json.Marshal(map[string]string{
		"client_id":     canton.Ledger.Auth.ClientID,
		"client_secret": canton.Ledger.Auth.ClientSecret,
		"audience":      canton.Ledger.Auth.Audience,
		"grant_type":    "client_credentials",
	})
	resp, err := http.Post(canton.Ledger.Auth.TokenURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("token endpoint %d: %s", resp.StatusCode, rb)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(rb, &tok); err != nil {
		return nil, "", err
	}
	var sub string
	if parts := strings.Split(tok.AccessToken, "."); len(parts) >= 2 {
		p := parts[1] + strings.Repeat("=", (-len(parts[1]))&3)
		if dec, err := base64.URLEncoding.DecodeString(p); err == nil {
			var c struct {
				Sub string `json:"sub"`
			}
			_ = json.Unmarshal(dec, &c)
			sub = c.Sub
		}
	}
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+tok.AccessToken)), sub, nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
