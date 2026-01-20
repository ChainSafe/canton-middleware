package main

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/chainsafe/canton-middleware/pkg/auth"
	"github.com/ethereum/go-ethereum/common"
	_ "github.com/lib/pq"
)

func main() {
	// Test addresses from config.e2e-local.yaml
	user1Addr := common.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
	user2Addr := common.HexToAddress("0x70997970C51812dc3A010C7d01b50e0d17dc79C8")

	fmt.Println("Testing whitelist address normalization")
	fmt.Println("========================================")

	// Show what gets stored
	fmt.Printf("User1 original: %s\n", "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266")
	fmt.Printf("User1 Hex():    %s\n", user1Addr.Hex())
	fmt.Printf("User1 Normalize: %s\n", auth.NormalizeAddress(user1Addr.Hex()))
	fmt.Println()

	fmt.Printf("User2 original: %s\n", "0x70997970C51812dc3A010C7d01b50e0d17dc79C8")
	fmt.Printf("User2 Hex():    %s\n", user2Addr.Hex())
	fmt.Printf("User2 Normalize: %s\n", auth.NormalizeAddress(user2Addr.Hex()))
	fmt.Println()

	// Connect to database
	dsn := "host=localhost port=5432 user=postgres password=p@ssw0rd dbname=erc20_api sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	fmt.Println("Checking database entries:")
	fmt.Println("==========================")

	// Query whitelist table
	rows, err := db.Query("SELECT evm_address FROM whitelist")
	if err != nil {
		log.Fatalf("Failed to query whitelist: %v", err)
	}
	defer rows.Close()

	var count int
	for rows.Next() {
		var addr string
		if err := rows.Scan(&addr); err != nil {
			log.Fatalf("Failed to scan row: %v", err)
		}
		count++
		normalized := auth.NormalizeAddress(addr)
		match1 := normalized == auth.NormalizeAddress(user1Addr.Hex())
		match2 := normalized == auth.NormalizeAddress(user2Addr.Hex())
		fmt.Printf("%d. DB: %s -> Normalized: %s (User1: %v, User2: %v)\n", count, addr, normalized, match1, match2)
	}

	if count == 0 {
		fmt.Println("⚠️  WARNING: No addresses in whitelist!")
	} else {
		fmt.Printf("\nTotal whitelist entries: %d\n", count)
	}

	// Test IsWhitelisted logic
	fmt.Println("\nTesting whitelist check:")
	fmt.Println("========================")

	for _, addr := range []common.Address{user1Addr, user2Addr} {
		normalized := auth.NormalizeAddress(addr.Hex())
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM whitelist WHERE evm_address = $1)", normalized).Scan(&exists)
		if err != nil {
			log.Printf("Error checking %s: %v", normalized, err)
		} else {
			status := "❌ NOT WHITELISTED"
			if exists {
				status = "✅ WHITELISTED"
			}
			fmt.Printf("%s: %s\n", normalized, status)
		}
	}
}
