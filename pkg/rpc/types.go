package rpc

import (
	"encoding/json"
	"fmt"
)

// JSON-RPC 2.0 Types
// https://www.jsonrpc.org/specification

// Request represents a JSON-RPC 2.0 request
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

// Response represents a JSON-RPC 2.0 response
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// Error represents a JSON-RPC 2.0 error
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603

	// Custom error codes (application-specific)
	Unauthorized      = -32001
	NotFound          = -32002
	InsufficientFunds = -32003
	NotWhitelisted    = -32004
	AlreadyRegistered = -32005
)

// Error messages
var errorMessages = map[int]string{
	ParseError:        "Parse error",
	InvalidRequest:    "Invalid Request",
	MethodNotFound:    "Method not found",
	InvalidParams:     "Invalid params",
	InternalError:     "Internal error",
	Unauthorized:      "Unauthorized",
	NotFound:          "Not found",
	InsufficientFunds: "Insufficient funds",
	NotWhitelisted:    "Address not whitelisted",
	AlreadyRegistered: "User already registered",
}

// NewError creates a new JSON-RPC error
func NewError(code int, data interface{}) *Error {
	msg, ok := errorMessages[code]
	if !ok {
		msg = "Unknown error"
	}
	return &Error{
		Code:    code,
		Message: msg,
		Data:    data,
	}
}

// NewErrorWithMessage creates a new JSON-RPC error with a custom message
func NewErrorWithMessage(code int, message string, data interface{}) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// Validate validates the JSON-RPC request
func (r *Request) Validate() error {
	if r.JSONRPC != "2.0" {
		return fmt.Errorf("invalid jsonrpc version: expected 2.0")
	}
	if r.Method == "" {
		return fmt.Errorf("method is required")
	}
	return nil
}

// SuccessResponse creates a successful JSON-RPC response
func SuccessResponse(id interface{}, result interface{}) *Response {
	return &Response{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

// ErrorResponse creates an error JSON-RPC response
func ErrorResponse(id interface{}, err *Error) *Response {
	return &Response{
		JSONRPC: "2.0",
		Error:   err,
		ID:      id,
	}
}

// =============================================================================
// RPC Method Parameters
// =============================================================================

// BalanceOfParams represents parameters for erc20_balanceOf
type BalanceOfParams struct {
	Address string `json:"address,omitempty"` // Optional - uses authenticated user if not provided
}

// TransferParams represents parameters for erc20_transfer
type TransferParams struct {
	To     string `json:"to"`
	Amount string `json:"amount"`
}

// RegisterParams represents parameters for user_register
// No params needed - uses signature headers for authentication
type RegisterParams struct{}

// =============================================================================
// RPC Method Results
// =============================================================================

// TokenMetadata represents token metadata result
type TokenMetadata struct {
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

// BalanceResult represents balance query result
type BalanceResult struct {
	Balance string `json:"balance"`
	Address string `json:"address"`
}

// TransferResult represents transfer result
type TransferResult struct {
	Success bool   `json:"success"`
	TxID    string `json:"txId,omitempty"`
}

// RegisterResult represents user registration result
type RegisterResult struct {
	Party       string `json:"party"`
	Fingerprint string `json:"fingerprint"`
	MappingCID  string `json:"mappingCid,omitempty"`
}

// SupplyResult represents total supply result
type SupplyResult struct {
	TotalSupply string `json:"totalSupply"`
}

