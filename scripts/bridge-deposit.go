//go:build ignore

// Bridge deposit script - deposits PROMPT from Sepolia to Canton
package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	// ERC20 ABI for approve and balanceOf
	erc20ABI = `[{"constant":false,"inputs":[{"name":"spender","type":"address"},{"name":"amount","type":"uint256"}],"name":"approve","outputs":[{"name":"","type":"bool"}],"type":"function"},{"constant":true,"inputs":[{"name":"account","type":"address"}],"name":"balanceOf","outputs":[{"name":"","type":"uint256"}],"type":"function"},{"constant":true,"inputs":[{"name":"owner","type":"address"},{"name":"spender","type":"address"}],"name":"allowance","outputs":[{"name":"","type":"uint256"}],"type":"function"}]`

	// Bridge ABI for depositToCanton
	bridgeABI = `[{"inputs":[{"internalType":"address","name":"token","type":"address"},{"internalType":"uint256","name":"amount","type":"uint256"},{"internalType":"bytes32","name":"cantonRecipient","type":"bytes32"}],"name":"depositToCanton","outputs":[],"stateMutability":"nonpayable","type":"function"}]`
)

func main() {
	// Configuration - get RPC URL from environment
	rpcURL := os.Getenv("ETHEREUM_RPC_URL")
	if rpcURL == "" {
		log.Fatal("ETHEREUM_RPC_URL environment variable not set. Source secrets/devnet-secrets.sh first.")
	}
	promptToken := common.HexToAddress("0x90cb4f9eF6d682F4338f0E360B9C079fbb32048e")
	bridgeContract := common.HexToAddress("0x363Dd0b55bf74D5b494B064AA8E8c2Ef5eD58d75")

	// Get private key from environment variable
	privateKeyHex := os.Getenv("USER1_PRIVATE_KEY")
	if privateKeyHex == "" {
		log.Fatal("USER1_PRIVATE_KEY environment variable not set. Source secrets/devnet-secrets.sh first.")
	}

	// Amount to deposit (50 PROMPT = 50 * 10^18)
	amount := new(big.Int)
	amount.SetString("50000000000000000000", 10)

	fmt.Println("=== Canton Bridge Deposit ===")
	fmt.Printf("Token:  %s\n", promptToken.Hex())
	fmt.Printf("Bridge: %s\n", bridgeContract.Hex())
	fmt.Printf("Amount: 50 PROMPT\n\n")

	// Connect to Sepolia
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect to Sepolia: %v", err)
	}
	defer client.Close()

	chainID, err := client.ChainID(context.Background())
	if err != nil {
		log.Fatalf("Failed to get chain ID: %v", err)
	}
	fmt.Printf("Connected to chain ID: %d\n", chainID)

	// Parse private key
	privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKeyHex, "0x"))
	if err != nil {
		log.Fatalf("Failed to parse private key: %v", err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("Failed to get public key")
	}
	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	fmt.Printf("From address: %s\n\n", fromAddress.Hex())

	// Parse ABIs
	tokenABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		log.Fatalf("Failed to parse ERC20 ABI: %v", err)
	}

	bridgeABIParsed, err := abi.JSON(strings.NewReader(bridgeABI))
	if err != nil {
		log.Fatalf("Failed to parse bridge ABI: %v", err)
	}

	// Check current balance
	balanceData, err := tokenABI.Pack("balanceOf", fromAddress)
	if err != nil {
		log.Fatalf("Failed to pack balanceOf: %v", err)
	}

	result, err := client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &promptToken,
		Data: balanceData,
	}, nil)
	if err != nil {
		log.Fatalf("Failed to call balanceOf: %v", err)
	}

	balance := new(big.Int).SetBytes(result)
	balanceFloat := new(big.Float).SetInt(balance)
	balanceFloat.Quo(balanceFloat, big.NewFloat(1e18))
	fmt.Printf("Current PROMPT balance: %s\n", balanceFloat.Text('f', 2))

	// Check current allowance
	allowanceData, err := tokenABI.Pack("allowance", fromAddress, bridgeContract)
	if err != nil {
		log.Fatalf("Failed to pack allowance: %v", err)
	}

	result, err = client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &promptToken,
		Data: allowanceData,
	}, nil)
	if err != nil {
		log.Fatalf("Failed to call allowance: %v", err)
	}

	allowance := new(big.Int).SetBytes(result)
	allowanceFloat := new(big.Float).SetInt(allowance)
	allowanceFloat.Quo(allowanceFloat, big.NewFloat(1e18))
	fmt.Printf("Current allowance: %s\n\n", allowanceFloat.Text('f', 2))

	// Get nonce
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		log.Fatalf("Failed to get nonce: %v", err)
	}

	// Get gas price
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatalf("Failed to get gas price: %v", err)
	}

	// Create transactor
	auth, err := bind.NewKeyedTransactorWithChainID(privateKey, chainID)
	if err != nil {
		log.Fatalf("Failed to create transactor: %v", err)
	}
	auth.Nonce = big.NewInt(int64(nonce))
	auth.GasPrice = gasPrice
	auth.GasLimit = uint64(100000)

	// Step 1: Approve if needed
	if allowance.Cmp(amount) < 0 {
		fmt.Println("=== Step 1: Approving bridge contract ===")

		approveData, err := tokenABI.Pack("approve", bridgeContract, amount)
		if err != nil {
			log.Fatalf("Failed to pack approve: %v", err)
		}

		tx := types.NewTransaction(nonce, promptToken, big.NewInt(0), auth.GasLimit, gasPrice, approveData)
		signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
		if err != nil {
			log.Fatalf("Failed to sign approve tx: %v", err)
		}

		err = client.SendTransaction(context.Background(), signedTx)
		if err != nil {
			log.Fatalf("Failed to send approve tx: %v", err)
		}

		fmt.Printf("Approve tx sent: %s\n", signedTx.Hash().Hex())
		fmt.Println("Waiting for confirmation...")

		receipt, err := waitForReceipt(client, signedTx.Hash())
		if err != nil {
			log.Fatalf("Failed to get approve receipt: %v", err)
		}

		if receipt.Status == 1 {
			fmt.Println("Approve confirmed!")
		} else {
			log.Fatal("Approve transaction failed")
		}

		nonce++
		time.Sleep(2 * time.Second)
	} else {
		fmt.Println("Sufficient allowance already exists, skipping approve")
	}

	// Step 2: Deposit
	fmt.Println("\n=== Step 2: Depositing to bridge ===")

	// Compute Canton recipient fingerprint from EVM address (keccak256 of address bytes)
	cantonRecipient := crypto.Keccak256Hash(fromAddress.Bytes())
	fmt.Printf("Canton recipient (fingerprint): %s\n", cantonRecipient.Hex())

	depositData, err := bridgeABIParsed.Pack("depositToCanton", promptToken, amount, cantonRecipient)
	if err != nil {
		log.Fatalf("Failed to pack deposit: %v", err)
	}

	tx := types.NewTransaction(nonce, bridgeContract, big.NewInt(0), uint64(200000), gasPrice, depositData)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		log.Fatalf("Failed to sign deposit tx: %v", err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Fatalf("Failed to send deposit tx: %v", err)
	}

	fmt.Printf("Deposit tx sent: %s\n", signedTx.Hash().Hex())
	fmt.Println("Waiting for confirmation...")

	receipt, err := waitForReceipt(client, signedTx.Hash())
	if err != nil {
		log.Fatalf("Failed to get deposit receipt: %v", err)
	}

	if receipt.Status == 1 {
		fmt.Println("\n✓ Deposit confirmed!")
		fmt.Printf("Block: %d\n", receipt.BlockNumber.Uint64())
		fmt.Printf("Gas used: %d\n", receipt.GasUsed)
		fmt.Println("\nThe relayer will now detect this deposit and mint PROMPT on Canton.")
		fmt.Println("This may take a few minutes depending on confirmation requirements.")
	} else {
		log.Fatal("Deposit transaction failed")
	}
}

func waitForReceipt(client *ethclient.Client, txHash common.Hash) (*types.Receipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err == nil {
			return receipt, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
			fmt.Print(".")
		}
	}
}
