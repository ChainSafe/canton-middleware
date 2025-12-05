// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package contracts

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// PromptTokenMetaData contains all meta data concerning the PromptToken contract.
var PromptTokenMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"DEFAULT_ADMIN_ROLE\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"INVOKE_WAYFINDER_CONFIGURATION_ROLE\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"addWayfinderHandlerContract\",\"inputs\":[{\"name\":\"_contractAddress\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"_nativeTokenDestinationAddress\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"_promptDestinationAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"allowance\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"approve\",\"inputs\":[{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"balanceOf\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"burn\",\"inputs\":[{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"burnFrom\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"decimals\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"internalType\":\"uint8\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getRoleAdmin\",\"inputs\":[{\"name\":\"role\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"grantRole\",\"inputs\":[{\"name\":\"role\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"hasRole\",\"inputs\":[{\"name\":\"role\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"invokeWayfinder\",\"inputs\":[{\"name\":\"_handlerAddress\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"_id\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"_promptValue\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"_data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"name\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"renounceRole\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"revokeRole\",\"inputs\":[{\"name\":\"role\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"supportsInterface\",\"inputs\":[{\"name\":\"interfaceId\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"symbol\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"totalSupply\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"transfer\",\"inputs\":[{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferFrom\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"wayfinderGateways\",\"inputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"nativeTokenDestinationAddress\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"promptDestinationAddress\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"invokeWayfinderHandler\",\"type\":\"address\",\"internalType\":\"contractInvokeWayfinderHandler\"}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"Approval\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"spender\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RoleAdminChanged\",\"inputs\":[{\"name\":\"role\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"previousAdminRole\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"newAdminRole\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RoleGranted\",\"inputs\":[{\"name\":\"role\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"account\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"sender\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RoleRevoked\",\"inputs\":[{\"name\":\"role\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"account\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"sender\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Transfer\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"to\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"WayfinderGatewayRegistered\",\"inputs\":[{\"name\":\"contractAddress\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"nativeTokenDestinationAddress\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"promptDestinationAddress\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"WayfinderInvoked\",\"inputs\":[{\"name\":\"from\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"nativeTokenDestination\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"promptDestination\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"id\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"nativeTokenValue\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"promptValue\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"data\",\"type\":\"bytes\",\"indexed\":false,\"internalType\":\"bytes\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AccessControlBadConfirmation\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"AccessControlUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"neededRole\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"ERC20InsufficientAllowance\",\"inputs\":[{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"allowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"needed\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ERC20InsufficientBalance\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"balance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"needed\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ERC20InvalidApprover\",\"inputs\":[{\"name\":\"approver\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC20InvalidReceiver\",\"inputs\":[{\"name\":\"receiver\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC20InvalidSender\",\"inputs\":[{\"name\":\"sender\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC20InvalidSpender\",\"inputs\":[{\"name\":\"spender\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]}]",
}

// PromptTokenABI is the input ABI used to generate the binding from.
// Deprecated: Use PromptTokenMetaData.ABI instead.
var PromptTokenABI = PromptTokenMetaData.ABI

// PromptToken is an auto generated Go binding around an Ethereum contract.
type PromptToken struct {
	PromptTokenCaller     // Read-only binding to the contract
	PromptTokenTransactor // Write-only binding to the contract
	PromptTokenFilterer   // Log filterer for contract events
}

// PromptTokenCaller is an auto generated read-only Go binding around an Ethereum contract.
type PromptTokenCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// PromptTokenTransactor is an auto generated write-only Go binding around an Ethereum contract.
type PromptTokenTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// PromptTokenFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type PromptTokenFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// PromptTokenSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type PromptTokenSession struct {
	Contract     *PromptToken      // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// PromptTokenCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type PromptTokenCallerSession struct {
	Contract *PromptTokenCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts      // Call options to use throughout this session
}

// PromptTokenTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type PromptTokenTransactorSession struct {
	Contract     *PromptTokenTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts      // Transaction auth options to use throughout this session
}

// PromptTokenRaw is an auto generated low-level Go binding around an Ethereum contract.
type PromptTokenRaw struct {
	Contract *PromptToken // Generic contract binding to access the raw methods on
}

// PromptTokenCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type PromptTokenCallerRaw struct {
	Contract *PromptTokenCaller // Generic read-only contract binding to access the raw methods on
}

// PromptTokenTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type PromptTokenTransactorRaw struct {
	Contract *PromptTokenTransactor // Generic write-only contract binding to access the raw methods on
}

