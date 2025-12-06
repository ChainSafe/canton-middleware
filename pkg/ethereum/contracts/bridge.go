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

// CantonBridgeMetaData contains all meta data concerning the CantonBridge contract.
var CantonBridgeMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"_relayer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"_maxTransferAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"_minTransferAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"addTokenMapping\",\"inputs\":[{\"name\":\"ethereumToken\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"cantonTokenId\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"cantonToEthereumToken\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"depositNonce\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"depositToCanton\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cantonRecipient\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"emergencyWithdraw\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"ethereumToCantonToken\",\"inputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getBridgeBalance\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"maxTransferAmount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"minTransferAmount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pause\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"paused\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"processedCantonTxs\",\"inputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"relayer\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"removeTokenMapping\",\"inputs\":[{\"name\":\"ethereumToken\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"unpause\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"updateLimits\",\"inputs\":[{\"name\":\"_maxTransferAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"_minTransferAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"updateRelayer\",\"inputs\":[{\"name\":\"newRelayer\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"withdrawFromCanton\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"recipient\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cantonTxHash\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"BridgeLimitsUpdated\",\"inputs\":[{\"name\":\"maxTransferAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"minTransferAmount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DepositToCanton\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"sender\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"cantonRecipient\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Paused\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RelayerUpdated\",\"inputs\":[{\"name\":\"oldRelayer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newRelayer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"TokenMappingAdded\",\"inputs\":[{\"name\":\"ethereumToken\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"cantonTokenId\",\"type\":\"bytes32\",\"indexed\":true,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"TokenMappingRemoved\",\"inputs\":[{\"name\":\"ethereumToken\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Unpaused\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"WithdrawFromCanton\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"recipient\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"amount\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"cantonTxHash\",\"type\":\"bytes32\",\"indexed\":false,\"internalType\":\"bytes32\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"EnforcedPause\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExpectedPause\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ReentrancyGuardReentrantCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"SafeERC20FailedOperation\",\"inputs\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"address\"}]}]",
}

// CantonBridgeABI is the input ABI used to generate the binding from.
// Deprecated: Use CantonBridgeMetaData.ABI instead.
var CantonBridgeABI = CantonBridgeMetaData.ABI

// CantonBridge is an auto generated Go binding around an Ethereum contract.
type CantonBridge struct {
	CantonBridgeCaller     // Read-only binding to the contract
	CantonBridgeTransactor // Write-only binding to the contract
	CantonBridgeFilterer   // Log filterer for contract events
}

// CantonBridgeCaller is an auto generated read-only Go binding around an Ethereum contract.
type CantonBridgeCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CantonBridgeTransactor is an auto generated write-only Go binding around an Ethereum contract.
type CantonBridgeTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CantonBridgeFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type CantonBridgeFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// CantonBridgeSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type CantonBridgeSession struct {
	Contract     *CantonBridge     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// CantonBridgeCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type CantonBridgeCallerSession struct {
	Contract *CantonBridgeCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// CantonBridgeTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type CantonBridgeTransactorSession struct {
	Contract     *CantonBridgeTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// CantonBridgeRaw is an auto generated low-level Go binding around an Ethereum contract.
type CantonBridgeRaw struct {
	Contract *CantonBridge // Generic contract binding to access the raw methods on
}

// CantonBridgeCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type CantonBridgeCallerRaw struct {
	Contract *CantonBridgeCaller // Generic read-only contract binding to access the raw methods on
}

// CantonBridgeTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type CantonBridgeTransactorRaw struct {
	Contract *CantonBridgeTransactor // Generic write-only contract binding to access the raw methods on
}

