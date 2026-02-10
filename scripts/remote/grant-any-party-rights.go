//go:build ignore
// +build ignore

// Grant CanExecuteAsAnyParty and CanReadAsAnyParty Rights
//
// This script grants the authenticated OAuth user the rights to act as
// and read as any party on the participant. This is required for custodial
// wallet operations where the API server submits commands on behalf of users.
//
// Usage:
//   go run scripts/remote/grant-any-party-rights.go -config config.api-server.devnet.local.yaml
//
// Prerequisites:
//   - Valid OAuth credentials in config
//   - User must already exist (created on first API call)

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	adminv2 "github.com/chainsafe/canton-middleware/pkg/canton/lapi/v2/admin"
	"github.com/chainsafe/canton-middleware/pkg/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var (
	configPath = flag.String("config", "config.api-server.devnet.local.yaml", "Path to config file")
	userID     = flag.String("user", "", "User ID to grant rights to (default: OAuth client sub)")
	listOnly   = flag.Bool("list", false, "Only list current rights, don't grant new ones")
)

func main() {
	flag.Parse()

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Grant CanExecuteAsAnyParty Rights")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Load config
	cfg, err := config.LoadAPIServer(*configPath)
	if err != nil {
		fatalf("Failed to load config: %v", err)
	}

	fmt.Printf(">>> Canton endpoint: %s\n", cfg.Canton.RPCURL)
	fmt.Println()

	// Get OAuth token
	fmt.Println(">>> Fetching OAuth token...")
	token, sub, err := getOAuthToken(&cfg.Canton)
	if err != nil {
		fatalf("Failed to get OAuth token: %v", err)
	}
	fmt.Printf("    Token obtained for: %s\n", sub)
	fmt.Println()

	// Determine user ID
	targetUserID := *userID
	if targetUserID == "" {
		targetUserID = sub
	}
	fmt.Printf(">>> Target user ID: %s\n", targetUserID)
	fmt.Println()

	// Connect to Canton
	fmt.Println(">>> Connecting to Canton...")
	conn, err := grpc.NewClient(cfg.Canton.RPCURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()
	fmt.Println("    Connected!")
	fmt.Println()

	// Create client
	client := adminv2.NewUserManagementServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Add auth header
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))

	// List current rights
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Current User Rights")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	listResp, err := client.ListUserRights(ctx, &adminv2.ListUserRightsRequest{
		UserId: targetUserID,
	})
	if err != nil {
		fmt.Printf("    WARNING: Failed to list rights: %v\n", err)
		fmt.Println("    User may not exist yet. Will try to create...")
		fmt.Println()

		// Try to create the user first
		_, err = client.CreateUser(ctx, &adminv2.CreateUserRequest{
			User: &adminv2.User{
				Id: targetUserID,
			},
		})
		if err != nil {
			fmt.Printf("    Note: Create user result: %v\n", err)
		} else {
			fmt.Println("    User created!")
		}
		fmt.Println()
	} else {
		printRights(listResp.Rights)
	}

	if *listOnly {
		fmt.Println(">>> List-only mode, not granting new rights.")
		return
	}

	// Check if already has the rights we need
	hasExecuteAny := false
	hasReadAny := false
	if listResp != nil {
		for _, r := range listResp.Rights {
			if r.GetCanExecuteAsAnyParty() != nil {
				hasExecuteAny = true
			}
			if r.GetCanReadAsAnyParty() != nil {
				hasReadAny = true
			}
		}
	}

	if hasExecuteAny && hasReadAny {
		fmt.Println()
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println("  Already Has Required Rights!")
		fmt.Println("══════════════════════════════════════════════════════════════════════")
		fmt.Println()
		fmt.Println("User already has CanExecuteAsAnyParty and CanReadAsAnyParty rights.")
		fmt.Println("No action needed.")
		return
	}

	// Grant the rights
	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Granting Rights")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	var rightsToGrant []*adminv2.Right

	if !hasExecuteAny {
		fmt.Println("    + CanExecuteAsAnyParty")
		rightsToGrant = append(rightsToGrant, &adminv2.Right{
			Kind: &adminv2.Right_CanExecuteAsAnyParty_{
				CanExecuteAsAnyParty: &adminv2.Right_CanExecuteAsAnyParty{},
			},
		})
	}

	if !hasReadAny {
		fmt.Println("    + CanReadAsAnyParty")
		rightsToGrant = append(rightsToGrant, &adminv2.Right{
			Kind: &adminv2.Right_CanReadAsAnyParty_{
				CanReadAsAnyParty: &adminv2.Right_CanReadAsAnyParty{},
			},
		})
	}

	fmt.Println()
	fmt.Println(">>> Calling GrantUserRights...")

	grantResp, err := client.GrantUserRights(ctx, &adminv2.GrantUserRightsRequest{
		UserId: targetUserID,
		Rights: rightsToGrant,
	})
	if err != nil {
		fatalf("Failed to grant rights: %v", err)
	}

	fmt.Printf("    Granted %d new right(s)\n", len(grantResp.NewlyGrantedRights))
	fmt.Println()

	// Verify by listing again
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Updated User Rights")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	listResp, err = client.ListUserRights(ctx, &adminv2.ListUserRightsRequest{
		UserId: targetUserID,
	})
	if err != nil {
		fmt.Printf("    WARNING: Failed to verify: %v\n", err)
	} else {
		printRights(listResp.Rights)
	}

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Done!")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Your OAuth user can now ActAs and ReadAs any party on this participant.")
	fmt.Println("Try running the interop demo again:")
	fmt.Println()
	fmt.Println("  go run scripts/testing/interop-demo.go -config config.api-server.devnet.local.yaml")
	fmt.Println()
}