// NewPromptToken creates a new instance of PromptToken, bound to a specific deployed contract.
func NewPromptToken(address common.Address, backend bind.ContractBackend) (*PromptToken, error) {
	contract, err := bindPromptToken(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &PromptToken{PromptTokenCaller: PromptTokenCaller{contract: contract}, PromptTokenTransactor: PromptTokenTransactor{contract: contract}, PromptTokenFilterer: PromptTokenFilterer{contract: contract}}, nil
}

// NewPromptTokenCaller creates a new read-only instance of PromptToken, bound to a specific deployed contract.
func NewPromptTokenCaller(address common.Address, caller bind.ContractCaller) (*PromptTokenCaller, error) {
	contract, err := bindPromptToken(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &PromptTokenCaller{contract: contract}, nil
}

// NewPromptTokenTransactor creates a new write-only instance of PromptToken, bound to a specific deployed contract.
func NewPromptTokenTransactor(address common.Address, transactor bind.ContractTransactor) (*PromptTokenTransactor, error) {
	contract, err := bindPromptToken(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &PromptTokenTransactor{contract: contract}, nil
}

// NewPromptTokenFilterer creates a new log filterer instance of PromptToken, bound to a specific deployed contract.
func NewPromptTokenFilterer(address common.Address, filterer bind.ContractFilterer) (*PromptTokenFilterer, error) {
	contract, err := bindPromptToken(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &PromptTokenFilterer{contract: contract}, nil
}

// bindPromptToken binds a generic wrapper to an already deployed contract.
func bindPromptToken(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := PromptTokenMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_PromptToken *PromptTokenRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _PromptToken.Contract.PromptTokenCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_PromptToken *PromptTokenRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _PromptToken.Contract.PromptTokenTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_PromptToken *PromptTokenRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _PromptToken.Contract.PromptTokenTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_PromptToken *PromptTokenCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _PromptToken.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_PromptToken *PromptTokenTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _PromptToken.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_PromptToken *PromptTokenTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _PromptToken.Contract.contract.Transact(opts, method, params...)
}

// DEFAULTADMINROLE is a free data retrieval call binding the contract method 0xa217fddf.
//
// Solidity: function DEFAULT_ADMIN_ROLE() view returns(bytes32)
func (_PromptToken *PromptTokenCaller) DEFAULTADMINROLE(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "DEFAULT_ADMIN_ROLE")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// DEFAULTADMINROLE is a free data retrieval call binding the contract method 0xa217fddf.
//
// Solidity: function DEFAULT_ADMIN_ROLE() view returns(bytes32)
func (_PromptToken *PromptTokenSession) DEFAULTADMINROLE() ([32]byte, error) {
	return _PromptToken.Contract.DEFAULTADMINROLE(&_PromptToken.CallOpts)
}

// DEFAULTADMINROLE is a free data retrieval call binding the contract method 0xa217fddf.
//
// Solidity: function DEFAULT_ADMIN_ROLE() view returns(bytes32)
func (_PromptToken *PromptTokenCallerSession) DEFAULTADMINROLE() ([32]byte, error) {
	return _PromptToken.Contract.DEFAULTADMINROLE(&_PromptToken.CallOpts)
}

// INVOKEWAYFINDERCONFIGURATIONROLE is a free data retrieval call binding the contract method 0x70b9b0a6.
//
// Solidity: function INVOKE_WAYFINDER_CONFIGURATION_ROLE() view returns(bytes32)
func (_PromptToken *PromptTokenCaller) INVOKEWAYFINDERCONFIGURATIONROLE(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "INVOKE_WAYFINDER_CONFIGURATION_ROLE")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// INVOKEWAYFINDERCONFIGURATIONROLE is a free data retrieval call binding the contract method 0x70b9b0a6.
//
// Solidity: function INVOKE_WAYFINDER_CONFIGURATION_ROLE() view returns(bytes32)
func (_PromptToken *PromptTokenSession) INVOKEWAYFINDERCONFIGURATIONROLE() ([32]byte, error) {
	return _PromptToken.Contract.INVOKEWAYFINDERCONFIGURATIONROLE(&_PromptToken.CallOpts)
}

// INVOKEWAYFINDERCONFIGURATIONROLE is a free data retrieval call binding the contract method 0x70b9b0a6.
//
// Solidity: function INVOKE_WAYFINDER_CONFIGURATION_ROLE() view returns(bytes32)
func (_PromptToken *PromptTokenCallerSession) INVOKEWAYFINDERCONFIGURATIONROLE() ([32]byte, error) {
	return _PromptToken.Contract.INVOKEWAYFINDERCONFIGURATIONROLE(&_PromptToken.CallOpts)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address owner, address spender) view returns(uint256)
func (_PromptToken *PromptTokenCaller) Allowance(opts *bind.CallOpts, owner common.Address, spender common.Address) (*big.Int, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "allowance", owner, spender)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address owner, address spender) view returns(uint256)
func (_PromptToken *PromptTokenSession) Allowance(owner common.Address, spender common.Address) (*big.Int, error) {
	return _PromptToken.Contract.Allowance(&_PromptToken.CallOpts, owner, spender)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address owner, address spender) view returns(uint256)
func (_PromptToken *PromptTokenCallerSession) Allowance(owner common.Address, spender common.Address) (*big.Int, error) {
	return _PromptToken.Contract.Allowance(&_PromptToken.CallOpts, owner, spender)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address account) view returns(uint256)
func (_PromptToken *PromptTokenCaller) BalanceOf(opts *bind.CallOpts, account common.Address) (*big.Int, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "balanceOf", account)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address account) view returns(uint256)
func (_PromptToken *PromptTokenSession) BalanceOf(account common.Address) (*big.Int, error) {
	return _PromptToken.Contract.BalanceOf(&_PromptToken.CallOpts, account)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address account) view returns(uint256)
func (_PromptToken *PromptTokenCallerSession) BalanceOf(account common.Address) (*big.Int, error) {
	return _PromptToken.Contract.BalanceOf(&_PromptToken.CallOpts, account)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_PromptToken *PromptTokenCaller) Decimals(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "decimals")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_PromptToken *PromptTokenSession) Decimals() (uint8, error) {
	return _PromptToken.Contract.Decimals(&_PromptToken.CallOpts)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_PromptToken *PromptTokenCallerSession) Decimals() (uint8, error) {
	return _PromptToken.Contract.Decimals(&_PromptToken.CallOpts)
}

// GetRoleAdmin is a free data retrieval call binding the contract method 0x248a9ca3.
//
// Solidity: function getRoleAdmin(bytes32 role) view returns(bytes32)
func (_PromptToken *PromptTokenCaller) GetRoleAdmin(opts *bind.CallOpts, role [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "getRoleAdmin", role)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetRoleAdmin is a free data retrieval call binding the contract method 0x248a9ca3.
//
// Solidity: function getRoleAdmin(bytes32 role) view returns(bytes32)
func (_PromptToken *PromptTokenSession) GetRoleAdmin(role [32]byte) ([32]byte, error) {
	return _PromptToken.Contract.GetRoleAdmin(&_PromptToken.CallOpts, role)
}

// GetRoleAdmin is a free data retrieval call binding the contract method 0x248a9ca3.
//
// Solidity: function getRoleAdmin(bytes32 role) view returns(bytes32)
func (_PromptToken *PromptTokenCallerSession) GetRoleAdmin(role [32]byte) ([32]byte, error) {
	return _PromptToken.Contract.GetRoleAdmin(&_PromptToken.CallOpts, role)
}

// HasRole is a free data retrieval call binding the contract method 0x91d14854.
//
// Solidity: function hasRole(bytes32 role, address account) view returns(bool)
func (_PromptToken *PromptTokenCaller) HasRole(opts *bind.CallOpts, role [32]byte, account common.Address) (bool, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "hasRole", role, account)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// HasRole is a free data retrieval call binding the contract method 0x91d14854.
//
// Solidity: function hasRole(bytes32 role, address account) view returns(bool)
func (_PromptToken *PromptTokenSession) HasRole(role [32]byte, account common.Address) (bool, error) {
	return _PromptToken.Contract.HasRole(&_PromptToken.CallOpts, role, account)
}

// HasRole is a free data retrieval call binding the contract method 0x91d14854.
//
// Solidity: function hasRole(bytes32 role, address account) view returns(bool)
func (_PromptToken *PromptTokenCallerSession) HasRole(role [32]byte, account common.Address) (bool, error) {
	return _PromptToken.Contract.HasRole(&_PromptToken.CallOpts, role, account)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_PromptToken *PromptTokenCaller) Name(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "name")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_PromptToken *PromptTokenSession) Name() (string, error) {
	return _PromptToken.Contract.Name(&_PromptToken.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_PromptToken *PromptTokenCallerSession) Name() (string, error) {
	return _PromptToken.Contract.Name(&_PromptToken.CallOpts)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_PromptToken *PromptTokenCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_PromptToken *PromptTokenSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _PromptToken.Contract.SupportsInterface(&_PromptToken.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_PromptToken *PromptTokenCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _PromptToken.Contract.SupportsInterface(&_PromptToken.CallOpts, interfaceId)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_PromptToken *PromptTokenCaller) Symbol(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "symbol")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_PromptToken *PromptTokenSession) Symbol() (string, error) {
	return _PromptToken.Contract.Symbol(&_PromptToken.CallOpts)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_PromptToken *PromptTokenCallerSession) Symbol() (string, error) {
	return _PromptToken.Contract.Symbol(&_PromptToken.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_PromptToken *PromptTokenCaller) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "totalSupply")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_PromptToken *PromptTokenSession) TotalSupply() (*big.Int, error) {
	return _PromptToken.Contract.TotalSupply(&_PromptToken.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_PromptToken *PromptTokenCallerSession) TotalSupply() (*big.Int, error) {
	return _PromptToken.Contract.TotalSupply(&_PromptToken.CallOpts)
}

// WayfinderGateways is a free data retrieval call binding the contract method 0x4122eedf.
//
// Solidity: function wayfinderGateways(address ) view returns(address nativeTokenDestinationAddress, address promptDestinationAddress, address invokeWayfinderHandler)
func (_PromptToken *PromptTokenCaller) WayfinderGateways(opts *bind.CallOpts, arg0 common.Address) (struct {
	NativeTokenDestinationAddress common.Address
	PromptDestinationAddress      common.Address
	InvokeWayfinderHandler        common.Address
}, error) {
	var out []interface{}
	err := _PromptToken.contract.Call(opts, &out, "wayfinderGateways", arg0)

	outstruct := new(struct {
		NativeTokenDestinationAddress common.Address
		PromptDestinationAddress      common.Address
		InvokeWayfinderHandler        common.Address
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.NativeTokenDestinationAddress = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.PromptDestinationAddress = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)
	outstruct.InvokeWayfinderHandler = *abi.ConvertType(out[2], new(common.Address)).(*common.Address)

	return *outstruct, err

}

// WayfinderGateways is a free data retrieval call binding the contract method 0x4122eedf.
//
// Solidity: function wayfinderGateways(address ) view returns(address nativeTokenDestinationAddress, address promptDestinationAddress, address invokeWayfinderHandler)
func (_PromptToken *PromptTokenSession) WayfinderGateways(arg0 common.Address) (struct {
	NativeTokenDestinationAddress common.Address
	PromptDestinationAddress      common.Address
	InvokeWayfinderHandler        common.Address
}, error) {
	return _PromptToken.Contract.WayfinderGateways(&_PromptToken.CallOpts, arg0)
}

// WayfinderGateways is a free data retrieval call binding the contract method 0x4122eedf.
//
// Solidity: function wayfinderGateways(address ) view returns(address nativeTokenDestinationAddress, address promptDestinationAddress, address invokeWayfinderHandler)
func (_PromptToken *PromptTokenCallerSession) WayfinderGateways(arg0 common.Address) (struct {
	NativeTokenDestinationAddress common.Address
	PromptDestinationAddress      common.Address
	InvokeWayfinderHandler        common.Address
}, error) {
	return _PromptToken.Contract.WayfinderGateways(&_PromptToken.CallOpts, arg0)
}

// AddWayfinderHandlerContract is a paid mutator transaction binding the contract method 0x84b09c0a.
//
// Solidity: function addWayfinderHandlerContract(address _contractAddress, address _nativeTokenDestinationAddress, address _promptDestinationAddress) returns()
func (_PromptToken *PromptTokenTransactor) AddWayfinderHandlerContract(opts *bind.TransactOpts, _contractAddress common.Address, _nativeTokenDestinationAddress common.Address, _promptDestinationAddress common.Address) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "addWayfinderHandlerContract", _contractAddress, _nativeTokenDestinationAddress, _promptDestinationAddress)
}

// AddWayfinderHandlerContract is a paid mutator transaction binding the contract method 0x84b09c0a.
//
// Solidity: function addWayfinderHandlerContract(address _contractAddress, address _nativeTokenDestinationAddress, address _promptDestinationAddress) returns()
func (_PromptToken *PromptTokenSession) AddWayfinderHandlerContract(_contractAddress common.Address, _nativeTokenDestinationAddress common.Address, _promptDestinationAddress common.Address) (*types.Transaction, error) {
	return _PromptToken.Contract.AddWayfinderHandlerContract(&_PromptToken.TransactOpts, _contractAddress, _nativeTokenDestinationAddress, _promptDestinationAddress)
}

// AddWayfinderHandlerContract is a paid mutator transaction binding the contract method 0x84b09c0a.
//
// Solidity: function addWayfinderHandlerContract(address _contractAddress, address _nativeTokenDestinationAddress, address _promptDestinationAddress) returns()
func (_PromptToken *PromptTokenTransactorSession) AddWayfinderHandlerContract(_contractAddress common.Address, _nativeTokenDestinationAddress common.Address, _promptDestinationAddress common.Address) (*types.Transaction, error) {
	return _PromptToken.Contract.AddWayfinderHandlerContract(&_PromptToken.TransactOpts, _contractAddress, _nativeTokenDestinationAddress, _promptDestinationAddress)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 value) returns(bool)
func (_PromptToken *PromptTokenTransactor) Approve(opts *bind.TransactOpts, spender common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "approve", spender, value)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 value) returns(bool)
func (_PromptToken *PromptTokenSession) Approve(spender common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.Approve(&_PromptToken.TransactOpts, spender, value)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 value) returns(bool)
func (_PromptToken *PromptTokenTransactorSession) Approve(spender common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.Approve(&_PromptToken.TransactOpts, spender, value)
}