// NewCantonBridge creates a new instance of CantonBridge, bound to a specific deployed contract.
func NewCantonBridge(address common.Address, backend bind.ContractBackend) (*CantonBridge, error) {
	contract, err := bindCantonBridge(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &CantonBridge{CantonBridgeCaller: CantonBridgeCaller{contract: contract}, CantonBridgeTransactor: CantonBridgeTransactor{contract: contract}, CantonBridgeFilterer: CantonBridgeFilterer{contract: contract}}, nil
}

// NewCantonBridgeCaller creates a new read-only instance of CantonBridge, bound to a specific deployed contract.
func NewCantonBridgeCaller(address common.Address, caller bind.ContractCaller) (*CantonBridgeCaller, error) {
	contract, err := bindCantonBridge(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &CantonBridgeCaller{contract: contract}, nil
}

// NewCantonBridgeTransactor creates a new write-only instance of CantonBridge, bound to a specific deployed contract.
func NewCantonBridgeTransactor(address common.Address, transactor bind.ContractTransactor) (*CantonBridgeTransactor, error) {
	contract, err := bindCantonBridge(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &CantonBridgeTransactor{contract: contract}, nil
}

// NewCantonBridgeFilterer creates a new log filterer instance of CantonBridge, bound to a specific deployed contract.
func NewCantonBridgeFilterer(address common.Address, filterer bind.ContractFilterer) (*CantonBridgeFilterer, error) {
	contract, err := bindCantonBridge(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &CantonBridgeFilterer{contract: contract}, nil
}

// bindCantonBridge binds a generic wrapper to an already deployed contract.
func bindCantonBridge(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := CantonBridgeMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CantonBridge *CantonBridgeRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CantonBridge.Contract.CantonBridgeCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CantonBridge *CantonBridgeRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CantonBridge.Contract.CantonBridgeTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CantonBridge *CantonBridgeRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CantonBridge.Contract.CantonBridgeTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_CantonBridge *CantonBridgeCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _CantonBridge.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_CantonBridge *CantonBridgeTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CantonBridge.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_CantonBridge *CantonBridgeTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _CantonBridge.Contract.contract.Transact(opts, method, params...)
}

// CantonToEthereumToken is a free data retrieval call binding the contract method 0x1a9bd38d.
//
// Solidity: function cantonToEthereumToken(bytes32 ) view returns(address)
func (_CantonBridge *CantonBridgeCaller) CantonToEthereumToken(opts *bind.CallOpts, arg0 [32]byte) (common.Address, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "cantonToEthereumToken", arg0)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// CantonToEthereumToken is a free data retrieval call binding the contract method 0x1a9bd38d.
//
// Solidity: function cantonToEthereumToken(bytes32 ) view returns(address)
func (_CantonBridge *CantonBridgeSession) CantonToEthereumToken(arg0 [32]byte) (common.Address, error) {
	return _CantonBridge.Contract.CantonToEthereumToken(&_CantonBridge.CallOpts, arg0)
}

// CantonToEthereumToken is a free data retrieval call binding the contract method 0x1a9bd38d.
//
// Solidity: function cantonToEthereumToken(bytes32 ) view returns(address)
func (_CantonBridge *CantonBridgeCallerSession) CantonToEthereumToken(arg0 [32]byte) (common.Address, error) {
	return _CantonBridge.Contract.CantonToEthereumToken(&_CantonBridge.CallOpts, arg0)
}

// DepositNonce is a free data retrieval call binding the contract method 0xde35f5cb.
//
// Solidity: function depositNonce() view returns(uint256)
func (_CantonBridge *CantonBridgeCaller) DepositNonce(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "depositNonce")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// DepositNonce is a free data retrieval call binding the contract method 0xde35f5cb.
//
// Solidity: function depositNonce() view returns(uint256)
func (_CantonBridge *CantonBridgeSession) DepositNonce() (*big.Int, error) {
	return _CantonBridge.Contract.DepositNonce(&_CantonBridge.CallOpts)
}

// DepositNonce is a free data retrieval call binding the contract method 0xde35f5cb.
//
// Solidity: function depositNonce() view returns(uint256)
func (_CantonBridge *CantonBridgeCallerSession) DepositNonce() (*big.Int, error) {
	return _CantonBridge.Contract.DepositNonce(&_CantonBridge.CallOpts)
}

// EthereumToCantonToken is a free data retrieval call binding the contract method 0x0cc761ae.
//
// Solidity: function ethereumToCantonToken(address ) view returns(bytes32)
func (_CantonBridge *CantonBridgeCaller) EthereumToCantonToken(opts *bind.CallOpts, arg0 common.Address) ([32]byte, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "ethereumToCantonToken", arg0)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// EthereumToCantonToken is a free data retrieval call binding the contract method 0x0cc761ae.
//
// Solidity: function ethereumToCantonToken(address ) view returns(bytes32)
func (_CantonBridge *CantonBridgeSession) EthereumToCantonToken(arg0 common.Address) ([32]byte, error) {
	return _CantonBridge.Contract.EthereumToCantonToken(&_CantonBridge.CallOpts, arg0)
}

// EthereumToCantonToken is a free data retrieval call binding the contract method 0x0cc761ae.
//
// Solidity: function ethereumToCantonToken(address ) view returns(bytes32)
func (_CantonBridge *CantonBridgeCallerSession) EthereumToCantonToken(arg0 common.Address) ([32]byte, error) {
	return _CantonBridge.Contract.EthereumToCantonToken(&_CantonBridge.CallOpts, arg0)
}

// GetBridgeBalance is a free data retrieval call binding the contract method 0x058c055f.
//
// Solidity: function getBridgeBalance(address token) view returns(uint256)
func (_CantonBridge *CantonBridgeCaller) GetBridgeBalance(opts *bind.CallOpts, token common.Address) (*big.Int, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "getBridgeBalance", token)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetBridgeBalance is a free data retrieval call binding the contract method 0x058c055f.
//
// Solidity: function getBridgeBalance(address token) view returns(uint256)
func (_CantonBridge *CantonBridgeSession) GetBridgeBalance(token common.Address) (*big.Int, error) {
	return _CantonBridge.Contract.GetBridgeBalance(&_CantonBridge.CallOpts, token)
}

// GetBridgeBalance is a free data retrieval call binding the contract method 0x058c055f.
//
// Solidity: function getBridgeBalance(address token) view returns(uint256)
func (_CantonBridge *CantonBridgeCallerSession) GetBridgeBalance(token common.Address) (*big.Int, error) {
	return _CantonBridge.Contract.GetBridgeBalance(&_CantonBridge.CallOpts, token)
}

// MaxTransferAmount is a free data retrieval call binding the contract method 0xa9e75723.
//
// Solidity: function maxTransferAmount() view returns(uint256)
func (_CantonBridge *CantonBridgeCaller) MaxTransferAmount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "maxTransferAmount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MaxTransferAmount is a free data retrieval call binding the contract method 0xa9e75723.
//
// Solidity: function maxTransferAmount() view returns(uint256)
func (_CantonBridge *CantonBridgeSession) MaxTransferAmount() (*big.Int, error) {
	return _CantonBridge.Contract.MaxTransferAmount(&_CantonBridge.CallOpts)
}

// MaxTransferAmount is a free data retrieval call binding the contract method 0xa9e75723.
//
// Solidity: function maxTransferAmount() view returns(uint256)
func (_CantonBridge *CantonBridgeCallerSession) MaxTransferAmount() (*big.Int, error) {
	return _CantonBridge.Contract.MaxTransferAmount(&_CantonBridge.CallOpts)
}

// MinTransferAmount is a free data retrieval call binding the contract method 0x68841431.
//
// Solidity: function minTransferAmount() view returns(uint256)
func (_CantonBridge *CantonBridgeCaller) MinTransferAmount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "minTransferAmount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MinTransferAmount is a free data retrieval call binding the contract method 0x68841431.
//
// Solidity: function minTransferAmount() view returns(uint256)
func (_CantonBridge *CantonBridgeSession) MinTransferAmount() (*big.Int, error) {
	return _CantonBridge.Contract.MinTransferAmount(&_CantonBridge.CallOpts)
}

// MinTransferAmount is a free data retrieval call binding the contract method 0x68841431.
//
// Solidity: function minTransferAmount() view returns(uint256)
func (_CantonBridge *CantonBridgeCallerSession) MinTransferAmount() (*big.Int, error) {
	return _CantonBridge.Contract.MinTransferAmount(&_CantonBridge.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_CantonBridge *CantonBridgeCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_CantonBridge *CantonBridgeSession) Owner() (common.Address, error) {
	return _CantonBridge.Contract.Owner(&_CantonBridge.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_CantonBridge *CantonBridgeCallerSession) Owner() (common.Address, error) {
	return _CantonBridge.Contract.Owner(&_CantonBridge.CallOpts)
}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_CantonBridge *CantonBridgeCaller) Paused(opts *bind.CallOpts) (bool, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "paused")

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_CantonBridge *CantonBridgeSession) Paused() (bool, error) {
	return _CantonBridge.Contract.Paused(&_CantonBridge.CallOpts)
}

// Paused is a free data retrieval call binding the contract method 0x5c975abb.
//
// Solidity: function paused() view returns(bool)
func (_CantonBridge *CantonBridgeCallerSession) Paused() (bool, error) {
	return _CantonBridge.Contract.Paused(&_CantonBridge.CallOpts)
}

// ProcessedCantonTxs is a free data retrieval call binding the contract method 0xd1569b4c.
//
// Solidity: function processedCantonTxs(bytes32 ) view returns(bool)
func (_CantonBridge *CantonBridgeCaller) ProcessedCantonTxs(opts *bind.CallOpts, arg0 [32]byte) (bool, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "processedCantonTxs", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// ProcessedCantonTxs is a free data retrieval call binding the contract method 0xd1569b4c.
//
// Solidity: function processedCantonTxs(bytes32 ) view returns(bool)
func (_CantonBridge *CantonBridgeSession) ProcessedCantonTxs(arg0 [32]byte) (bool, error) {
	return _CantonBridge.Contract.ProcessedCantonTxs(&_CantonBridge.CallOpts, arg0)
}

// ProcessedCantonTxs is a free data retrieval call binding the contract method 0xd1569b4c.
//
// Solidity: function processedCantonTxs(bytes32 ) view returns(bool)
func (_CantonBridge *CantonBridgeCallerSession) ProcessedCantonTxs(arg0 [32]byte) (bool, error) {
	return _CantonBridge.Contract.ProcessedCantonTxs(&_CantonBridge.CallOpts, arg0)
}

// Relayer is a free data retrieval call binding the contract method 0x8406c079.
//
// Solidity: function relayer() view returns(address)
func (_CantonBridge *CantonBridgeCaller) Relayer(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "relayer")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Relayer is a free data retrieval call binding the contract method 0x8406c079.
//
// Solidity: function relayer() view returns(address)
func (_CantonBridge *CantonBridgeSession) Relayer() (common.Address, error) {
	return _CantonBridge.Contract.Relayer(&_CantonBridge.CallOpts)
}

// Relayer is a free data retrieval call binding the contract method 0x8406c079.
//
// Solidity: function relayer() view returns(address)
func (_CantonBridge *CantonBridgeCallerSession) Relayer() (common.Address, error) {
	return _CantonBridge.Contract.Relayer(&_CantonBridge.CallOpts)
}

// AddTokenMapping is a paid mutator transaction binding the contract method 0x3db00af5.
//
// Solidity: function addTokenMapping(address ethereumToken, bytes32 cantonTokenId) returns()
func (_CantonBridge *CantonBridgeTransactor) AddTokenMapping(opts *bind.TransactOpts, ethereumToken common.Address, cantonTokenId [32]byte) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "addTokenMapping", ethereumToken, cantonTokenId)
}

// AddTokenMapping is a paid mutator transaction binding the contract method 0x3db00af5.
//
// Solidity: function addTokenMapping(address ethereumToken, bytes32 cantonTokenId) returns()
func (_CantonBridge *CantonBridgeSession) AddTokenMapping(ethereumToken common.Address, cantonTokenId [32]byte) (*types.Transaction, error) {
	return _CantonBridge.Contract.AddTokenMapping(&_CantonBridge.TransactOpts, ethereumToken, cantonTokenId)
}

// AddTokenMapping is a paid mutator transaction binding the contract method 0x3db00af5.
//
// Solidity: function addTokenMapping(address ethereumToken, bytes32 cantonTokenId) returns()
func (_CantonBridge *CantonBridgeTransactorSession) AddTokenMapping(ethereumToken common.Address, cantonTokenId [32]byte) (*types.Transaction, error) {
	return _CantonBridge.Contract.AddTokenMapping(&_CantonBridge.TransactOpts, ethereumToken, cantonTokenId)
}

// DepositToCanton is a paid mutator transaction binding the contract method 0x62cc30d0.
//
// Solidity: function depositToCanton(address token, uint256 amount, bytes32 cantonRecipient) returns()
func (_CantonBridge *CantonBridgeTransactor) DepositToCanton(opts *bind.TransactOpts, token common.Address, amount *big.Int, cantonRecipient [32]byte) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "depositToCanton", token, amount, cantonRecipient)
}

// DepositToCanton is a paid mutator transaction binding the contract method 0x62cc30d0.
//
// Solidity: function depositToCanton(address token, uint256 amount, bytes32 cantonRecipient) returns()
func (_CantonBridge *CantonBridgeSession) DepositToCanton(token common.Address, amount *big.Int, cantonRecipient [32]byte) (*types.Transaction, error) {
	return _CantonBridge.Contract.DepositToCanton(&_CantonBridge.TransactOpts, token, amount, cantonRecipient)
}

// DepositToCanton is a paid mutator transaction binding the contract method 0x62cc30d0.
//
// Solidity: function depositToCanton(address token, uint256 amount, bytes32 cantonRecipient) returns()
func (_CantonBridge *CantonBridgeTransactorSession) DepositToCanton(token common.Address, amount *big.Int, cantonRecipient [32]byte) (*types.Transaction, error) {
	return _CantonBridge.Contract.DepositToCanton(&_CantonBridge.TransactOpts, token, amount, cantonRecipient)
}

// EmergencyWithdraw is a paid mutator transaction binding the contract method 0x95ccea67.
//
// Solidity: function emergencyWithdraw(address token, uint256 amount) returns()
func (_CantonBridge *CantonBridgeTransactor) EmergencyWithdraw(opts *bind.TransactOpts, token common.Address, amount *big.Int) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "emergencyWithdraw", token, amount)
}