func getOAuthToken(cfg *config.CantonConfig) (token string, subject string, err error) {
	payload := map[string]string{
		"client_id":     cfg.Auth.ClientID,
		"client_secret": cfg.Auth.ClientSecret,
		"audience":      cfg.Auth.Audience,
		"grant_type":    "client_credentials",
	}
	bodyBytes, _ := json.Marshal(payload)

	resp, err := http.Post(cfg.Auth.TokenURL, "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("token request failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", "", fmt.Errorf("failed to parse token response: %w", err)
	}

	// Decode JWT to get subject
	parts := strings.Split(tokenResp.AccessToken, ".")
	if len(parts) >= 2 {
		// JWT uses URL-safe base64 without padding
		payloadStr := parts[1]
		// Add padding if needed
		switch len(payloadStr) % 4 {
		case 2:
			payloadStr += "=="
		case 3:
			payloadStr += "="
		}
		decoded, err := base64.URLEncoding.DecodeString(payloadStr)
		if err != nil {
			// Try standard encoding with URL-safe chars replaced
			payloadStr = strings.ReplaceAll(parts[1], "-", "+")
			payloadStr = strings.ReplaceAll(payloadStr, "_", "/")
			switch len(payloadStr) % 4 {
			case 2:
				payloadStr += "=="
			case 3:
				payloadStr += "="
			}
			decoded, _ = base64.StdEncoding.DecodeString(payloadStr)
		}
		var claims struct {
			Sub string `json:"sub"`
		}
		json.Unmarshal(decoded, &claims)
		subject = claims.Sub
	}

	return tokenResp.AccessToken, subject, nil
}

func printRights(rights []*adminv2.Right) {
	if len(rights) == 0 {
		fmt.Println("    (no rights)")
		return
	}

	for _, r := range rights {
		switch r.Kind.(type) {
		case *adminv2.Right_ParticipantAdmin_:
			fmt.Println("    - ParticipantAdmin")
		case *adminv2.Right_CanActAs_:
			fmt.Printf("    - CanActAs: %s\n", r.GetCanActAs().Party)
		case *adminv2.Right_CanReadAs_:
			fmt.Printf("    - CanReadAs: %s\n", r.GetCanReadAs().Party)
		case *adminv2.Right_CanExecuteAs_:
			fmt.Printf("    - CanExecuteAs: %s\n", r.GetCanExecuteAs().Party)
		case *adminv2.Right_CanExecuteAsAnyParty_:
			fmt.Println("    - CanExecuteAsAnyParty ✓")
		case *adminv2.Right_CanReadAsAnyParty_:
			fmt.Println("    - CanReadAsAnyParty ✓")
		case *adminv2.Right_IdentityProviderAdmin_:
			fmt.Println("    - IdentityProviderAdmin")
		default:
			fmt.Printf("    - Unknown right: %T\n", r.Kind)
		}
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Printf("ERROR: "+format+"\n", args...)
	os.Exit(1)
}
