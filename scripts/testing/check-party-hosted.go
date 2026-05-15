//go:build ignore

// check-party-hosted.go — Verify if a Canton party is hosted on our participant.
//
// Calls PartyManagementService.GetParties and reports IsLocal.
//
// Usage:
//   go run scripts/testing/check-party-hosted.go \
//     -config config.api-server.devnet-test.yaml \
//     -party "user_f39Fd6e5::1220d7dca32461837f5507effa024b31e5cd2119c23e7581f465c55fb7257761beb5"

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
	adminv2 "github.com/chainsafe/canton-middleware/pkg/cantonsdk/lapi/v2/admin"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	expcreds "google.golang.org/grpc/experimental/credentials"
	"google.golang.org/grpc/metadata"
)

var (
	configPath = flag.String("config", "config.api-server.devnet-test.yaml", "Path to API server config file")
	partyID    = flag.String("party", "", "Canton party ID to check (required)")
)

func main() {
	flag.Parse()
	if *partyID == "" {
		fatalf("-party flag is required")
	}

	cfg, err := cfgpkg.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("Failed to load config: %v", err)
	}

	ctx := context.Background()

	// Fetch OAuth token (client credentials flow)
	token, err := fetchOAuthToken(
		cfg.Canton.Ledger.Auth.TokenURL,
		cfg.Canton.Ledger.Auth.ClientID,
		cfg.Canton.Ledger.Auth.ClientSecret,
		cfg.Canton.Ledger.Auth.Audience,
	)
	if err != nil {
		fatalf("OAuth token: %v", err)
	}

	// Connect to Canton ledger gRPC
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
		fatalf("dial Canton: %v", err)
	}
	defer conn.Close()

	authCtx := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)

	// Get participant ID
	pmClient := adminv2.NewPartyManagementServiceClient(conn)
	pidCtx, cancelPID := context.WithTimeout(authCtx, 30*time.Second)
	defer cancelPID()
	pidResp, err := pmClient.GetParticipantId(pidCtx, &adminv2.GetParticipantIdRequest{})
	if err != nil {
		fatalf("GetParticipantId: %v", err)
	}

	// Query the party
	getCtx, cancelGet := context.WithTimeout(authCtx, 30*time.Second)
	defer cancelGet()
	resp, err := pmClient.GetParties(getCtx, &adminv2.GetPartiesRequest{
		Parties: []string{*partyID},
	})
	if err != nil {
		fatalf("GetParties: %v", err)
	}

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  Party Hosting Check")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Printf("  Participant ID: %s\n", pidResp.ParticipantId)
	fmt.Printf("  Query party:    %s\n", *partyID)
	fmt.Println()

	if len(resp.PartyDetails) == 0 {
		fmt.Println(">>> Party NOT FOUND on this participant")
		fmt.Println("    The participant has no record of this party — it is not hosted here.")
		os.Exit(0)
	}

	for _, pd := range resp.PartyDetails {
		fmt.Printf("  Party:    %s\n", pd.Party)
		fmt.Printf("  IsLocal:  %v   (true = hosted on our participant)\n", pd.IsLocal)
		if pd.IsLocal {
			fmt.Println("\n>>> CONFIRMED: This party is hosted by our participant.")
		} else {
			fmt.Println("\n>>> NOT HOSTED: Party exists on the network but our participant does not host it.")
		}
	}
}

func fetchOAuthToken(tokenURL, clientID, clientSecret, audience string) (string, error) {
	payload := map[string]string{
		"client_id":     clientID,
		"client_secret": clientSecret,
		"audience":      audience,
		"grant_type":    "client_credentials",
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(tokenURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request %d: %s", resp.StatusCode, string(respBody))
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