// EmergencyWithdraw is a paid mutator transaction binding the contract method 0x95ccea67.
//
// Solidity: function emergencyWithdraw(address token, uint256 amount) returns()
func (_CantonBridge *CantonBridgeSession) EmergencyWithdraw(token common.Address, amount *big.Int) (*types.Transaction, error) {
	return _CantonBridge.Contract.EmergencyWithdraw(&_CantonBridge.TransactOpts, token, amount)
}

// EmergencyWithdraw is a paid mutator transaction binding the contract method 0x95ccea67.
//
// Solidity: function emergencyWithdraw(address token, uint256 amount) returns()
func (_CantonBridge *CantonBridgeTransactorSession) EmergencyWithdraw(token common.Address, amount *big.Int) (*types.Transaction, error) {
	return _CantonBridge.Contract.EmergencyWithdraw(&_CantonBridge.TransactOpts, token, amount)
}

// Pause is a paid mutator transaction binding the contract method 0x8456cb59.
//
// Solidity: function pause() returns()
func (_CantonBridge *CantonBridgeTransactor) Pause(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "pause")
}

// Pause is a paid mutator transaction binding the contract method 0x8456cb59.
//
// Solidity: function pause() returns()
func (_CantonBridge *CantonBridgeSession) Pause() (*types.Transaction, error) {
	return _CantonBridge.Contract.Pause(&_CantonBridge.TransactOpts)
}

// Pause is a paid mutator transaction binding the contract method 0x8456cb59.
//
// Solidity: function pause() returns()
func (_CantonBridge *CantonBridgeTransactorSession) Pause() (*types.Transaction, error) {
	return _CantonBridge.Contract.Pause(&_CantonBridge.TransactOpts)
}

// RemoveTokenMapping is a paid mutator transaction binding the contract method 0x828a54fb.
//
// Solidity: function removeTokenMapping(address ethereumToken) returns()
func (_CantonBridge *CantonBridgeTransactor) RemoveTokenMapping(opts *bind.TransactOpts, ethereumToken common.Address) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "removeTokenMapping", ethereumToken)
}

// RemoveTokenMapping is a paid mutator transaction binding the contract method 0x828a54fb.
//
// Solidity: function removeTokenMapping(address ethereumToken) returns()
func (_CantonBridge *CantonBridgeSession) RemoveTokenMapping(ethereumToken common.Address) (*types.Transaction, error) {
	return _CantonBridge.Contract.RemoveTokenMapping(&_CantonBridge.TransactOpts, ethereumToken)
}