// Burn is a paid mutator transaction binding the contract method 0x42966c68.
//
// Solidity: function burn(uint256 value) returns()
func (_PromptToken *PromptTokenTransactor) Burn(opts *bind.TransactOpts, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "burn", value)
}

// Burn is a paid mutator transaction binding the contract method 0x42966c68.
//
// Solidity: function burn(uint256 value) returns()
func (_PromptToken *PromptTokenSession) Burn(value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.Burn(&_PromptToken.TransactOpts, value)
}

// Burn is a paid mutator transaction binding the contract method 0x42966c68.
//
// Solidity: function burn(uint256 value) returns()
func (_PromptToken *PromptTokenTransactorSession) Burn(value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.Burn(&_PromptToken.TransactOpts, value)
}

// BurnFrom is a paid mutator transaction binding the contract method 0x79cc6790.
//
// Solidity: function burnFrom(address account, uint256 value) returns()
func (_PromptToken *PromptTokenTransactor) BurnFrom(opts *bind.TransactOpts, account common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "burnFrom", account, value)
}

// BurnFrom is a paid mutator transaction binding the contract method 0x79cc6790.
//
// Solidity: function burnFrom(address account, uint256 value) returns()
func (_PromptToken *PromptTokenSession) BurnFrom(account common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.BurnFrom(&_PromptToken.TransactOpts, account, value)
}

