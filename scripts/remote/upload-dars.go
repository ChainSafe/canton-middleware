//go:build ignore

// upload-dars.go - Upload DAR files to ChainSafe Canton (DevNet/Mainnet)
//
// This script uploads DAR files via the Package Management gRPC API.
//
// Usage:
//   go run scripts/remote/upload-dars.go -config config.api-server.devnet.yaml
//   go run scripts/remote/upload-dars.go -config config.api-server.mainnet.yaml
//
// The script will:
// 1. Connect to Canton with OAuth2 authentication
// 2. List currently known packages
// 3. Upload each DAR file
// 4. Output the new package IDs for config update

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	admin "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2/admin"
)

var (
	tokenMu     sync.Mutex
	cachedToken string
	tokenExpiry time.Time
)

// DAR files to upload in dependency order
var darFiles = []string{
	"common",
	"cip56-token",
	"native-token",
	"bridge-core",
	"bridge-wayfinder",
}

func main() {
	configPath := flag.String("config", "config.api-server.devnet.yaml", "Path to config file")
	darDir := flag.String("dar-dir", "", "Directory containing DAR files (default: contracts/canton-erc20/daml/*/dist/)")
	listOnly := flag.Bool("list-only", false, "Only list known packages, don't upload")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Connect to Canton
	var opts []grpc.DialOption
	if cfg.Canton.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2"},
		}
		creds := credentials.NewTLS(tlsConfig)
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	conn, err := grpc.NewClient(cfg.Canton.RPCURL, opts...)
	if err != nil {
		log.Fatalf("Failed to connect to Canton: %v", err)
	}
	defer conn.Close()

	// Get auth context
	ctx, err = getAuthContext(ctx, &cfg.Canton.Auth)
	if err != nil {
		log.Fatalf("Failed to get auth context: %v", err)
	}

	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("DAR UPLOAD TOOL")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Printf("Canton RPC: %s\n", cfg.Canton.RPCURL)
	fmt.Println()

	pkgClient := admin.NewPackageManagementServiceClient(conn)

	// List known packages
	fmt.Println(">>> Listing known packages...")
	listResp, err := pkgClient.ListKnownPackages(ctx, &admin.ListKnownPackagesRequest{})
	if err != nil {
		log.Fatalf("Failed to list packages: %v", err)
	}

	// Group by name
	pkgByName := make(map[string][]*admin.PackageDetails)
	for _, pkg := range listResp.PackageDetails {
		pkgByName[pkg.Name] = append(pkgByName[pkg.Name], pkg)
	}

	// Sort names
	var names []string
	for name := range pkgByName {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("    Found %d packages:\n", len(listResp.PackageDetails))
	for _, name := range names {
		pkgs := pkgByName[name]
		if len(pkgs) == 1 {
			fmt.Printf("    - %s v%s (%s...)\n", name, pkgs[0].Version, pkgs[0].PackageId[:16])
		} else {
			fmt.Printf("    - %s:\n", name)
			for _, pkg := range pkgs {
				fmt.Printf("        v%s (%s...)\n", pkg.Version, pkg.PackageId[:16])
			}
		}
	}
	fmt.Println()

	if *listOnly {
		fmt.Println("List-only mode, exiting.")
		return
	}

	// Find DAR files
	var darPaths []string
	if *darDir != "" {
		// Use specified directory
		for _, name := range darFiles {
			pattern := filepath.Join(*darDir, name+"*.dar")
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				darPaths = append(darPaths, matches[0])
			}
		}
	} else {
		// Use default locations
		for _, name := range darFiles {
			pattern := filepath.Join("contracts/canton-erc20/daml", name, ".daml/dist/*.dar")
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				darPaths = append(darPaths, matches[0])
			}
		}
	}

	if len(darPaths) == 0 {
		log.Fatal("No DAR files found. Run './scripts/setup/build-dars.sh' first.")
	}

	fmt.Printf(">>> Found %d DAR files to upload:\n", len(darPaths))
	for _, p := range darPaths {
		fmt.Printf("    - %s\n", filepath.Base(p))
	}
	fmt.Println()

	// Upload each DAR
	fmt.Println(">>> Uploading DARs...")
	newPackages := make(map[string]string)

	for _, darPath := range darPaths {
		darName := filepath.Base(darPath)
		fmt.Printf("    Uploading %s...\n", darName)

		darBytes, err := os.ReadFile(darPath)
		if err != nil {
			log.Fatalf("Failed to read %s: %v", darPath, err)
		}

		uploadReq := &admin.UploadDarFileRequest{
			DarFile:       darBytes,
			SubmissionId:  fmt.Sprintf("upload-%s-%d", darName, time.Now().UnixNano()),
			VettingChange: admin.UploadDarFileRequest_VETTING_CHANGE_DONT_VET_ANY_PACKAGES,
		}

		_, err = pkgClient.UploadDarFile(ctx, uploadReq)
		if err != nil {
			// Check if it's a duplicate package error
			if strings.Contains(err.Error(), "DUPLICATE") || strings.Contains(err.Error(), "already exists") {
				fmt.Printf("    [SKIP] %s already uploaded\n", darName)
				continue
			}
			log.Fatalf("Failed to upload %s: %v", darPath, err)
		}
		fmt.Printf("    [OK] %s uploaded\n", darName)
	}
	fmt.Println()

	// List packages again to get new IDs
	fmt.Println(">>> Fetching updated package list...")
	listResp, err = pkgClient.ListKnownPackages(ctx, &admin.ListKnownPackagesRequest{})
	if err != nil {
		log.Fatalf("Failed to list packages: %v", err)
	}

	// Find the target version (1.3.0) of each package we care about
	targetVersion := "1.3.0"
	targetPackages := map[string]string{
		"common":           "COMMON_PACKAGE_ID",
		"cip56-token":      "CIP56_PACKAGE_ID",
		"native-token":     "NATIVE_TOKEN_PACKAGE_ID",
		"bridge-core":      "BRIDGE_CORE_PACKAGE_ID",
		"bridge-wayfinder": "BRIDGE_WAYFINDER_PACKAGE_ID",
	}

	// Track version for comparison
	packageVersions := make(map[string]string)

	for _, pkg := range listResp.PackageDetails {
		for darName, envName := range targetPackages {
			if pkg.Name == darName && pkg.Version == targetVersion {
				newPackages[envName] = pkg.PackageId
				packageVersions[envName] = pkg.Version
			}
		}
	}

	// Now vet the v1.3.0 packages with force flag
	fmt.Println(">>> Vetting new packages with force flag...")
	var packagesToVet []*admin.VettedPackagesRef
	for envName, pkgId := range newPackages {
		if packageVersions[envName] == "1.3.0" {
			packagesToVet = append(packagesToVet, &admin.VettedPackagesRef{
				PackageId: pkgId,
			})
		}
	}

	if len(packagesToVet) > 0 {
		vetReq := &admin.UpdateVettedPackagesRequest{
			Changes: []*admin.VettedPackagesChange{
				{
					Operation: &admin.VettedPackagesChange_Vet_{
						Vet: &admin.VettedPackagesChange_Vet{
							Packages: packagesToVet,
						},
					},
				},
			},
			UpdateVettedPackagesForceFlags: []admin.UpdateVettedPackagesForceFlag{
				admin.UpdateVettedPackagesForceFlag_UPDATE_VETTED_PACKAGES_FORCE_FLAG_ALLOW_VET_INCOMPATIBLE_UPGRADES,
			},
		}

		_, err = pkgClient.UpdateVettedPackages(ctx, vetReq)
		if err != nil {
			fmt.Printf("    [WARN] Vetting failed: %v\n", err)
			fmt.Println("    Packages uploaded but not vetted - may need manual vetting")
		} else {
			fmt.Printf("    [OK] Vetted %d packages\n", len(packagesToVet))
		}
	}
	fmt.Println()

	fmt.Println()
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println("UPLOAD COMPLETE - Update your config with these package IDs:")
	fmt.Println("=" + strings.Repeat("=", 69))
	fmt.Println()
	fmt.Println("# For config.api-server.devnet.yaml / config.api-server.mainnet.yaml:")
	fmt.Println("canton:")

	// Print in a specific order
	order := []string{"CIP56_PACKAGE_ID", "NATIVE_TOKEN_PACKAGE_ID", "COMMON_PACKAGE_ID", "BRIDGE_CORE_PACKAGE_ID", "BRIDGE_WAYFINDER_PACKAGE_ID"}
	configKeys := map[string]string{
		"CIP56_PACKAGE_ID":            "cip56_package_id",
		"NATIVE_TOKEN_PACKAGE_ID":     "native_token_package_id",
		"COMMON_PACKAGE_ID":           "common_package_id",
		"BRIDGE_CORE_PACKAGE_ID":      "core_package_id",
		"BRIDGE_WAYFINDER_PACKAGE_ID": "bridge_package_id",
	}

	for _, envName := range order {
		if id, ok := newPackages[envName]; ok {
			fmt.Printf("  %s: \"%s\"\n", configKeys[envName], id)
		}
	}

	fmt.Println()
	fmt.Println("# For .env file:")
	for _, envName := range order {
		if id, ok := newPackages[envName]; ok {
			fmt.Printf("%s=%s\n", envName, id)
		}
	}
}