// RemoveTokenMapping is a paid mutator transaction binding the contract method 0x828a54fb.
//
// Solidity: function removeTokenMapping(address ethereumToken) returns()
func (_CantonBridge *CantonBridgeTransactorSession) RemoveTokenMapping(ethereumToken common.Address) (*types.Transaction, error) {
	return _CantonBridge.Contract.RemoveTokenMapping(&_CantonBridge.TransactOpts, ethereumToken)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_CantonBridge *CantonBridgeTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_CantonBridge *CantonBridgeSession) RenounceOwnership() (*types.Transaction, error) {
	return _CantonBridge.Contract.RenounceOwnership(&_CantonBridge.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_CantonBridge *CantonBridgeTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _CantonBridge.Contract.RenounceOwnership(&_CantonBridge.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_CantonBridge *CantonBridgeTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_CantonBridge *CantonBridgeSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _CantonBridge.Contract.TransferOwnership(&_CantonBridge.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_CantonBridge *CantonBridgeTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _CantonBridge.Contract.TransferOwnership(&_CantonBridge.TransactOpts, newOwner)
}

// Unpause is a paid mutator transaction binding the contract method 0x3f4ba83a.
//
// Solidity: function unpause() returns()
func (_CantonBridge *CantonBridgeTransactor) Unpause(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "unpause")
}

// Unpause is a paid mutator transaction binding the contract method 0x3f4ba83a.
//
// Solidity: function unpause() returns()
func (_CantonBridge *CantonBridgeSession) Unpause() (*types.Transaction, error) {
	return _CantonBridge.Contract.Unpause(&_CantonBridge.TransactOpts)
}

// Unpause is a paid mutator transaction binding the contract method 0x3f4ba83a.
//
// Solidity: function unpause() returns()
func (_CantonBridge *CantonBridgeTransactorSession) Unpause() (*types.Transaction, error) {
	return _CantonBridge.Contract.Unpause(&_CantonBridge.TransactOpts)
}

// UpdateLimits is a paid mutator transaction binding the contract method 0xa2240e19.
//
// Solidity: function updateLimits(uint256 _maxTransferAmount, uint256 _minTransferAmount) returns()
func (_CantonBridge *CantonBridgeTransactor) UpdateLimits(opts *bind.TransactOpts, _maxTransferAmount *big.Int, _minTransferAmount *big.Int) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "updateLimits", _maxTransferAmount, _minTransferAmount)
}

// UpdateLimits is a paid mutator transaction binding the contract method 0xa2240e19.
//
// Solidity: function updateLimits(uint256 _maxTransferAmount, uint256 _minTransferAmount) returns()
func (_CantonBridge *CantonBridgeSession) UpdateLimits(_maxTransferAmount *big.Int, _minTransferAmount *big.Int) (*types.Transaction, error) {
	return _CantonBridge.Contract.UpdateLimits(&_CantonBridge.TransactOpts, _maxTransferAmount, _minTransferAmount)
}

// UpdateLimits is a paid mutator transaction binding the contract method 0xa2240e19.
//
// Solidity: function updateLimits(uint256 _maxTransferAmount, uint256 _minTransferAmount) returns()
func (_CantonBridge *CantonBridgeTransactorSession) UpdateLimits(_maxTransferAmount *big.Int, _minTransferAmount *big.Int) (*types.Transaction, error) {
	return _CantonBridge.Contract.UpdateLimits(&_CantonBridge.TransactOpts, _maxTransferAmount, _minTransferAmount)
}

// UpdateRelayer is a paid mutator transaction binding the contract method 0x8f83ab13.
//
// Solidity: function updateRelayer(address newRelayer) returns()
func (_CantonBridge *CantonBridgeTransactor) UpdateRelayer(opts *bind.TransactOpts, newRelayer common.Address) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "updateRelayer", newRelayer)
}

// UpdateRelayer is a paid mutator transaction binding the contract method 0x8f83ab13.
//
// Solidity: function updateRelayer(address newRelayer) returns()
func (_CantonBridge *CantonBridgeSession) UpdateRelayer(newRelayer common.Address) (*types.Transaction, error) {
	return _CantonBridge.Contract.UpdateRelayer(&_CantonBridge.TransactOpts, newRelayer)
}

// UpdateRelayer is a paid mutator transaction binding the contract method 0x8f83ab13.
//
// Solidity: function updateRelayer(address newRelayer) returns()
func (_CantonBridge *CantonBridgeTransactorSession) UpdateRelayer(newRelayer common.Address) (*types.Transaction, error) {
	return _CantonBridge.Contract.UpdateRelayer(&_CantonBridge.TransactOpts, newRelayer)
}

// WithdrawFromCanton is a paid mutator transaction binding the contract method 0x5a94d032.
//
// Solidity: function withdrawFromCanton(address token, address recipient, uint256 amount, uint256 nonce, bytes32 cantonTxHash) returns()
func (_CantonBridge *CantonBridgeTransactor) WithdrawFromCanton(opts *bind.TransactOpts, token common.Address, recipient common.Address, amount *big.Int, nonce *big.Int, cantonTxHash [32]byte) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "withdrawFromCanton", token, recipient, amount, nonce, cantonTxHash)
}

// WithdrawFromCanton is a paid mutator transaction binding the contract method 0x5a94d032.
//
// Solidity: function withdrawFromCanton(address token, address recipient, uint256 amount, uint256 nonce, bytes32 cantonTxHash) returns()
func (_CantonBridge *CantonBridgeSession) WithdrawFromCanton(token common.Address, recipient common.Address, amount *big.Int, nonce *big.Int, cantonTxHash [32]byte) (*types.Transaction, error) {
	return _CantonBridge.Contract.WithdrawFromCanton(&_CantonBridge.TransactOpts, token, recipient, amount, nonce, cantonTxHash)
}

// WithdrawFromCanton is a paid mutator transaction binding the contract method 0x5a94d032.
//
// Solidity: function withdrawFromCanton(address token, address recipient, uint256 amount, uint256 nonce, bytes32 cantonTxHash) returns()
func (_CantonBridge *CantonBridgeTransactorSession) WithdrawFromCanton(token common.Address, recipient common.Address, amount *big.Int, nonce *big.Int, cantonTxHash [32]byte) (*types.Transaction, error) {
	return _CantonBridge.Contract.WithdrawFromCanton(&_CantonBridge.TransactOpts, token, recipient, amount, nonce, cantonTxHash)
}