// BurnFrom is a paid mutator transaction binding the contract method 0x79cc6790.
//
// Solidity: function burnFrom(address account, uint256 value) returns()
func (_PromptToken *PromptTokenTransactorSession) BurnFrom(account common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.BurnFrom(&_PromptToken.TransactOpts, account, value)
}

// GrantRole is a paid mutator transaction binding the contract method 0x2f2ff15d.
//
// Solidity: function grantRole(bytes32 role, address account) returns()
func (_PromptToken *PromptTokenTransactor) GrantRole(opts *bind.TransactOpts, role [32]byte, account common.Address) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "grantRole", role, account)
}

// GrantRole is a paid mutator transaction binding the contract method 0x2f2ff15d.
//
// Solidity: function grantRole(bytes32 role, address account) returns()
func (_PromptToken *PromptTokenSession) GrantRole(role [32]byte, account common.Address) (*types.Transaction, error) {
	return _PromptToken.Contract.GrantRole(&_PromptToken.TransactOpts, role, account)
}

// GrantRole is a paid mutator transaction binding the contract method 0x2f2ff15d.
//
// Solidity: function grantRole(bytes32 role, address account) returns()
func (_PromptToken *PromptTokenTransactorSession) GrantRole(role [32]byte, account common.Address) (*types.Transaction, error) {
	return _PromptToken.Contract.GrantRole(&_PromptToken.TransactOpts, role, account)
}

// InvokeWayfinder is a paid mutator transaction binding the contract method 0x7287a99e.
//
// Solidity: function invokeWayfinder(address _handlerAddress, uint256 _id, uint256 _promptValue, bytes _data) payable returns()
func (_PromptToken *PromptTokenTransactor) InvokeWayfinder(opts *bind.TransactOpts, _handlerAddress common.Address, _id *big.Int, _promptValue *big.Int, _data []byte) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "invokeWayfinder", _handlerAddress, _id, _promptValue, _data)
}

// InvokeWayfinder is a paid mutator transaction binding the contract method 0x7287a99e.
//
// Solidity: function invokeWayfinder(address _handlerAddress, uint256 _id, uint256 _promptValue, bytes _data) payable returns()
func (_PromptToken *PromptTokenSession) InvokeWayfinder(_handlerAddress common.Address, _id *big.Int, _promptValue *big.Int, _data []byte) (*types.Transaction, error) {
	return _PromptToken.Contract.InvokeWayfinder(&_PromptToken.TransactOpts, _handlerAddress, _id, _promptValue, _data)
}

// InvokeWayfinder is a paid mutator transaction binding the contract method 0x7287a99e.
//
// Solidity: function invokeWayfinder(address _handlerAddress, uint256 _id, uint256 _promptValue, bytes _data) payable returns()
func (_PromptToken *PromptTokenTransactorSession) InvokeWayfinder(_handlerAddress common.Address, _id *big.Int, _promptValue *big.Int, _data []byte) (*types.Transaction, error) {
	return _PromptToken.Contract.InvokeWayfinder(&_PromptToken.TransactOpts, _handlerAddress, _id, _promptValue, _data)
}

// RenounceRole is a paid mutator transaction binding the contract method 0x36568abe.
//
// Solidity: function renounceRole(bytes32 , address ) returns()
func (_PromptToken *PromptTokenTransactor) RenounceRole(opts *bind.TransactOpts, arg0 [32]byte, arg1 common.Address) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "renounceRole", arg0, arg1)
}

// RenounceRole is a paid mutator transaction binding the contract method 0x36568abe.
//
// Solidity: function renounceRole(bytes32 , address ) returns()
func (_PromptToken *PromptTokenSession) RenounceRole(arg0 [32]byte, arg1 common.Address) (*types.Transaction, error) {
	return _PromptToken.Contract.RenounceRole(&_PromptToken.TransactOpts, arg0, arg1)
}

// RenounceRole is a paid mutator transaction binding the contract method 0x36568abe.
//
// Solidity: function renounceRole(bytes32 , address ) returns()
func (_PromptToken *PromptTokenTransactorSession) RenounceRole(arg0 [32]byte, arg1 common.Address) (*types.Transaction, error) {
	return _PromptToken.Contract.RenounceRole(&_PromptToken.TransactOpts, arg0, arg1)
}

// RevokeRole is a paid mutator transaction binding the contract method 0xd547741f.
//
// Solidity: function revokeRole(bytes32 role, address account) returns()
func (_PromptToken *PromptTokenTransactor) RevokeRole(opts *bind.TransactOpts, role [32]byte, account common.Address) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "revokeRole", role, account)
}