func getAuthContext(ctx context.Context, auth *config.AuthConfig) (context.Context, error) {
	if auth.ClientID == "" || auth.ClientSecret == "" || auth.Audience == "" || auth.TokenURL == "" {
		return ctx, nil
	}

	token, err := getOAuthToken(auth)
	if err != nil {
		return nil, err
	}

	md := metadata.Pairs("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func getOAuthToken(auth *config.AuthConfig) (string, error) {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	now := time.Now()
	if cachedToken != "" && now.Before(tokenExpiry) {
		return cachedToken, nil
	}

	payload := map[string]string{
		"client_id":     auth.ClientID,
		"client_secret": auth.ClientSecret,
		"audience":      auth.Audience,
		"grant_type":    "client_credentials",
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OAuth token request: %w", err)
	}

	req, err := http.NewRequest("POST", auth.TokenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create OAuth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call OAuth token endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("OAuth token endpoint returned %d: %s", resp.StatusCode, string(b))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode OAuth token response: %w", err)
	}

	expiry := now.Add(5 * time.Minute)
	if tokenResp.ExpiresIn > 0 {
		leeway := 60
		if tokenResp.ExpiresIn <= leeway {
			leeway = tokenResp.ExpiresIn / 2
		}
		expiry = now.Add(time.Duration(tokenResp.ExpiresIn-leeway) * time.Second)
	}

	cachedToken = tokenResp.AccessToken
	tokenExpiry = expiry

	return tokenResp.AccessToken, nil
}