// CantonBridgeBridgeLimitsUpdatedIterator is returned from FilterBridgeLimitsUpdated and is used to iterate over the raw logs and unpacked data for BridgeLimitsUpdated events raised by the CantonBridge contract.
type CantonBridgeBridgeLimitsUpdatedIterator struct {
	Event *CantonBridgeBridgeLimitsUpdated // Event containing the contract specifics and raw log

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
func (it *CantonBridgeBridgeLimitsUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CantonBridgeBridgeLimitsUpdated)
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
		it.Event = new(CantonBridgeBridgeLimitsUpdated)
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
func (it *CantonBridgeBridgeLimitsUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CantonBridgeBridgeLimitsUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CantonBridgeBridgeLimitsUpdated represents a BridgeLimitsUpdated event raised by the CantonBridge contract.
type CantonBridgeBridgeLimitsUpdated struct {
	MaxTransferAmount *big.Int
	MinTransferAmount *big.Int
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterBridgeLimitsUpdated is a free log retrieval operation binding the contract event 0xcfc62648bfe6689bfdf06f50d3233f77d9e91b33d62e0b52350865779cd83151.
//
// Solidity: event BridgeLimitsUpdated(uint256 maxTransferAmount, uint256 minTransferAmount)
func (_CantonBridge *CantonBridgeFilterer) FilterBridgeLimitsUpdated(opts *bind.FilterOpts) (*CantonBridgeBridgeLimitsUpdatedIterator, error) {

	logs, sub, err := _CantonBridge.contract.FilterLogs(opts, "BridgeLimitsUpdated")
	if err != nil {
		return nil, err
	}
	return &CantonBridgeBridgeLimitsUpdatedIterator{contract: _CantonBridge.contract, event: "BridgeLimitsUpdated", logs: logs, sub: sub}, nil
}

// WatchBridgeLimitsUpdated is a free log subscription operation binding the contract event 0xcfc62648bfe6689bfdf06f50d3233f77d9e91b33d62e0b52350865779cd83151.
//
// Solidity: event BridgeLimitsUpdated(uint256 maxTransferAmount, uint256 minTransferAmount)
func (_CantonBridge *CantonBridgeFilterer) WatchBridgeLimitsUpdated(opts *bind.WatchOpts, sink chan<- *CantonBridgeBridgeLimitsUpdated) (event.Subscription, error) {

	logs, sub, err := _CantonBridge.contract.WatchLogs(opts, "BridgeLimitsUpdated")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CantonBridgeBridgeLimitsUpdated)
				if err := _CantonBridge.contract.UnpackLog(event, "BridgeLimitsUpdated", log); err != nil {
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

// ParseBridgeLimitsUpdated is a log parse operation binding the contract event 0xcfc62648bfe6689bfdf06f50d3233f77d9e91b33d62e0b52350865779cd83151.
//
// Solidity: event BridgeLimitsUpdated(uint256 maxTransferAmount, uint256 minTransferAmount)
func (_CantonBridge *CantonBridgeFilterer) ParseBridgeLimitsUpdated(log types.Log) (*CantonBridgeBridgeLimitsUpdated, error) {
	event := new(CantonBridgeBridgeLimitsUpdated)
	if err := _CantonBridge.contract.UnpackLog(event, "BridgeLimitsUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CantonBridgeDepositToCantonIterator is returned from FilterDepositToCanton and is used to iterate over the raw logs and unpacked data for DepositToCanton events raised by the CantonBridge contract.
type CantonBridgeDepositToCantonIterator struct {
	Event *CantonBridgeDepositToCanton // Event containing the contract specifics and raw log

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
func (it *CantonBridgeDepositToCantonIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CantonBridgeDepositToCanton)
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
		it.Event = new(CantonBridgeDepositToCanton)
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
func (it *CantonBridgeDepositToCantonIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CantonBridgeDepositToCantonIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CantonBridgeDepositToCanton represents a DepositToCanton event raised by the CantonBridge contract.
type CantonBridgeDepositToCanton struct {
	Token           common.Address
	Sender          common.Address
	CantonRecipient [32]byte
	Amount          *big.Int
	Nonce           *big.Int
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterDepositToCanton is a free log retrieval operation binding the contract event 0xeb81e98a4f9bc1a984adfd1d0fa704f53a91dca986f7198277bc93037cb7e8ba.
//
// Solidity: event DepositToCanton(address indexed token, address indexed sender, bytes32 indexed cantonRecipient, uint256 amount, uint256 nonce)
func (_CantonBridge *CantonBridgeFilterer) FilterDepositToCanton(opts *bind.FilterOpts, token []common.Address, sender []common.Address, cantonRecipient [][32]byte) (*CantonBridgeDepositToCantonIterator, error) {

	var tokenRule []interface{}
	for _, tokenItem := range token {
		tokenRule = append(tokenRule, tokenItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}
	var cantonRecipientRule []interface{}
	for _, cantonRecipientItem := range cantonRecipient {
		cantonRecipientRule = append(cantonRecipientRule, cantonRecipientItem)
	}

	logs, sub, err := _CantonBridge.contract.FilterLogs(opts, "DepositToCanton", tokenRule, senderRule, cantonRecipientRule)
	if err != nil {
		return nil, err
	}
	return &CantonBridgeDepositToCantonIterator{contract: _CantonBridge.contract, event: "DepositToCanton", logs: logs, sub: sub}, nil
}

// WatchDepositToCanton is a free log subscription operation binding the contract event 0xeb81e98a4f9bc1a984adfd1d0fa704f53a91dca986f7198277bc93037cb7e8ba.
//
// Solidity: event DepositToCanton(address indexed token, address indexed sender, bytes32 indexed cantonRecipient, uint256 amount, uint256 nonce)
func (_CantonBridge *CantonBridgeFilterer) WatchDepositToCanton(opts *bind.WatchOpts, sink chan<- *CantonBridgeDepositToCanton, token []common.Address, sender []common.Address, cantonRecipient [][32]byte) (event.Subscription, error) {

	var tokenRule []interface{}
	for _, tokenItem := range token {
		tokenRule = append(tokenRule, tokenItem)
	}
	var senderRule []interface{}
	for _, senderItem := range sender {
		senderRule = append(senderRule, senderItem)
	}
	var cantonRecipientRule []interface{}
	for _, cantonRecipientItem := range cantonRecipient {
		cantonRecipientRule = append(cantonRecipientRule, cantonRecipientItem)
	}

	logs, sub, err := _CantonBridge.contract.WatchLogs(opts, "DepositToCanton", tokenRule, senderRule, cantonRecipientRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CantonBridgeDepositToCanton)
				if err := _CantonBridge.contract.UnpackLog(event, "DepositToCanton", log); err != nil {
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

// ParseDepositToCanton is a log parse operation binding the contract event 0xeb81e98a4f9bc1a984adfd1d0fa704f53a91dca986f7198277bc93037cb7e8ba.
//
// Solidity: event DepositToCanton(address indexed token, address indexed sender, bytes32 indexed cantonRecipient, uint256 amount, uint256 nonce)
func (_CantonBridge *CantonBridgeFilterer) ParseDepositToCanton(log types.Log) (*CantonBridgeDepositToCanton, error) {
	event := new(CantonBridgeDepositToCanton)
	if err := _CantonBridge.contract.UnpackLog(event, "DepositToCanton", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CantonBridgeOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the CantonBridge contract.
type CantonBridgeOwnershipTransferredIterator struct {
	Event *CantonBridgeOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *CantonBridgeOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CantonBridgeOwnershipTransferred)
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
		it.Event = new(CantonBridgeOwnershipTransferred)
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
func (it *CantonBridgeOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CantonBridgeOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CantonBridgeOwnershipTransferred represents a OwnershipTransferred event raised by the CantonBridge contract.
type CantonBridgeOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_CantonBridge *CantonBridgeFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*CantonBridgeOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _CantonBridge.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &CantonBridgeOwnershipTransferredIterator{contract: _CantonBridge.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_CantonBridge *CantonBridgeFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *CantonBridgeOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _CantonBridge.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CantonBridgeOwnershipTransferred)
				if err := _CantonBridge.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_CantonBridge *CantonBridgeFilterer) ParseOwnershipTransferred(log types.Log) (*CantonBridgeOwnershipTransferred, error) {
	event := new(CantonBridgeOwnershipTransferred)
	if err := _CantonBridge.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CantonBridgePausedIterator is returned from FilterPaused and is used to iterate over the raw logs and unpacked data for Paused events raised by the CantonBridge contract.
type CantonBridgePausedIterator struct {
	Event *CantonBridgePaused // Event containing the contract specifics and raw log

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
func (it *CantonBridgePausedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CantonBridgePaused)
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
		it.Event = new(CantonBridgePaused)
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
func (it *CantonBridgePausedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CantonBridgePausedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CantonBridgePaused represents a Paused event raised by the CantonBridge contract.
type CantonBridgePaused struct {
	Account common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterPaused is a free log retrieval operation binding the contract event 0x62e78cea01bee320cd4e420270b5ea74000d11b0c9f74754ebdbfc544b05a258.
//
// Solidity: event Paused(address account)
func (_CantonBridge *CantonBridgeFilterer) FilterPaused(opts *bind.FilterOpts) (*CantonBridgePausedIterator, error) {

	logs, sub, err := _CantonBridge.contract.FilterLogs(opts, "Paused")
	if err != nil {
		return nil, err
	}
	return &CantonBridgePausedIterator{contract: _CantonBridge.contract, event: "Paused", logs: logs, sub: sub}, nil
}

// WatchPaused is a free log subscription operation binding the contract event 0x62e78cea01bee320cd4e420270b5ea74000d11b0c9f74754ebdbfc544b05a258.
//
// Solidity: event Paused(address account)
func (_CantonBridge *CantonBridgeFilterer) WatchPaused(opts *bind.WatchOpts, sink chan<- *CantonBridgePaused) (event.Subscription, error) {

	logs, sub, err := _CantonBridge.contract.WatchLogs(opts, "Paused")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CantonBridgePaused)
				if err := _CantonBridge.contract.UnpackLog(event, "Paused", log); err != nil {
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

// ParsePaused is a log parse operation binding the contract event 0x62e78cea01bee320cd4e420270b5ea74000d11b0c9f74754ebdbfc544b05a258.
//
// Solidity: event Paused(address account)
func (_CantonBridge *CantonBridgeFilterer) ParsePaused(log types.Log) (*CantonBridgePaused, error) {
	event := new(CantonBridgePaused)
	if err := _CantonBridge.contract.UnpackLog(event, "Paused", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CantonBridgeRelayerUpdatedIterator is returned from FilterRelayerUpdated and is used to iterate over the raw logs and unpacked data for RelayerUpdated events raised by the CantonBridge contract.
type CantonBridgeRelayerUpdatedIterator struct {
	Event *CantonBridgeRelayerUpdated // Event containing the contract specifics and raw log

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
func (it *CantonBridgeRelayerUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CantonBridgeRelayerUpdated)
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
		it.Event = new(CantonBridgeRelayerUpdated)
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
func (it *CantonBridgeRelayerUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CantonBridgeRelayerUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CantonBridgeRelayerUpdated represents a RelayerUpdated event raised by the CantonBridge contract.
type CantonBridgeRelayerUpdated struct {
	OldRelayer common.Address
	NewRelayer common.Address
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterRelayerUpdated is a free log retrieval operation binding the contract event 0x605ca4e43489fb38b91aa63dd9147cd3847957694b080b9285ec898b34269f0c.
//
// Solidity: event RelayerUpdated(address indexed oldRelayer, address indexed newRelayer)
func (_CantonBridge *CantonBridgeFilterer) FilterRelayerUpdated(opts *bind.FilterOpts, oldRelayer []common.Address, newRelayer []common.Address) (*CantonBridgeRelayerUpdatedIterator, error) {

	var oldRelayerRule []interface{}
	for _, oldRelayerItem := range oldRelayer {
		oldRelayerRule = append(oldRelayerRule, oldRelayerItem)
	}
	var newRelayerRule []interface{}
	for _, newRelayerItem := range newRelayer {
		newRelayerRule = append(newRelayerRule, newRelayerItem)
	}

	logs, sub, err := _CantonBridge.contract.FilterLogs(opts, "RelayerUpdated", oldRelayerRule, newRelayerRule)
	if err != nil {
		return nil, err
	}
	return &CantonBridgeRelayerUpdatedIterator{contract: _CantonBridge.contract, event: "RelayerUpdated", logs: logs, sub: sub}, nil
}

// WatchRelayerUpdated is a free log subscription operation binding the contract event 0x605ca4e43489fb38b91aa63dd9147cd3847957694b080b9285ec898b34269f0c.
//
// Solidity: event RelayerUpdated(address indexed oldRelayer, address indexed newRelayer)
func (_CantonBridge *CantonBridgeFilterer) WatchRelayerUpdated(opts *bind.WatchOpts, sink chan<- *CantonBridgeRelayerUpdated, oldRelayer []common.Address, newRelayer []common.Address) (event.Subscription, error) {

	var oldRelayerRule []interface{}
	for _, oldRelayerItem := range oldRelayer {
		oldRelayerRule = append(oldRelayerRule, oldRelayerItem)
	}
	var newRelayerRule []interface{}
	for _, newRelayerItem := range newRelayer {
		newRelayerRule = append(newRelayerRule, newRelayerItem)
	}

	logs, sub, err := _CantonBridge.contract.WatchLogs(opts, "RelayerUpdated", oldRelayerRule, newRelayerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CantonBridgeRelayerUpdated)
				if err := _CantonBridge.contract.UnpackLog(event, "RelayerUpdated", log); err != nil {
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

// ParseRelayerUpdated is a log parse operation binding the contract event 0x605ca4e43489fb38b91aa63dd9147cd3847957694b080b9285ec898b34269f0c.
//
// Solidity: event RelayerUpdated(address indexed oldRelayer, address indexed newRelayer)
func (_CantonBridge *CantonBridgeFilterer) ParseRelayerUpdated(log types.Log) (*CantonBridgeRelayerUpdated, error) {
	event := new(CantonBridgeRelayerUpdated)
	if err := _CantonBridge.contract.UnpackLog(event, "RelayerUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CantonBridgeTokenMappingAddedIterator is returned from FilterTokenMappingAdded and is used to iterate over the raw logs and unpacked data for TokenMappingAdded events raised by the CantonBridge contract.
type CantonBridgeTokenMappingAddedIterator struct {
	Event *CantonBridgeTokenMappingAdded // Event containing the contract specifics and raw log

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
func (it *CantonBridgeTokenMappingAddedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CantonBridgeTokenMappingAdded)
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
		it.Event = new(CantonBridgeTokenMappingAdded)
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
func (it *CantonBridgeTokenMappingAddedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CantonBridgeTokenMappingAddedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CantonBridgeTokenMappingAdded represents a TokenMappingAdded event raised by the CantonBridge contract.
type CantonBridgeTokenMappingAdded struct {
	EthereumToken common.Address
	CantonTokenId [32]byte
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterTokenMappingAdded is a free log retrieval operation binding the contract event 0x47d12254d1e957e8b2a7d8d640476c8cee321d3f095b3bc87b3c679f713f684b.
//
// Solidity: event TokenMappingAdded(address indexed ethereumToken, bytes32 indexed cantonTokenId)
func (_CantonBridge *CantonBridgeFilterer) FilterTokenMappingAdded(opts *bind.FilterOpts, ethereumToken []common.Address, cantonTokenId [][32]byte) (*CantonBridgeTokenMappingAddedIterator, error) {

	var ethereumTokenRule []interface{}
	for _, ethereumTokenItem := range ethereumToken {
		ethereumTokenRule = append(ethereumTokenRule, ethereumTokenItem)
	}
	var cantonTokenIdRule []interface{}
	for _, cantonTokenIdItem := range cantonTokenId {
		cantonTokenIdRule = append(cantonTokenIdRule, cantonTokenIdItem)
	}

	logs, sub, err := _CantonBridge.contract.FilterLogs(opts, "TokenMappingAdded", ethereumTokenRule, cantonTokenIdRule)
	if err != nil {
		return nil, err
	}
	return &CantonBridgeTokenMappingAddedIterator{contract: _CantonBridge.contract, event: "TokenMappingAdded", logs: logs, sub: sub}, nil
}

// WatchTokenMappingAdded is a free log subscription operation binding the contract event 0x47d12254d1e957e8b2a7d8d640476c8cee321d3f095b3bc87b3c679f713f684b.
//
// Solidity: event TokenMappingAdded(address indexed ethereumToken, bytes32 indexed cantonTokenId)
func (_CantonBridge *CantonBridgeFilterer) WatchTokenMappingAdded(opts *bind.WatchOpts, sink chan<- *CantonBridgeTokenMappingAdded, ethereumToken []common.Address, cantonTokenId [][32]byte) (event.Subscription, error) {

	var ethereumTokenRule []interface{}
	for _, ethereumTokenItem := range ethereumToken {
		ethereumTokenRule = append(ethereumTokenRule, ethereumTokenItem)
	}
	var cantonTokenIdRule []interface{}
	for _, cantonTokenIdItem := range cantonTokenId {
		cantonTokenIdRule = append(cantonTokenIdRule, cantonTokenIdItem)
	}

	logs, sub, err := _CantonBridge.contract.WatchLogs(opts, "TokenMappingAdded", ethereumTokenRule, cantonTokenIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CantonBridgeTokenMappingAdded)
				if err := _CantonBridge.contract.UnpackLog(event, "TokenMappingAdded", log); err != nil {
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

// ParseTokenMappingAdded is a log parse operation binding the contract event 0x47d12254d1e957e8b2a7d8d640476c8cee321d3f095b3bc87b3c679f713f684b.
//
// Solidity: event TokenMappingAdded(address indexed ethereumToken, bytes32 indexed cantonTokenId)
func (_CantonBridge *CantonBridgeFilterer) ParseTokenMappingAdded(log types.Log) (*CantonBridgeTokenMappingAdded, error) {
	event := new(CantonBridgeTokenMappingAdded)
	if err := _CantonBridge.contract.UnpackLog(event, "TokenMappingAdded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CantonBridgeTokenMappingRemovedIterator is returned from FilterTokenMappingRemoved and is used to iterate over the raw logs and unpacked data for TokenMappingRemoved events raised by the CantonBridge contract.
type CantonBridgeTokenMappingRemovedIterator struct {
	Event *CantonBridgeTokenMappingRemoved // Event containing the contract specifics and raw log

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
func (it *CantonBridgeTokenMappingRemovedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CantonBridgeTokenMappingRemoved)
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
		it.Event = new(CantonBridgeTokenMappingRemoved)
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
func (it *CantonBridgeTokenMappingRemovedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CantonBridgeTokenMappingRemovedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CantonBridgeTokenMappingRemoved represents a TokenMappingRemoved event raised by the CantonBridge contract.
type CantonBridgeTokenMappingRemoved struct {
	EthereumToken common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterTokenMappingRemoved is a free log retrieval operation binding the contract event 0x3cc70907a9d2be7ea4e6b746d9739cad4d110ebb6c622b34380fee32d6cbed70.
//
// Solidity: event TokenMappingRemoved(address indexed ethereumToken)
func (_CantonBridge *CantonBridgeFilterer) FilterTokenMappingRemoved(opts *bind.FilterOpts, ethereumToken []common.Address) (*CantonBridgeTokenMappingRemovedIterator, error) {

	var ethereumTokenRule []interface{}
	for _, ethereumTokenItem := range ethereumToken {
		ethereumTokenRule = append(ethereumTokenRule, ethereumTokenItem)
	}

	logs, sub, err := _CantonBridge.contract.FilterLogs(opts, "TokenMappingRemoved", ethereumTokenRule)
	if err != nil {
		return nil, err
	}
	return &CantonBridgeTokenMappingRemovedIterator{contract: _CantonBridge.contract, event: "TokenMappingRemoved", logs: logs, sub: sub}, nil
}

// WatchTokenMappingRemoved is a free log subscription operation binding the contract event 0x3cc70907a9d2be7ea4e6b746d9739cad4d110ebb6c622b34380fee32d6cbed70.
//
// Solidity: event TokenMappingRemoved(address indexed ethereumToken)
func (_CantonBridge *CantonBridgeFilterer) WatchTokenMappingRemoved(opts *bind.WatchOpts, sink chan<- *CantonBridgeTokenMappingRemoved, ethereumToken []common.Address) (event.Subscription, error) {

	var ethereumTokenRule []interface{}
	for _, ethereumTokenItem := range ethereumToken {
		ethereumTokenRule = append(ethereumTokenRule, ethereumTokenItem)
	}

	logs, sub, err := _CantonBridge.contract.WatchLogs(opts, "TokenMappingRemoved", ethereumTokenRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CantonBridgeTokenMappingRemoved)
				if err := _CantonBridge.contract.UnpackLog(event, "TokenMappingRemoved", log); err != nil {
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

// ParseTokenMappingRemoved is a log parse operation binding the contract event 0x3cc70907a9d2be7ea4e6b746d9739cad4d110ebb6c622b34380fee32d6cbed70.
//
// Solidity: event TokenMappingRemoved(address indexed ethereumToken)
func (_CantonBridge *CantonBridgeFilterer) ParseTokenMappingRemoved(log types.Log) (*CantonBridgeTokenMappingRemoved, error) {
	event := new(CantonBridgeTokenMappingRemoved)
	if err := _CantonBridge.contract.UnpackLog(event, "TokenMappingRemoved", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CantonBridgeUnpausedIterator is returned from FilterUnpaused and is used to iterate over the raw logs and unpacked data for Unpaused events raised by the CantonBridge contract.
type CantonBridgeUnpausedIterator struct {
	Event *CantonBridgeUnpaused // Event containing the contract specifics and raw log

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
func (it *CantonBridgeUnpausedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CantonBridgeUnpaused)
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
		it.Event = new(CantonBridgeUnpaused)
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
func (it *CantonBridgeUnpausedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CantonBridgeUnpausedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CantonBridgeUnpaused represents a Unpaused event raised by the CantonBridge contract.
type CantonBridgeUnpaused struct {
	Account common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterUnpaused is a free log retrieval operation binding the contract event 0x5db9ee0a495bf2e6ff9c91a7834c1ba4fdd244a5e8aa4e537bd38aeae4b073aa.
//
// Solidity: event Unpaused(address account)
func (_CantonBridge *CantonBridgeFilterer) FilterUnpaused(opts *bind.FilterOpts) (*CantonBridgeUnpausedIterator, error) {

	logs, sub, err := _CantonBridge.contract.FilterLogs(opts, "Unpaused")
	if err != nil {
		return nil, err
	}
	return &CantonBridgeUnpausedIterator{contract: _CantonBridge.contract, event: "Unpaused", logs: logs, sub: sub}, nil
}

// WatchUnpaused is a free log subscription operation binding the contract event 0x5db9ee0a495bf2e6ff9c91a7834c1ba4fdd244a5e8aa4e537bd38aeae4b073aa.
//
// Solidity: event Unpaused(address account)
func (_CantonBridge *CantonBridgeFilterer) WatchUnpaused(opts *bind.WatchOpts, sink chan<- *CantonBridgeUnpaused) (event.Subscription, error) {

	logs, sub, err := _CantonBridge.contract.WatchLogs(opts, "Unpaused")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CantonBridgeUnpaused)
				if err := _CantonBridge.contract.UnpackLog(event, "Unpaused", log); err != nil {
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

// ParseUnpaused is a log parse operation binding the contract event 0x5db9ee0a495bf2e6ff9c91a7834c1ba4fdd244a5e8aa4e537bd38aeae4b073aa.
//
// Solidity: event Unpaused(address account)
func (_CantonBridge *CantonBridgeFilterer) ParseUnpaused(log types.Log) (*CantonBridgeUnpaused, error) {
	event := new(CantonBridgeUnpaused)
	if err := _CantonBridge.contract.UnpackLog(event, "Unpaused", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// CantonBridgeWithdrawFromCantonIterator is returned from FilterWithdrawFromCanton and is used to iterate over the raw logs and unpacked data for WithdrawFromCanton events raised by the CantonBridge contract.
type CantonBridgeWithdrawFromCantonIterator struct {
	Event *CantonBridgeWithdrawFromCanton // Event containing the contract specifics and raw log

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
func (it *CantonBridgeWithdrawFromCantonIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(CantonBridgeWithdrawFromCanton)
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
		it.Event = new(CantonBridgeWithdrawFromCanton)
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
func (it *CantonBridgeWithdrawFromCantonIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *CantonBridgeWithdrawFromCantonIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// CantonBridgeWithdrawFromCanton represents a WithdrawFromCanton event raised by the CantonBridge contract.
type CantonBridgeWithdrawFromCanton struct {
	Token        common.Address
	Recipient    common.Address
	Amount       *big.Int
	Nonce        *big.Int
	CantonTxHash [32]byte
	Raw          types.Log // Blockchain specific contextual infos
}

// FilterWithdrawFromCanton is a free log retrieval operation binding the contract event 0x08f0eddc3c8a0bc5c0fb02dfcd2a51f46e8d93a0473f21c11715b6efce3380a0.
//
// Solidity: event WithdrawFromCanton(address indexed token, address indexed recipient, uint256 amount, uint256 nonce, bytes32 cantonTxHash)
func (_CantonBridge *CantonBridgeFilterer) FilterWithdrawFromCanton(opts *bind.FilterOpts, token []common.Address, recipient []common.Address) (*CantonBridgeWithdrawFromCantonIterator, error) {

	var tokenRule []interface{}
	for _, tokenItem := range token {
		tokenRule = append(tokenRule, tokenItem)
	}
	var recipientRule []interface{}
	for _, recipientItem := range recipient {
		recipientRule = append(recipientRule, recipientItem)
	}

	logs, sub, err := _CantonBridge.contract.FilterLogs(opts, "WithdrawFromCanton", tokenRule, recipientRule)
	if err != nil {
		return nil, err
	}
	return &CantonBridgeWithdrawFromCantonIterator{contract: _CantonBridge.contract, event: "WithdrawFromCanton", logs: logs, sub: sub}, nil
}

// WatchWithdrawFromCanton is a free log subscription operation binding the contract event 0x08f0eddc3c8a0bc5c0fb02dfcd2a51f46e8d93a0473f21c11715b6efce3380a0.
//
// Solidity: event WithdrawFromCanton(address indexed token, address indexed recipient, uint256 amount, uint256 nonce, bytes32 cantonTxHash)
func (_CantonBridge *CantonBridgeFilterer) WatchWithdrawFromCanton(opts *bind.WatchOpts, sink chan<- *CantonBridgeWithdrawFromCanton, token []common.Address, recipient []common.Address) (event.Subscription, error) {

	var tokenRule []interface{}
	for _, tokenItem := range token {
		tokenRule = append(tokenRule, tokenItem)
	}
	var recipientRule []interface{}
	for _, recipientItem := range recipient {
		recipientRule = append(recipientRule, recipientItem)
	}

	logs, sub, err := _CantonBridge.contract.WatchLogs(opts, "WithdrawFromCanton", tokenRule, recipientRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(CantonBridgeWithdrawFromCanton)
				if err := _CantonBridge.contract.UnpackLog(event, "WithdrawFromCanton", log); err != nil {
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

// ParseWithdrawFromCanton is a log parse operation binding the contract event 0x08f0eddc3c8a0bc5c0fb02dfcd2a51f46e8d93a0473f21c11715b6efce3380a0.
//
// Solidity: event WithdrawFromCanton(address indexed token, address indexed recipient, uint256 amount, uint256 nonce, bytes32 cantonTxHash)
func (_CantonBridge *CantonBridgeFilterer) ParseWithdrawFromCanton(log types.Log) (*CantonBridgeWithdrawFromCanton, error) {
	event := new(CantonBridgeWithdrawFromCanton)
	if err := _CantonBridge.contract.UnpackLog(event, "WithdrawFromCanton", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