// RevokeRole is a paid mutator transaction binding the contract method 0xd547741f.
//
// Solidity: function revokeRole(bytes32 role, address account) returns()
func (_PromptToken *PromptTokenSession) RevokeRole(role [32]byte, account common.Address) (*types.Transaction, error) {
	return _PromptToken.Contract.RevokeRole(&_PromptToken.TransactOpts, role, account)
}

// RevokeRole is a paid mutator transaction binding the contract method 0xd547741f.
//
// Solidity: function revokeRole(bytes32 role, address account) returns()
func (_PromptToken *PromptTokenTransactorSession) RevokeRole(role [32]byte, account common.Address) (*types.Transaction, error) {
	return _PromptToken.Contract.RevokeRole(&_PromptToken.TransactOpts, role, account)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 value) returns(bool)
func (_PromptToken *PromptTokenTransactor) Transfer(opts *bind.TransactOpts, to common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "transfer", to, value)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 value) returns(bool)
func (_PromptToken *PromptTokenSession) Transfer(to common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.Transfer(&_PromptToken.TransactOpts, to, value)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 value) returns(bool)
func (_PromptToken *PromptTokenTransactorSession) Transfer(to common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.Transfer(&_PromptToken.TransactOpts, to, value)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 value) returns(bool)
func (_PromptToken *PromptTokenTransactor) TransferFrom(opts *bind.TransactOpts, from common.Address, to common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.contract.Transact(opts, "transferFrom", from, to, value)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 value) returns(bool)
func (_PromptToken *PromptTokenSession) TransferFrom(from common.Address, to common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.TransferFrom(&_PromptToken.TransactOpts, from, to, value)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 value) returns(bool)
func (_PromptToken *PromptTokenTransactorSession) TransferFrom(from common.Address, to common.Address, value *big.Int) (*types.Transaction, error) {
	return _PromptToken.Contract.TransferFrom(&_PromptToken.TransactOpts, from, to, value)
}

// PromptTokenApprovalIterator is returned from FilterApproval and is used to iterate over the raw logs and unpacked data for Approval events raised by the PromptToken contract.
type PromptTokenApprovalIterator struct {
	Event *PromptTokenApproval // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PromptTokenApprovalIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PromptTokenApproval)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PromptTokenApproval)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PromptTokenApprovalIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PromptTokenApprovalIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PromptTokenApproval represents a Approval event raised by the PromptToken contract.
type PromptTokenApproval struct {
	Owner   common.Address
	Spender common.Address
	Value   *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterApproval is a free log retrieval operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_PromptToken *PromptTokenFilterer) FilterApproval(opts *bind.FilterOpts, owner []common.Address, spender []common.Address) (*PromptTokenApprovalIterator, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var spenderRule []interface{}
	for _, spenderItem := range spender {
		spenderRule = append(spenderRule, spenderItem)
	}

	logs, sub, err := _PromptToken.contract.FilterLogs(opts, "Approval", ownerRule, spenderRule)
	if err != nil {
		return nil, err
	}
	return &PromptTokenApprovalIterator{contract: _PromptToken.contract, event: "Approval", logs: logs, sub: sub}, nil
}

// WatchApproval is a free log subscription operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_PromptToken *PromptTokenFilterer) WatchApproval(opts *bind.WatchOpts, sink chan<- *PromptTokenApproval, owner []common.Address, spender []common.Address) (event.Subscription, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var spenderRule []interface{}
	for _, spenderItem := range spender {
		spenderRule = append(spenderRule, spenderItem)
	}

	logs, sub, err := _PromptToken.contract.WatchLogs(opts, "Approval", ownerRule, spenderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PromptTokenApproval)
				if err := _PromptToken.contract.UnpackLog(event, "Approval", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseApproval is a log parse operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_PromptToken *PromptTokenFilterer) ParseApproval(log types.Log) (*PromptTokenApproval, error) {
	event := new(PromptTokenApproval)
	if err := _PromptToken.contract.UnpackLog(event, "Approval", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PromptTokenRoleAdminChangedIterator is returned from FilterRoleAdminChanged and is used to iterate over the raw logs and unpacked data for RoleAdminChanged events raised by the PromptToken contract.
type PromptTokenRoleAdminChangedIterator struct {
	Event *PromptTokenRoleAdminChanged // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PromptTokenRoleAdminChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PromptTokenRoleAdminChanged)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PromptTokenRoleAdminChanged)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PromptTokenRoleAdminChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PromptTokenRoleAdminChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PromptTokenRoleAdminChanged represents a RoleAdminChanged event raised by the PromptToken contract.
type PromptTokenRoleAdminChanged struct {
	Role              [32]byte
	PreviousAdminRole [32]byte
	NewAdminRole      [32]byte
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterRoleAdminChanged is a free log retrieval operation binding the contract event 0xbd79b86ffe0ab8e8776151514217cd7cacd52c909f66475c3af44e129f0b00ff.
//
// Solidity: event RoleAdminChanged(bytes32 indexed role, bytes32 indexed previousAdminRole, bytes32 indexed newAdminRole)
func (_PromptToken *PromptTokenFilterer) FilterRoleAdminChanged(opts *bind.FilterOpts, role [][32]byte, previousAdminRole [][32]byte, newAdminRole [][32]byte) (*PromptTokenRoleAdminChangedIterator, error) {

	var roleRule []interface{}
	for _, roleItem := range role {
		roleRule = append(roleRule, roleItem)
	}
	var previousAdminRoleRule []interface{}
	for _, previousAdminRoleItem := range previousAdminRole {
		previousAdminRoleRule = append(previousAdminRoleRule, previousAdminRoleItem)
	}
	var newAdminRoleRule []interface{}
	for _, newAdminRoleItem := range newAdminRole {
		newAdminRoleRule = append(newAdminRoleRule, newAdminRoleItem)
	}

	logs, sub, err := _PromptToken.contract.FilterLogs(opts, "RoleAdminChanged", roleRule, previousAdminRoleRule, newAdminRoleRule)
	if err != nil {
		return nil, err
	}
	return &PromptTokenRoleAdminChangedIterator{contract: _PromptToken.contract, event: "RoleAdminChanged", logs: logs, sub: sub}, nil
}

// WatchRoleAdminChanged is a free log subscription operation binding the contract event 0xbd79b86ffe0ab8e8776151514217cd7cacd52c909f66475c3af44e129f0b00ff.
//
// Solidity: event RoleAdminChanged(bytes32 indexed role, bytes32 indexed previousAdminRole, bytes32 indexed newAdminRole)
func (_PromptToken *PromptTokenFilterer) WatchRoleAdminChanged(opts *bind.WatchOpts, sink chan<- *PromptTokenRoleAdminChanged, role [][32]byte, previousAdminRole [][32]byte, newAdminRole [][32]byte) (event.Subscription, error) {

	var roleRule []interface{}
	for _, roleItem := range role {
		roleRule = append(roleRule, roleItem)
	}
	var previousAdminRoleRule []interface{}
	for _, previousAdminRoleItem := range previousAdminRole {
		previousAdminRoleRule = append(previousAdminRoleRule, previousAdminRoleItem)
	}
	var newAdminRoleRule []interface{}
	for _, newAdminRoleItem := range newAdminRole {
		newAdminRoleRule = append(newAdminRoleRule, newAdminRoleItem)
	}

	logs, sub, err := _PromptToken.contract.WatchLogs(opts, "RoleAdminChanged", roleRule, previousAdminRoleRule, newAdminRoleRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PromptTokenRoleAdminChanged)
				if err := _PromptToken.contract.UnpackLog(event, "RoleAdminChanged", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRoleAdminChanged is a log parse operation binding the contract event 0xbd79b86ffe0ab8e8776151514217cd7cacd52c909f66475c3af44e129f0b00ff.
//
// Solidity: event RoleAdminChanged(bytes32 indexed role, bytes32 indexed previousAdminRole, bytes32 indexed newAdminRole)
func (_PromptToken *PromptTokenFilterer) ParseRoleAdminChanged(log types.Log) (*PromptTokenRoleAdminChanged, error) {
	event := new(PromptTokenRoleAdminChanged)
	if err := _PromptToken.contract.UnpackLog(event, "RoleAdminChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PromptTokenRoleGrantedIterator is returned from FilterRoleGranted and is used to iterate over the raw logs and unpacked data for RoleGranted events raised by the PromptToken contract.
type PromptTokenRoleGrantedIterator struct {
	Event *PromptTokenRoleGranted // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PromptTokenRoleGrantedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PromptTokenRoleGranted)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PromptTokenRoleGranted)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PromptTokenRoleGrantedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PromptTokenRoleGrantedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PromptTokenRoleGranted represents a RoleGranted event raised by the PromptToken contract.
type PromptTokenRoleGranted struct {
	Role    [32]byte
	Account common.Address
	Sender  common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterRoleGranted is a free log retrieval operation binding the contract event 0x2f8788117e7eff1d82e926ec794901d17c78024a50270940304540a733656f0d.
//
// Solidity: event RoleGranted(bytes32 indexed role, address indexed account, address indexed sender)
func (_PromptToken *PromptTokenFilterer) FilterRoleGranted(opts *bind.FilterOpts, role [][32]byte, account []common.Address, sender []common.Address) (*PromptTokenRoleGrantedIterator, error) {

	var roleRule []interface{}
	for _, roleItem := range role {
		roleRule = append(roleRule, roleItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _PromptToken.contract.FilterLogs(opts, "RoleGranted", roleRule, accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return &PromptTokenRoleGrantedIterator{contract: _PromptToken.contract, event: "RoleGranted", logs: logs, sub: sub}, nil
}

// WatchRoleGranted is a free log subscription operation binding the contract event 0x2f8788117e7eff1d82e926ec794901d17c78024a50270940304540a733656f0d.
//
// Solidity: event RoleGranted(bytes32 indexed role, address indexed account, address indexed sender)
func (_PromptToken *PromptTokenFilterer) WatchRoleGranted(opts *bind.WatchOpts, sink chan<- *PromptTokenRoleGranted, role [][32]byte, account []common.Address, sender []common.Address) (event.Subscription, error) {

	var roleRule []interface{}
	for _, roleItem := range role {
		roleRule = append(roleRule, roleItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _PromptToken.contract.WatchLogs(opts, "RoleGranted", roleRule, accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PromptTokenRoleGranted)
				if err := _PromptToken.contract.UnpackLog(event, "RoleGranted", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRoleGranted is a log parse operation binding the contract event 0x2f8788117e7eff1d82e926ec794901d17c78024a50270940304540a733656f0d.
//
// Solidity: event RoleGranted(bytes32 indexed role, address indexed account, address indexed sender)
func (_PromptToken *PromptTokenFilterer) ParseRoleGranted(log types.Log) (*PromptTokenRoleGranted, error) {
	event := new(PromptTokenRoleGranted)
	if err := _PromptToken.contract.UnpackLog(event, "RoleGranted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PromptTokenRoleRevokedIterator is returned from FilterRoleRevoked and is used to iterate over the raw logs and unpacked data for RoleRevoked events raised by the PromptToken contract.
type PromptTokenRoleRevokedIterator struct {
	Event *PromptTokenRoleRevoked // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PromptTokenRoleRevokedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PromptTokenRoleRevoked)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PromptTokenRoleRevoked)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PromptTokenRoleRevokedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PromptTokenRoleRevokedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PromptTokenRoleRevoked represents a RoleRevoked event raised by the PromptToken contract.
type PromptTokenRoleRevoked struct {
	Role    [32]byte
	Account common.Address
	Sender  common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterRoleRevoked is a free log retrieval operation binding the contract event 0xf6391f5c32d9c69d2a47ea670b442974b53935d1edc7fd64eb21e047a839171b.
//
// Solidity: event RoleRevoked(bytes32 indexed role, address indexed account, address indexed sender)
func (_PromptToken *PromptTokenFilterer) FilterRoleRevoked(opts *bind.FilterOpts, role [][32]byte, account []common.Address, sender []common.Address) (*PromptTokenRoleRevokedIterator, error) {

	var roleRule []interface{}
	for _, roleItem := range role {
		roleRule = append(roleRule, roleItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _PromptToken.contract.FilterLogs(opts, "RoleRevoked", roleRule, accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return &PromptTokenRoleRevokedIterator{contract: _PromptToken.contract, event: "RoleRevoked", logs: logs, sub: sub}, nil
}

// WatchRoleRevoked is a free log subscription operation binding the contract event 0xf6391f5c32d9c69d2a47ea670b442974b53935d1edc7fd64eb21e047a839171b.
//
// Solidity: event RoleRevoked(bytes32 indexed role, address indexed account, address indexed sender)
func (_PromptToken *PromptTokenFilterer) WatchRoleRevoked(opts *bind.WatchOpts, sink chan<- *PromptTokenRoleRevoked, role [][32]byte, account []common.Address, sender []common.Address) (event.Subscription, error) {

	var roleRule []interface{}
	for _, roleItem := range role {
		roleRule = append(roleRule, roleItem)
	}
	var accountRule []interface{}
	for _, accountItem := range account {
		accountRule = append(accountRule, accountItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}

	logs, sub, err := _PromptToken.contract.WatchLogs(opts, "RoleRevoked", roleRule, accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PromptTokenRoleRevoked)
				if err := _PromptToken.contract.UnpackLog(event, "RoleRevoked", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseRoleRevoked is a log parse operation binding the contract event 0xf6391f5c32d9c69d2a47ea670b442974b53935d1edc7fd64eb21e047a839171b.
//
// Solidity: event RoleRevoked(bytes32 indexed role, address indexed account, address indexed sender)
func (_PromptToken *PromptTokenFilterer) ParseRoleRevoked(log types.Log) (*PromptTokenRoleRevoked, error) {
	event := new(PromptTokenRoleRevoked)
	if err := _PromptToken.contract.UnpackLog(event, "RoleRevoked", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PromptTokenTransferIterator is returned from FilterTransfer and is used to iterate over the raw logs and unpacked data for Transfer events raised by the PromptToken contract.
type PromptTokenTransferIterator struct {
	Event *PromptTokenTransfer // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PromptTokenTransferIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PromptTokenTransfer)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PromptTokenTransfer)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PromptTokenTransferIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PromptTokenTransferIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PromptTokenTransfer represents a Transfer event raised by the PromptToken contract.
type PromptTokenTransfer struct {
	From  common.Address
	To    common.Address
	Value *big.Int
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterTransfer is a free log retrieval operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_PromptToken *PromptTokenFilterer) FilterTransfer(opts *bind.FilterOpts, from []common.Address, to []common.Address) (*PromptTokenTransferIterator, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _PromptToken.contract.FilterLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &PromptTokenTransferIterator{contract: _PromptToken.contract, event: "Transfer", logs: logs, sub: sub}, nil
}

// WatchTransfer is a free log subscription operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_PromptToken *PromptTokenFilterer) WatchTransfer(opts *bind.WatchOpts, sink chan<- *PromptTokenTransfer, from []common.Address, to []common.Address) (event.Subscription, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _PromptToken.contract.WatchLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PromptTokenTransfer)
				if err := _PromptToken.contract.UnpackLog(event, "Transfer", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseTransfer is a log parse operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_PromptToken *PromptTokenFilterer) ParseTransfer(log types.Log) (*PromptTokenTransfer, error) {
	event := new(PromptTokenTransfer)
	if err := _PromptToken.contract.UnpackLog(event, "Transfer", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PromptTokenWayfinderGatewayRegisteredIterator is returned from FilterWayfinderGatewayRegistered and is used to iterate over the raw logs and unpacked data for WayfinderGatewayRegistered events raised by the PromptToken contract.
type PromptTokenWayfinderGatewayRegisteredIterator struct {
	Event *PromptTokenWayfinderGatewayRegistered // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PromptTokenWayfinderGatewayRegisteredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PromptTokenWayfinderGatewayRegistered)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PromptTokenWayfinderGatewayRegistered)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PromptTokenWayfinderGatewayRegisteredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PromptTokenWayfinderGatewayRegisteredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PromptTokenWayfinderGatewayRegistered represents a WayfinderGatewayRegistered event raised by the PromptToken contract.
type PromptTokenWayfinderGatewayRegistered struct {
	ContractAddress               common.Address
	NativeTokenDestinationAddress common.Address
	PromptDestinationAddress      common.Address
	Raw                           types.Log // Blockchain specific contextual infos
}

// FilterWayfinderGatewayRegistered is a free log retrieval operation binding the contract event 0x8cf6993df5e0b575ca3342eea156230eac743b5c3cc8755911acd9ac953a953e.
//
// Solidity: event WayfinderGatewayRegistered(address indexed contractAddress, address indexed nativeTokenDestinationAddress, address indexed promptDestinationAddress)
func (_PromptToken *PromptTokenFilterer) FilterWayfinderGatewayRegistered(opts *bind.FilterOpts, contractAddress []common.Address, nativeTokenDestinationAddress []common.Address, promptDestinationAddress []common.Address) (*PromptTokenWayfinderGatewayRegisteredIterator, error) {

	var contractAddressRule []interface{}
	for _, contractAddressItem := range contractAddress {
		contractAddressRule = append(contractAddressRule, contractAddressItem)
	}
	var nativeTokenDestinationAddressRule []interface{}
	for _, nativeTokenDestinationAddressItem := range nativeTokenDestinationAddress {
		nativeTokenDestinationAddressRule = append(nativeTokenDestinationAddressRule, nativeTokenDestinationAddressItem)
	}
	var promptDestinationAddressRule []interface{}
	for _, promptDestinationAddressItem := range promptDestinationAddress {
		promptDestinationAddressRule = append(promptDestinationAddressRule, promptDestinationAddressItem)
	}

	logs, sub, err := _PromptToken.contract.FilterLogs(opts, "WayfinderGatewayRegistered", contractAddressRule, nativeTokenDestinationAddressRule, promptDestinationAddressRule)
	if err != nil {
		return nil, err
	}
	return &PromptTokenWayfinderGatewayRegisteredIterator{contract: _PromptToken.contract, event: "WayfinderGatewayRegistered", logs: logs, sub: sub}, nil
}

// WatchWayfinderGatewayRegistered is a free log subscription operation binding the contract event 0x8cf6993df5e0b575ca3342eea156230eac743b5c3cc8755911acd9ac953a953e.
//
// Solidity: event WayfinderGatewayRegistered(address indexed contractAddress, address indexed nativeTokenDestinationAddress, address indexed promptDestinationAddress)
func (_PromptToken *PromptTokenFilterer) WatchWayfinderGatewayRegistered(opts *bind.WatchOpts, sink chan<- *PromptTokenWayfinderGatewayRegistered, contractAddress []common.Address, nativeTokenDestinationAddress []common.Address, promptDestinationAddress []common.Address) (event.Subscription, error) {

	var contractAddressRule []interface{}
	for _, contractAddressItem := range contractAddress {
		contractAddressRule = append(contractAddressRule, contractAddressItem)
	}
	var nativeTokenDestinationAddressRule []interface{}
	for _, nativeTokenDestinationAddressItem := range nativeTokenDestinationAddress {
		nativeTokenDestinationAddressRule = append(nativeTokenDestinationAddressRule, nativeTokenDestinationAddressItem)
	}
	var promptDestinationAddressRule []interface{}
	for _, promptDestinationAddressItem := range promptDestinationAddress {
		promptDestinationAddressRule = append(promptDestinationAddressRule, promptDestinationAddressItem)
	}

	logs, sub, err := _PromptToken.contract.WatchLogs(opts, "WayfinderGatewayRegistered", contractAddressRule, nativeTokenDestinationAddressRule, promptDestinationAddressRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PromptTokenWayfinderGatewayRegistered)
				if err := _PromptToken.contract.UnpackLog(event, "WayfinderGatewayRegistered", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseWayfinderGatewayRegistered is a log parse operation binding the contract event 0x8cf6993df5e0b575ca3342eea156230eac743b5c3cc8755911acd9ac953a953e.
//
// Solidity: event WayfinderGatewayRegistered(address indexed contractAddress, address indexed nativeTokenDestinationAddress, address indexed promptDestinationAddress)
func (_PromptToken *PromptTokenFilterer) ParseWayfinderGatewayRegistered(log types.Log) (*PromptTokenWayfinderGatewayRegistered, error) {
	event := new(PromptTokenWayfinderGatewayRegistered)
	if err := _PromptToken.contract.UnpackLog(event, "WayfinderGatewayRegistered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// PromptTokenWayfinderInvokedIterator is returned from FilterWayfinderInvoked and is used to iterate over the raw logs and unpacked data for WayfinderInvoked events raised by the PromptToken contract.
type PromptTokenWayfinderInvokedIterator struct {
	Event *PromptTokenWayfinderInvoked // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *PromptTokenWayfinderInvokedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(PromptTokenWayfinderInvoked)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(PromptTokenWayfinderInvoked)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *PromptTokenWayfinderInvokedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *PromptTokenWayfinderInvokedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// PromptTokenWayfinderInvoked represents a WayfinderInvoked event raised by the PromptToken contract.
type PromptTokenWayfinderInvoked struct {
	From                   common.Address
	NativeTokenDestination common.Address
	PromptDestination      common.Address
	Id                     *big.Int
	NativeTokenValue       *big.Int
	PromptValue            *big.Int
	Data                   []byte
	Raw                    types.Log // Blockchain specific contextual infos
}

// FilterWayfinderInvoked is a free log retrieval operation binding the contract event 0xe18ff98b8b2004eeb7ed2e5f72ed7232b7f8e004305c73c87b4f21ad29448e3b.
//
// Solidity: event WayfinderInvoked(address indexed from, address indexed nativeTokenDestination, address indexed promptDestination, uint256 id, uint256 nativeTokenValue, uint256 promptValue, bytes data)
func (_PromptToken *PromptTokenFilterer) FilterWayfinderInvoked(opts *bind.FilterOpts, from []common.Address, nativeTokenDestination []common.Address, promptDestination []common.Address) (*PromptTokenWayfinderInvokedIterator, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var nativeTokenDestinationRule []interface{}
	for _, nativeTokenDestinationItem := range nativeTokenDestination {
		nativeTokenDestinationRule = append(nativeTokenDestinationRule, nativeTokenDestinationItem)
	}
	var promptDestinationRule []interface{}
	for _, promptDestinationItem := range promptDestination {
		promptDestinationRule = append(promptDestinationRule, promptDestinationItem)
	}

	logs, sub, err := _PromptToken.contract.FilterLogs(opts, "WayfinderInvoked", fromRule, nativeTokenDestinationRule, promptDestinationRule)
	if err != nil {
		return nil, err
	}
	return &PromptTokenWayfinderInvokedIterator{contract: _PromptToken.contract, event: "WayfinderInvoked", logs: logs, sub: sub}, nil
}

// WatchWayfinderInvoked is a free log subscription operation binding the contract event 0xe18ff98b8b2004eeb7ed2e5f72ed7232b7f8e004305c73c87b4f21ad29448e3b.
//
// Solidity: event WayfinderInvoked(address indexed from, address indexed nativeTokenDestination, address indexed promptDestination, uint256 id, uint256 nativeTokenValue, uint256 promptValue, bytes data)
func (_PromptToken *PromptTokenFilterer) WatchWayfinderInvoked(opts *bind.WatchOpts, sink chan<- *PromptTokenWayfinderInvoked, from []common.Address, nativeTokenDestination []common.Address, promptDestination []common.Address) (event.Subscription, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var nativeTokenDestinationRule []interface{}
	for _, nativeTokenDestinationItem := range nativeTokenDestination {
		nativeTokenDestinationRule = append(nativeTokenDestinationRule, nativeTokenDestinationItem)
	}
	var promptDestinationRule []interface{}
	for _, promptDestinationItem := range promptDestination {
		promptDestinationRule = append(promptDestinationRule, promptDestinationItem)
	}

	logs, sub, err := _PromptToken.contract.WatchLogs(opts, "WayfinderInvoked", fromRule, nativeTokenDestinationRule, promptDestinationRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(PromptTokenWayfinderInvoked)
				if err := _PromptToken.contract.UnpackLog(event, "WayfinderInvoked", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseWayfinderInvoked is a log parse operation binding the contract event 0xe18ff98b8b2004eeb7ed2e5f72ed7232b7f8e004305c73c87b4f21ad29448e3b.
//
// Solidity: event WayfinderInvoked(address indexed from, address indexed nativeTokenDestination, address indexed promptDestination, uint256 id, uint256 nativeTokenValue, uint256 promptValue, bytes data)
func (_PromptToken *PromptTokenFilterer) ParseWayfinderInvoked(log types.Log) (*PromptTokenWayfinderInvoked, error) {
	event := new(PromptTokenWayfinderInvoked)
	if err := _PromptToken.contract.UnpackLog(event, "WayfinderInvoked", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
