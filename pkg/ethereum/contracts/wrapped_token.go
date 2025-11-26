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

// WrappedTokenMetaData contains all meta data concerning the WrappedToken contract.
var WrappedTokenMetaData = &bind.MetaData{
	ABI: "[{\"inputs\":[{\"internalType\":\"string\",\"name\":\"name\",\"type\":\"string\"},{\"internalType\":\"string\",\"name\":\"symbol\",\"type\":\"string\"},{\"internalType\":\"bytes32\",\"name\":\"_cantonTokenId\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"bridge\",\"type\":\"address\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"AccessControlBadConfirmation\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"neededRole\",\"type\":\"bytes32\"}],\"name\":\"AccessControlUnauthorizedAccount\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"allowance\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"needed\",\"type\":\"uint256\"}],\"name\":\"ERC20InsufficientAllowance\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"balance\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"needed\",\"type\":\"uint256\"}],\"name\":\"ERC20InsufficientBalance\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"approver\",\"type\":\"address\"}],\"name\":\"ERC20InvalidApprover\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"receiver\",\"type\":\"address\"}],\"name\":\"ERC20InvalidReceiver\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"}],\"name\":\"ERC20InvalidSender\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"}],\"name\":\"ERC20InvalidSpender\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"Approval\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"role\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"previousAdminRole\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"newAdminRole\",\"type\":\"bytes32\"}],\"name\":\"RoleAdminChanged\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"role\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"}],\"name\":\"RoleGranted\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"role\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"}],\"name\":\"RoleRevoked\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"Transfer\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"BURNER_ROLE\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"DEFAULT_ADMIN_ROLE\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"MINTER_ROLE\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"}],\"name\":\"allowance\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"spender\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"approve\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"balanceOf\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"burn\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"burnFrom\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"cantonTokenId\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"decimals\",\"outputs\":[{\"internalType\":\"uint8\",\"name\":\"\",\"type\":\"uint8\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"role\",\"type\":\"bytes32\"}],\"name\":\"getRoleAdmin\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"role\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"grantRole\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"role\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"hasRole\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"mint\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"name\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"role\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"callerConfirmation\",\"type\":\"address\"}],\"name\":\"renounceRole\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"role\",\"type\":\"bytes32\"},{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"revokeRole\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes4\",\"name\":\"interfaceId\",\"type\":\"bytes4\"}],\"name\":\"supportsInterface\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"symbol\",\"outputs\":[{\"internalType\":\"string\",\"name\":\"\",\"type\":\"string\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"totalSupply\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"transfer\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"from\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"to\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"transferFrom\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x60806040523480156200001157600080fd5b5060405162001143380380620011438339810160408190526200003491620002f0565b8383600362000044838262000414565b50600462000053828262000414565b5050506001600160a01b038116620000b25760405162461bcd60e51b815260206004820152601660248201527f496e76616c69642062726964676520616464726573730000000000000000000060448201526064015b60405180910390fd5b81620001015760405162461bcd60e51b815260206004820152601760248201527f496e76616c69642043616e746f6e20746f6b656e2049440000000000000000006044820152606401620000a9565b60068290556200011360003362000178565b50620001407f9f2df0fed2c77648de5860a4cc508cd0818c85b8b8a1ab4ceeef8d981c8956a68262000178565b506200016d7f3c11d16cbaffd01df69ce1c404f6340ee057498f5f00246190ea54220576a8488262000178565b5050505050620004e0565b60008281526005602090815260408083206001600160a01b038516845290915281205460ff16620002215760008381526005602090815260408083206001600160a01b03861684529091529020805460ff19166001179055620001d83390565b6001600160a01b0316826001600160a01b0316847f2f8788117e7eff1d82e926ec794901d17c78024a50270940304540a733656f0d60405160405180910390a450600162000225565b5060005b92915050565b634e487b7160e01b600052604160045260246000fd5b600082601f8301126200025357600080fd5b81516001600160401b03808211156200027057620002706200022b565b604051601f8301601f19908116603f011681019082821181831017156200029b576200029b6200022b565b81604052838152602092508683858801011115620002b857600080fd5b600091505b83821015620002dc5785820183015181830184015290820190620002bd565b600093810190920192909252949350505050565b600080600080608085870312156200030757600080fd5b84516001600160401b03808211156200031f57600080fd5b6200032d8883890162000241565b955060208701519150808211156200034457600080fd5b50620003538782880162000241565b60408701516060880151919550935090506001600160a01b03811681146200037a57600080fd5b939692955090935050565b600181811c908216806200039a57607f821691505b602082108103620003bb57634e487b7160e01b600052602260045260246000fd5b50919050565b601f8211156200040f57600081815260208120601f850160051c81016020861015620003ea5750805b601f850160051c820191505b818110156200040b57828155600101620003f6565b5050505b505050565b81516001600160401b038111156200043057620004306200022b565b620004488162000441845462000385565b84620003c1565b602080601f831160018114620004805760008415620004675750858301515b600019600386901b1c1916600185901b1785556200040b565b600085815260208120601f198616915b82811015620004b15788860151825594840194600190910190840162000490565b5085821015620004d05787850151600019600388901b60f8161c191681555b5050505050600190811b01905550565b610c5380620004f06000396000f3fe608060405234801561001057600080fd5b50600436106101425760003560e01c806340c10f19116100b857806395d89b411161007c57806395d89b41146102bb578063a217fddf146102c3578063a9059cbb146102cb578063d5391393146102de578063d547741f14610305578063dd62ed3e1461031857600080fd5b806340c10f191461024657806342966c681461025957806370a082311461026c57806379cc67901461029557806391d14854146102a857600080fd5b806323b872dd1161010a57806323b872dd146101b2578063248a9ca3146101c5578063282c51f3146101e85780632f2ff15d1461020f578063313ce5671461022457806336568abe1461023357600080fd5b806301ffc9a71461014757806306fdde031461016f578063095ea7b31461018457806318160ddd146101975780631cbf9bbb146101a9575b600080fd5b61015a610155366004610a37565b610351565b60405190151581526020015b60405180910390f35b610177610388565b6040516101669190610a68565b61015a610192366004610ad2565b61041a565b6002545b604051908152602001610166565b61019b60065481565b61015a6101c0366004610afc565b610432565b61019b6101d3366004610b38565b60009081526005602052604090206001015490565b61019b7f3c11d16cbaffd01df69ce1c404f6340ee057498f5f00246190ea54220576a84881565b61022261021d366004610b51565b610456565b005b60405160128152602001610166565b610222610241366004610b51565b610481565b610222610254366004610ad2565b6104b9565b610222610267366004610b38565b6104ed565b61019b61027a366004610b7d565b6001600160a01b031660009081526020819052604090205490565b6102226102a3366004610ad2565b6104fa565b61015a6102b6366004610b51565b61052e565b610177610559565b61019b600081565b61015a6102d9366004610ad2565b610568565b61019b7f9f2df0fed2c77648de5860a4cc508cd0818c85b8b8a1ab4ceeef8d981c8956a681565b610222610313366004610b51565b610576565b61019b610326366004610b98565b6001600160a01b03918216600090815260016020908152604080832093909416825291909152205490565b60006001600160e01b03198216637965db0b60e01b148061038257506301ffc9a760e01b6001600160e01b03198316145b92915050565b60606003805461039790610bc2565b80601f01602080910402602001604051908101604052809291908181526020018280546103c390610bc2565b80156104105780601f106103e557610100808354040283529160200191610410565b820191906000526020600020905b8154815290600101906020018083116103f357829003601f168201915b5050505050905090565b60003361042881858561059b565b5060019392505050565b6000336104408582856105a8565b61044b858585610625565b506001949350505050565b60008281526005602052604090206001015461047181610684565b61047b838361068e565b50505050565b6001600160a01b03811633146104aa5760405163334bd91960e11b815260040160405180910390fd5b6104b48282610722565b505050565b7f9f2df0fed2c77648de5860a4cc508cd0818c85b8b8a1ab4ceeef8d981c8956a66104e381610684565b6104b4838361078f565b6104f733826107c9565b50565b7f3c11d16cbaffd01df69ce1c404f6340ee057498f5f00246190ea54220576a84861052481610684565b6104b483836107c9565b60009182526005602090815260408084206001600160a01b0393909316845291905290205460ff1690565b60606004805461039790610bc2565b600033610428818585610625565b60008281526005602052604090206001015461059181610684565b61047b8383610722565b6104b483838360016107ff565b6001600160a01b03838116600090815260016020908152604080832093861683529290522054600019811461047b578181101561061657604051637dc7a0d960e11b81526001600160a01b038416600482015260248101829052604481018390526064015b60405180910390fd5b61047b848484840360006107ff565b6001600160a01b03831661064f57604051634b637e8f60e11b81526000600482015260240161060d565b6001600160a01b0382166106795760405163ec442f0560e01b81526000600482015260240161060d565b6104b48383836108d4565b6104f781336109fe565b600061069a838361052e565b61071a5760008381526005602090815260408083206001600160a01b03861684529091529020805460ff191660011790556106d23390565b6001600160a01b0316826001600160a01b0316847f2f8788117e7eff1d82e926ec794901d17c78024a50270940304540a733656f0d60405160405180910390a4506001610382565b506000610382565b600061072e838361052e565b1561071a5760008381526005602090815260408083206001600160a01b0386168085529252808320805460ff1916905551339286917ff6391f5c32d9c69d2a47ea670b442974b53935d1edc7fd64eb21e047a839171b9190a4506001610382565b6001600160a01b0382166107b95760405163ec442f0560e01b81526000600482015260240161060d565b6107c5600083836108d4565b5050565b6001600160a01b0382166107f357604051634b637e8f60e11b81526000600482015260240161060d565b6107c5826000836108d4565b6001600160a01b0384166108295760405163e602df0560e01b81526000600482015260240161060d565b6001600160a01b03831661085357604051634a1406b160e11b81526000600482015260240161060d565b6001600160a01b038085166000908152600160209081526040808320938716835292905220829055801561047b57826001600160a01b0316846001600160a01b03167f8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925846040516108c691815260200190565b60405180910390a350505050565b6001600160a01b0383166108ff5780600260008282546108f49190610bfc565b909155506109719050565b6001600160a01b038316600090815260208190526040902054818110156109525760405163391434e360e21b81526001600160a01b0385166004820152602481018290526044810183905260640161060d565b6001600160a01b03841660009081526020819052604090209082900390555b6001600160a01b03821661098d576002805482900390556109ac565b6001600160a01b03821660009081526020819052604090208054820190555b816001600160a01b0316836001600160a01b03167fddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef836040516109f191815260200190565b60405180910390a3505050565b610a08828261052e565b6107c55760405163e2517d3f60e01b81526001600160a01b03821660048201526024810183905260440161060d565b600060208284031215610a4957600080fd5b81356001600160e01b031981168114610a6157600080fd5b9392505050565b600060208083528351808285015260005b81811015610a9557858101830151858201604001528201610a79565b506000604082860101526040601f19601f8301168501019250505092915050565b80356001600160a01b0381168114610acd57600080fd5b919050565b60008060408385031215610ae557600080fd5b610aee83610ab6565b946020939093013593505050565b600080600060608486031215610b1157600080fd5b610b1a84610ab6565b9250610b2860208501610ab6565b9150604084013590509250925092565b600060208284031215610b4a57600080fd5b5035919050565b60008060408385031215610b6457600080fd5b82359150610b7460208401610ab6565b90509250929050565b600060208284031215610b8f57600080fd5b610a6182610ab6565b60008060408385031215610bab57600080fd5b610bb483610ab6565b9150610b7460208401610ab6565b600181811c90821680610bd657607f821691505b602082108103610bf657634e487b7160e01b600052602260045260246000fd5b50919050565b8082018082111561038257634e487b7160e01b600052601160045260246000fdfea264697066735822122060427667d1ba0d8fe2e0335962340459c90f1b5ef66a2e60c2bef7a0a3efe99364736f6c63430008140033",
}

// WrappedTokenABI is the input ABI used to generate the binding from.
// Deprecated: Use WrappedTokenMetaData.ABI instead.
var WrappedTokenABI = WrappedTokenMetaData.ABI

// WrappedTokenBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use WrappedTokenMetaData.Bin instead.
var WrappedTokenBin = WrappedTokenMetaData.Bin

// DeployWrappedToken deploys a new Ethereum contract, binding an instance of WrappedToken to it.
func DeployWrappedToken(auth *bind.TransactOpts, backend bind.ContractBackend, name string, symbol string, _cantonTokenId [32]byte, bridge common.Address) (common.Address, *types.Transaction, *WrappedToken, error) {
	parsed, err := WrappedTokenMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(WrappedTokenBin), backend, name, symbol, _cantonTokenId, bridge)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &WrappedToken{WrappedTokenCaller: WrappedTokenCaller{contract: contract}, WrappedTokenTransactor: WrappedTokenTransactor{contract: contract}, WrappedTokenFilterer: WrappedTokenFilterer{contract: contract}}, nil
}

// WrappedToken is an auto generated Go binding around an Ethereum contract.
type WrappedToken struct {
	WrappedTokenCaller     // Read-only binding to the contract
	WrappedTokenTransactor // Write-only binding to the contract
	WrappedTokenFilterer   // Log filterer for contract events
}

// WrappedTokenCaller is an auto generated read-only Go binding around an Ethereum contract.
type WrappedTokenCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// WrappedTokenTransactor is an auto generated write-only Go binding around an Ethereum contract.
type WrappedTokenTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// WrappedTokenFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type WrappedTokenFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// WrappedTokenSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type WrappedTokenSession struct {
	Contract     *WrappedToken     // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// WrappedTokenCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type WrappedTokenCallerSession struct {
	Contract *WrappedTokenCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts       // Call options to use throughout this session
}

// WrappedTokenTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type WrappedTokenTransactorSession struct {
	Contract     *WrappedTokenTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts       // Transaction auth options to use throughout this session
}

// WrappedTokenRaw is an auto generated low-level Go binding around an Ethereum contract.
type WrappedTokenRaw struct {
	Contract *WrappedToken // Generic contract binding to access the raw methods on
}

// WrappedTokenCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type WrappedTokenCallerRaw struct {
	Contract *WrappedTokenCaller // Generic read-only contract binding to access the raw methods on
}

// WrappedTokenTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type WrappedTokenTransactorRaw struct {
	Contract *WrappedTokenTransactor // Generic write-only contract binding to access the raw methods on
}

// NewWrappedToken creates a new instance of WrappedToken, bound to a specific deployed contract.
func NewWrappedToken(address common.Address, backend bind.ContractBackend) (*WrappedToken, error) {
	contract, err := bindWrappedToken(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &WrappedToken{WrappedTokenCaller: WrappedTokenCaller{contract: contract}, WrappedTokenTransactor: WrappedTokenTransactor{contract: contract}, WrappedTokenFilterer: WrappedTokenFilterer{contract: contract}}, nil
}

// NewWrappedTokenCaller creates a new read-only instance of WrappedToken, bound to a specific deployed contract.
func NewWrappedTokenCaller(address common.Address, caller bind.ContractCaller) (*WrappedTokenCaller, error) {
	contract, err := bindWrappedToken(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &WrappedTokenCaller{contract: contract}, nil
}

// NewWrappedTokenTransactor creates a new write-only instance of WrappedToken, bound to a specific deployed contract.
func NewWrappedTokenTransactor(address common.Address, transactor bind.ContractTransactor) (*WrappedTokenTransactor, error) {
	contract, err := bindWrappedToken(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &WrappedTokenTransactor{contract: contract}, nil
}

// NewWrappedTokenFilterer creates a new log filterer instance of WrappedToken, bound to a specific deployed contract.
func NewWrappedTokenFilterer(address common.Address, filterer bind.ContractFilterer) (*WrappedTokenFilterer, error) {
	contract, err := bindWrappedToken(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &WrappedTokenFilterer{contract: contract}, nil
}

// bindWrappedToken binds a generic wrapper to an already deployed contract.
func bindWrappedToken(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := WrappedTokenMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_WrappedToken *WrappedTokenRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _WrappedToken.Contract.WrappedTokenCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_WrappedToken *WrappedTokenRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _WrappedToken.Contract.WrappedTokenTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_WrappedToken *WrappedTokenRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _WrappedToken.Contract.WrappedTokenTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_WrappedToken *WrappedTokenCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _WrappedToken.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_WrappedToken *WrappedTokenTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _WrappedToken.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_WrappedToken *WrappedTokenTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _WrappedToken.Contract.contract.Transact(opts, method, params...)
}

// BURNERROLE is a free data retrieval call binding the contract method 0x282c51f3.
//
// Solidity: function BURNER_ROLE() view returns(bytes32)
func (_WrappedToken *WrappedTokenCaller) BURNERROLE(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "BURNER_ROLE")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// BURNERROLE is a free data retrieval call binding the contract method 0x282c51f3.
//
// Solidity: function BURNER_ROLE() view returns(bytes32)
func (_WrappedToken *WrappedTokenSession) BURNERROLE() ([32]byte, error) {
	return _WrappedToken.Contract.BURNERROLE(&_WrappedToken.CallOpts)
}

// BURNERROLE is a free data retrieval call binding the contract method 0x282c51f3.
//
// Solidity: function BURNER_ROLE() view returns(bytes32)
func (_WrappedToken *WrappedTokenCallerSession) BURNERROLE() ([32]byte, error) {
	return _WrappedToken.Contract.BURNERROLE(&_WrappedToken.CallOpts)
}

// DEFAULTADMINROLE is a free data retrieval call binding the contract method 0xa217fddf.
//
// Solidity: function DEFAULT_ADMIN_ROLE() view returns(bytes32)
func (_WrappedToken *WrappedTokenCaller) DEFAULTADMINROLE(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "DEFAULT_ADMIN_ROLE")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// DEFAULTADMINROLE is a free data retrieval call binding the contract method 0xa217fddf.
//
// Solidity: function DEFAULT_ADMIN_ROLE() view returns(bytes32)
func (_WrappedToken *WrappedTokenSession) DEFAULTADMINROLE() ([32]byte, error) {
	return _WrappedToken.Contract.DEFAULTADMINROLE(&_WrappedToken.CallOpts)
}

// DEFAULTADMINROLE is a free data retrieval call binding the contract method 0xa217fddf.
//
// Solidity: function DEFAULT_ADMIN_ROLE() view returns(bytes32)
func (_WrappedToken *WrappedTokenCallerSession) DEFAULTADMINROLE() ([32]byte, error) {
	return _WrappedToken.Contract.DEFAULTADMINROLE(&_WrappedToken.CallOpts)
}

// MINTERROLE is a free data retrieval call binding the contract method 0xd5391393.
//
// Solidity: function MINTER_ROLE() view returns(bytes32)
func (_WrappedToken *WrappedTokenCaller) MINTERROLE(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "MINTER_ROLE")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// MINTERROLE is a free data retrieval call binding the contract method 0xd5391393.
//
// Solidity: function MINTER_ROLE() view returns(bytes32)
func (_WrappedToken *WrappedTokenSession) MINTERROLE() ([32]byte, error) {
	return _WrappedToken.Contract.MINTERROLE(&_WrappedToken.CallOpts)
}

// MINTERROLE is a free data retrieval call binding the contract method 0xd5391393.
//
// Solidity: function MINTER_ROLE() view returns(bytes32)
func (_WrappedToken *WrappedTokenCallerSession) MINTERROLE() ([32]byte, error) {
	return _WrappedToken.Contract.MINTERROLE(&_WrappedToken.CallOpts)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address owner, address spender) view returns(uint256)
func (_WrappedToken *WrappedTokenCaller) Allowance(opts *bind.CallOpts, owner common.Address, spender common.Address) (*big.Int, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "allowance", owner, spender)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address owner, address spender) view returns(uint256)
func (_WrappedToken *WrappedTokenSession) Allowance(owner common.Address, spender common.Address) (*big.Int, error) {
	return _WrappedToken.Contract.Allowance(&_WrappedToken.CallOpts, owner, spender)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(address owner, address spender) view returns(uint256)
func (_WrappedToken *WrappedTokenCallerSession) Allowance(owner common.Address, spender common.Address) (*big.Int, error) {
	return _WrappedToken.Contract.Allowance(&_WrappedToken.CallOpts, owner, spender)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address account) view returns(uint256)
func (_WrappedToken *WrappedTokenCaller) BalanceOf(opts *bind.CallOpts, account common.Address) (*big.Int, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "balanceOf", account)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address account) view returns(uint256)
func (_WrappedToken *WrappedTokenSession) BalanceOf(account common.Address) (*big.Int, error) {
	return _WrappedToken.Contract.BalanceOf(&_WrappedToken.CallOpts, account)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(address account) view returns(uint256)
func (_WrappedToken *WrappedTokenCallerSession) BalanceOf(account common.Address) (*big.Int, error) {
	return _WrappedToken.Contract.BalanceOf(&_WrappedToken.CallOpts, account)
}

// CantonTokenId is a free data retrieval call binding the contract method 0x1cbf9bbb.
//
// Solidity: function cantonTokenId() view returns(bytes32)
func (_WrappedToken *WrappedTokenCaller) CantonTokenId(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "cantonTokenId")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// CantonTokenId is a free data retrieval call binding the contract method 0x1cbf9bbb.
//
// Solidity: function cantonTokenId() view returns(bytes32)
func (_WrappedToken *WrappedTokenSession) CantonTokenId() ([32]byte, error) {
	return _WrappedToken.Contract.CantonTokenId(&_WrappedToken.CallOpts)
}

// CantonTokenId is a free data retrieval call binding the contract method 0x1cbf9bbb.
//
// Solidity: function cantonTokenId() view returns(bytes32)
func (_WrappedToken *WrappedTokenCallerSession) CantonTokenId() ([32]byte, error) {
	return _WrappedToken.Contract.CantonTokenId(&_WrappedToken.CallOpts)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_WrappedToken *WrappedTokenCaller) Decimals(opts *bind.CallOpts) (uint8, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "decimals")

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_WrappedToken *WrappedTokenSession) Decimals() (uint8, error) {
	return _WrappedToken.Contract.Decimals(&_WrappedToken.CallOpts)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() view returns(uint8)
func (_WrappedToken *WrappedTokenCallerSession) Decimals() (uint8, error) {
	return _WrappedToken.Contract.Decimals(&_WrappedToken.CallOpts)
}

// GetRoleAdmin is a free data retrieval call binding the contract method 0x248a9ca3.
//
// Solidity: function getRoleAdmin(bytes32 role) view returns(bytes32)
func (_WrappedToken *WrappedTokenCaller) GetRoleAdmin(opts *bind.CallOpts, role [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "getRoleAdmin", role)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// GetRoleAdmin is a free data retrieval call binding the contract method 0x248a9ca3.
//
// Solidity: function getRoleAdmin(bytes32 role) view returns(bytes32)
func (_WrappedToken *WrappedTokenSession) GetRoleAdmin(role [32]byte) ([32]byte, error) {
	return _WrappedToken.Contract.GetRoleAdmin(&_WrappedToken.CallOpts, role)
}

// GetRoleAdmin is a free data retrieval call binding the contract method 0x248a9ca3.
//
// Solidity: function getRoleAdmin(bytes32 role) view returns(bytes32)
func (_WrappedToken *WrappedTokenCallerSession) GetRoleAdmin(role [32]byte) ([32]byte, error) {
	return _WrappedToken.Contract.GetRoleAdmin(&_WrappedToken.CallOpts, role)
}

// HasRole is a free data retrieval call binding the contract method 0x91d14854.
//
// Solidity: function hasRole(bytes32 role, address account) view returns(bool)
func (_WrappedToken *WrappedTokenCaller) HasRole(opts *bind.CallOpts, role [32]byte, account common.Address) (bool, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "hasRole", role, account)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// HasRole is a free data retrieval call binding the contract method 0x91d14854.
//
// Solidity: function hasRole(bytes32 role, address account) view returns(bool)
func (_WrappedToken *WrappedTokenSession) HasRole(role [32]byte, account common.Address) (bool, error) {
	return _WrappedToken.Contract.HasRole(&_WrappedToken.CallOpts, role, account)
}

// HasRole is a free data retrieval call binding the contract method 0x91d14854.
//
// Solidity: function hasRole(bytes32 role, address account) view returns(bool)
func (_WrappedToken *WrappedTokenCallerSession) HasRole(role [32]byte, account common.Address) (bool, error) {
	return _WrappedToken.Contract.HasRole(&_WrappedToken.CallOpts, role, account)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_WrappedToken *WrappedTokenCaller) Name(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "name")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_WrappedToken *WrappedTokenSession) Name() (string, error) {
	return _WrappedToken.Contract.Name(&_WrappedToken.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() view returns(string)
func (_WrappedToken *WrappedTokenCallerSession) Name() (string, error) {
	return _WrappedToken.Contract.Name(&_WrappedToken.CallOpts)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_WrappedToken *WrappedTokenCaller) SupportsInterface(opts *bind.CallOpts, interfaceId [4]byte) (bool, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "supportsInterface", interfaceId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_WrappedToken *WrappedTokenSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _WrappedToken.Contract.SupportsInterface(&_WrappedToken.CallOpts, interfaceId)
}

// SupportsInterface is a free data retrieval call binding the contract method 0x01ffc9a7.
//
// Solidity: function supportsInterface(bytes4 interfaceId) view returns(bool)
func (_WrappedToken *WrappedTokenCallerSession) SupportsInterface(interfaceId [4]byte) (bool, error) {
	return _WrappedToken.Contract.SupportsInterface(&_WrappedToken.CallOpts, interfaceId)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_WrappedToken *WrappedTokenCaller) Symbol(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "symbol")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_WrappedToken *WrappedTokenSession) Symbol() (string, error) {
	return _WrappedToken.Contract.Symbol(&_WrappedToken.CallOpts)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() view returns(string)
func (_WrappedToken *WrappedTokenCallerSession) Symbol() (string, error) {
	return _WrappedToken.Contract.Symbol(&_WrappedToken.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_WrappedToken *WrappedTokenCaller) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _WrappedToken.contract.Call(opts, &out, "totalSupply")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_WrappedToken *WrappedTokenSession) TotalSupply() (*big.Int, error) {
	return _WrappedToken.Contract.TotalSupply(&_WrappedToken.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() view returns(uint256)
func (_WrappedToken *WrappedTokenCallerSession) TotalSupply() (*big.Int, error) {
	return _WrappedToken.Contract.TotalSupply(&_WrappedToken.CallOpts)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 value) returns(bool)
func (_WrappedToken *WrappedTokenTransactor) Approve(opts *bind.TransactOpts, spender common.Address, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.contract.Transact(opts, "approve", spender, value)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 value) returns(bool)
func (_WrappedToken *WrappedTokenSession) Approve(spender common.Address, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.Approve(&_WrappedToken.TransactOpts, spender, value)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(address spender, uint256 value) returns(bool)
func (_WrappedToken *WrappedTokenTransactorSession) Approve(spender common.Address, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.Approve(&_WrappedToken.TransactOpts, spender, value)
}

// Burn is a paid mutator transaction binding the contract method 0x42966c68.
//
// Solidity: function burn(uint256 value) returns()
func (_WrappedToken *WrappedTokenTransactor) Burn(opts *bind.TransactOpts, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.contract.Transact(opts, "burn", value)
}

// Burn is a paid mutator transaction binding the contract method 0x42966c68.
//
// Solidity: function burn(uint256 value) returns()
func (_WrappedToken *WrappedTokenSession) Burn(value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.Burn(&_WrappedToken.TransactOpts, value)
}

// Burn is a paid mutator transaction binding the contract method 0x42966c68.
//
// Solidity: function burn(uint256 value) returns()
func (_WrappedToken *WrappedTokenTransactorSession) Burn(value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.Burn(&_WrappedToken.TransactOpts, value)
}

// BurnFrom is a paid mutator transaction binding the contract method 0x79cc6790.
//
// Solidity: function burnFrom(address from, uint256 amount) returns()
func (_WrappedToken *WrappedTokenTransactor) BurnFrom(opts *bind.TransactOpts, from common.Address, amount *big.Int) (*types.Transaction, error) {
	return _WrappedToken.contract.Transact(opts, "burnFrom", from, amount)
}

// BurnFrom is a paid mutator transaction binding the contract method 0x79cc6790.
//
// Solidity: function burnFrom(address from, uint256 amount) returns()
func (_WrappedToken *WrappedTokenSession) BurnFrom(from common.Address, amount *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.BurnFrom(&_WrappedToken.TransactOpts, from, amount)
}

// BurnFrom is a paid mutator transaction binding the contract method 0x79cc6790.
//
// Solidity: function burnFrom(address from, uint256 amount) returns()
func (_WrappedToken *WrappedTokenTransactorSession) BurnFrom(from common.Address, amount *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.BurnFrom(&_WrappedToken.TransactOpts, from, amount)
}

// GrantRole is a paid mutator transaction binding the contract method 0x2f2ff15d.
//
// Solidity: function grantRole(bytes32 role, address account) returns()
func (_WrappedToken *WrappedTokenTransactor) GrantRole(opts *bind.TransactOpts, role [32]byte, account common.Address) (*types.Transaction, error) {
	return _WrappedToken.contract.Transact(opts, "grantRole", role, account)
}

// GrantRole is a paid mutator transaction binding the contract method 0x2f2ff15d.
//
// Solidity: function grantRole(bytes32 role, address account) returns()
func (_WrappedToken *WrappedTokenSession) GrantRole(role [32]byte, account common.Address) (*types.Transaction, error) {
	return _WrappedToken.Contract.GrantRole(&_WrappedToken.TransactOpts, role, account)
}

// GrantRole is a paid mutator transaction binding the contract method 0x2f2ff15d.
//
// Solidity: function grantRole(bytes32 role, address account) returns()
func (_WrappedToken *WrappedTokenTransactorSession) GrantRole(role [32]byte, account common.Address) (*types.Transaction, error) {
	return _WrappedToken.Contract.GrantRole(&_WrappedToken.TransactOpts, role, account)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_WrappedToken *WrappedTokenTransactor) Mint(opts *bind.TransactOpts, to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _WrappedToken.contract.Transact(opts, "mint", to, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_WrappedToken *WrappedTokenSession) Mint(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.Mint(&_WrappedToken.TransactOpts, to, amount)
}

// Mint is a paid mutator transaction binding the contract method 0x40c10f19.
//
// Solidity: function mint(address to, uint256 amount) returns()
func (_WrappedToken *WrappedTokenTransactorSession) Mint(to common.Address, amount *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.Mint(&_WrappedToken.TransactOpts, to, amount)
}

// RenounceRole is a paid mutator transaction binding the contract method 0x36568abe.
//
// Solidity: function renounceRole(bytes32 role, address callerConfirmation) returns()
func (_WrappedToken *WrappedTokenTransactor) RenounceRole(opts *bind.TransactOpts, role [32]byte, callerConfirmation common.Address) (*types.Transaction, error) {
	return _WrappedToken.contract.Transact(opts, "renounceRole", role, callerConfirmation)
}

// RenounceRole is a paid mutator transaction binding the contract method 0x36568abe.
//
// Solidity: function renounceRole(bytes32 role, address callerConfirmation) returns()
func (_WrappedToken *WrappedTokenSession) RenounceRole(role [32]byte, callerConfirmation common.Address) (*types.Transaction, error) {
	return _WrappedToken.Contract.RenounceRole(&_WrappedToken.TransactOpts, role, callerConfirmation)
}

// RenounceRole is a paid mutator transaction binding the contract method 0x36568abe.
//
// Solidity: function renounceRole(bytes32 role, address callerConfirmation) returns()
func (_WrappedToken *WrappedTokenTransactorSession) RenounceRole(role [32]byte, callerConfirmation common.Address) (*types.Transaction, error) {
	return _WrappedToken.Contract.RenounceRole(&_WrappedToken.TransactOpts, role, callerConfirmation)
}

// RevokeRole is a paid mutator transaction binding the contract method 0xd547741f.
//
// Solidity: function revokeRole(bytes32 role, address account) returns()
func (_WrappedToken *WrappedTokenTransactor) RevokeRole(opts *bind.TransactOpts, role [32]byte, account common.Address) (*types.Transaction, error) {
	return _WrappedToken.contract.Transact(opts, "revokeRole", role, account)
}

// RevokeRole is a paid mutator transaction binding the contract method 0xd547741f.
//
// Solidity: function revokeRole(bytes32 role, address account) returns()
func (_WrappedToken *WrappedTokenSession) RevokeRole(role [32]byte, account common.Address) (*types.Transaction, error) {
	return _WrappedToken.Contract.RevokeRole(&_WrappedToken.TransactOpts, role, account)
}

// RevokeRole is a paid mutator transaction binding the contract method 0xd547741f.
//
// Solidity: function revokeRole(bytes32 role, address account) returns()
func (_WrappedToken *WrappedTokenTransactorSession) RevokeRole(role [32]byte, account common.Address) (*types.Transaction, error) {
	return _WrappedToken.Contract.RevokeRole(&_WrappedToken.TransactOpts, role, account)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 value) returns(bool)
func (_WrappedToken *WrappedTokenTransactor) Transfer(opts *bind.TransactOpts, to common.Address, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.contract.Transact(opts, "transfer", to, value)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 value) returns(bool)
func (_WrappedToken *WrappedTokenSession) Transfer(to common.Address, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.Transfer(&_WrappedToken.TransactOpts, to, value)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(address to, uint256 value) returns(bool)
func (_WrappedToken *WrappedTokenTransactorSession) Transfer(to common.Address, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.Transfer(&_WrappedToken.TransactOpts, to, value)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 value) returns(bool)
func (_WrappedToken *WrappedTokenTransactor) TransferFrom(opts *bind.TransactOpts, from common.Address, to common.Address, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.contract.Transact(opts, "transferFrom", from, to, value)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 value) returns(bool)
func (_WrappedToken *WrappedTokenSession) TransferFrom(from common.Address, to common.Address, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.TransferFrom(&_WrappedToken.TransactOpts, from, to, value)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(address from, address to, uint256 value) returns(bool)
func (_WrappedToken *WrappedTokenTransactorSession) TransferFrom(from common.Address, to common.Address, value *big.Int) (*types.Transaction, error) {
	return _WrappedToken.Contract.TransferFrom(&_WrappedToken.TransactOpts, from, to, value)
}

// WrappedTokenApprovalIterator is returned from FilterApproval and is used to iterate over the raw logs and unpacked data for Approval events raised by the WrappedToken contract.
type WrappedTokenApprovalIterator struct {
	Event *WrappedTokenApproval // Event containing the contract specifics and raw log

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
func (it *WrappedTokenApprovalIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(WrappedTokenApproval)
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
		it.Event = new(WrappedTokenApproval)
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
func (it *WrappedTokenApprovalIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *WrappedTokenApprovalIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// WrappedTokenApproval represents a Approval event raised by the WrappedToken contract.
type WrappedTokenApproval struct {
	Owner   common.Address
	Spender common.Address
	Value   *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterApproval is a free log retrieval operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_WrappedToken *WrappedTokenFilterer) FilterApproval(opts *bind.FilterOpts, owner []common.Address, spender []common.Address) (*WrappedTokenApprovalIterator, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var spenderRule []interface{}
	for _, spenderItem := range spender {
		spenderRule = append(spenderRule, spenderItem)
	}

	logs, sub, err := _WrappedToken.contract.FilterLogs(opts, "Approval", ownerRule, spenderRule)
	if err != nil {
		return nil, err
	}
	return &WrappedTokenApprovalIterator{contract: _WrappedToken.contract, event: "Approval", logs: logs, sub: sub}, nil
}

// WatchApproval is a free log subscription operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(address indexed owner, address indexed spender, uint256 value)
func (_WrappedToken *WrappedTokenFilterer) WatchApproval(opts *bind.WatchOpts, sink chan<- *WrappedTokenApproval, owner []common.Address, spender []common.Address) (event.Subscription, error) {

	var ownerRule []interface{}
	for _, ownerItem := range owner {
		ownerRule = append(ownerRule, ownerItem)
	}
	var spenderRule []interface{}
	for _, spenderItem := range spender {
		spenderRule = append(spenderRule, spenderItem)
	}

	logs, sub, err := _WrappedToken.contract.WatchLogs(opts, "Approval", ownerRule, spenderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(WrappedTokenApproval)
				if err := _WrappedToken.contract.UnpackLog(event, "Approval", log); err != nil {
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
func (_WrappedToken *WrappedTokenFilterer) ParseApproval(log types.Log) (*WrappedTokenApproval, error) {
	event := new(WrappedTokenApproval)
	if err := _WrappedToken.contract.UnpackLog(event, "Approval", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// WrappedTokenRoleAdminChangedIterator is returned from FilterRoleAdminChanged and is used to iterate over the raw logs and unpacked data for RoleAdminChanged events raised by the WrappedToken contract.
type WrappedTokenRoleAdminChangedIterator struct {
	Event *WrappedTokenRoleAdminChanged // Event containing the contract specifics and raw log

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
func (it *WrappedTokenRoleAdminChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(WrappedTokenRoleAdminChanged)
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
		it.Event = new(WrappedTokenRoleAdminChanged)
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
func (it *WrappedTokenRoleAdminChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *WrappedTokenRoleAdminChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// WrappedTokenRoleAdminChanged represents a RoleAdminChanged event raised by the WrappedToken contract.
type WrappedTokenRoleAdminChanged struct {
	Role              [32]byte
	PreviousAdminRole [32]byte
	NewAdminRole      [32]byte
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterRoleAdminChanged is a free log retrieval operation binding the contract event 0xbd79b86ffe0ab8e8776151514217cd7cacd52c909f66475c3af44e129f0b00ff.
//
// Solidity: event RoleAdminChanged(bytes32 indexed role, bytes32 indexed previousAdminRole, bytes32 indexed newAdminRole)
func (_WrappedToken *WrappedTokenFilterer) FilterRoleAdminChanged(opts *bind.FilterOpts, role [][32]byte, previousAdminRole [][32]byte, newAdminRole [][32]byte) (*WrappedTokenRoleAdminChangedIterator, error) {

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

	logs, sub, err := _WrappedToken.contract.FilterLogs(opts, "RoleAdminChanged", roleRule, previousAdminRoleRule, newAdminRoleRule)
	if err != nil {
		return nil, err
	}
	return &WrappedTokenRoleAdminChangedIterator{contract: _WrappedToken.contract, event: "RoleAdminChanged", logs: logs, sub: sub}, nil
}

// WatchRoleAdminChanged is a free log subscription operation binding the contract event 0xbd79b86ffe0ab8e8776151514217cd7cacd52c909f66475c3af44e129f0b00ff.
//
// Solidity: event RoleAdminChanged(bytes32 indexed role, bytes32 indexed previousAdminRole, bytes32 indexed newAdminRole)
func (_WrappedToken *WrappedTokenFilterer) WatchRoleAdminChanged(opts *bind.WatchOpts, sink chan<- *WrappedTokenRoleAdminChanged, role [][32]byte, previousAdminRole [][32]byte, newAdminRole [][32]byte) (event.Subscription, error) {

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

	logs, sub, err := _WrappedToken.contract.WatchLogs(opts, "RoleAdminChanged", roleRule, previousAdminRoleRule, newAdminRoleRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(WrappedTokenRoleAdminChanged)
				if err := _WrappedToken.contract.UnpackLog(event, "RoleAdminChanged", log); err != nil {
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
func (_WrappedToken *WrappedTokenFilterer) ParseRoleAdminChanged(log types.Log) (*WrappedTokenRoleAdminChanged, error) {
	event := new(WrappedTokenRoleAdminChanged)
	if err := _WrappedToken.contract.UnpackLog(event, "RoleAdminChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// WrappedTokenRoleGrantedIterator is returned from FilterRoleGranted and is used to iterate over the raw logs and unpacked data for RoleGranted events raised by the WrappedToken contract.
type WrappedTokenRoleGrantedIterator struct {
	Event *WrappedTokenRoleGranted // Event containing the contract specifics and raw log

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
func (it *WrappedTokenRoleGrantedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(WrappedTokenRoleGranted)
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
		it.Event = new(WrappedTokenRoleGranted)
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
func (it *WrappedTokenRoleGrantedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *WrappedTokenRoleGrantedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// WrappedTokenRoleGranted represents a RoleGranted event raised by the WrappedToken contract.
type WrappedTokenRoleGranted struct {
	Role    [32]byte
	Account common.Address
	Sender  common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterRoleGranted is a free log retrieval operation binding the contract event 0x2f8788117e7eff1d82e926ec794901d17c78024a50270940304540a733656f0d.
//
// Solidity: event RoleGranted(bytes32 indexed role, address indexed account, address indexed sender)
func (_WrappedToken *WrappedTokenFilterer) FilterRoleGranted(opts *bind.FilterOpts, role [][32]byte, account []common.Address, sender []common.Address) (*WrappedTokenRoleGrantedIterator, error) {

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

	logs, sub, err := _WrappedToken.contract.FilterLogs(opts, "RoleGranted", roleRule, accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return &WrappedTokenRoleGrantedIterator{contract: _WrappedToken.contract, event: "RoleGranted", logs: logs, sub: sub}, nil
}

// WatchRoleGranted is a free log subscription operation binding the contract event 0x2f8788117e7eff1d82e926ec794901d17c78024a50270940304540a733656f0d.
//
// Solidity: event RoleGranted(bytes32 indexed role, address indexed account, address indexed sender)
func (_WrappedToken *WrappedTokenFilterer) WatchRoleGranted(opts *bind.WatchOpts, sink chan<- *WrappedTokenRoleGranted, role [][32]byte, account []common.Address, sender []common.Address) (event.Subscription, error) {

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

	logs, sub, err := _WrappedToken.contract.WatchLogs(opts, "RoleGranted", roleRule, accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(WrappedTokenRoleGranted)
				if err := _WrappedToken.contract.UnpackLog(event, "RoleGranted", log); err != nil {
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
func (_WrappedToken *WrappedTokenFilterer) ParseRoleGranted(log types.Log) (*WrappedTokenRoleGranted, error) {
	event := new(WrappedTokenRoleGranted)
	if err := _WrappedToken.contract.UnpackLog(event, "RoleGranted", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// WrappedTokenRoleRevokedIterator is returned from FilterRoleRevoked and is used to iterate over the raw logs and unpacked data for RoleRevoked events raised by the WrappedToken contract.
type WrappedTokenRoleRevokedIterator struct {
	Event *WrappedTokenRoleRevoked // Event containing the contract specifics and raw log

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
func (it *WrappedTokenRoleRevokedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(WrappedTokenRoleRevoked)
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
		it.Event = new(WrappedTokenRoleRevoked)
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
func (it *WrappedTokenRoleRevokedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *WrappedTokenRoleRevokedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// WrappedTokenRoleRevoked represents a RoleRevoked event raised by the WrappedToken contract.
type WrappedTokenRoleRevoked struct {
	Role    [32]byte
	Account common.Address
	Sender  common.Address
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterRoleRevoked is a free log retrieval operation binding the contract event 0xf6391f5c32d9c69d2a47ea670b442974b53935d1edc7fd64eb21e047a839171b.
//
// Solidity: event RoleRevoked(bytes32 indexed role, address indexed account, address indexed sender)
func (_WrappedToken *WrappedTokenFilterer) FilterRoleRevoked(opts *bind.FilterOpts, role [][32]byte, account []common.Address, sender []common.Address) (*WrappedTokenRoleRevokedIterator, error) {

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

	logs, sub, err := _WrappedToken.contract.FilterLogs(opts, "RoleRevoked", roleRule, accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return &WrappedTokenRoleRevokedIterator{contract: _WrappedToken.contract, event: "RoleRevoked", logs: logs, sub: sub}, nil
}

// WatchRoleRevoked is a free log subscription operation binding the contract event 0xf6391f5c32d9c69d2a47ea670b442974b53935d1edc7fd64eb21e047a839171b.
//
// Solidity: event RoleRevoked(bytes32 indexed role, address indexed account, address indexed sender)
func (_WrappedToken *WrappedTokenFilterer) WatchRoleRevoked(opts *bind.WatchOpts, sink chan<- *WrappedTokenRoleRevoked, role [][32]byte, account []common.Address, sender []common.Address) (event.Subscription, error) {

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

	logs, sub, err := _WrappedToken.contract.WatchLogs(opts, "RoleRevoked", roleRule, accountRule, senderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(WrappedTokenRoleRevoked)
				if err := _WrappedToken.contract.UnpackLog(event, "RoleRevoked", log); err != nil {
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
func (_WrappedToken *WrappedTokenFilterer) ParseRoleRevoked(log types.Log) (*WrappedTokenRoleRevoked, error) {
	event := new(WrappedTokenRoleRevoked)
	if err := _WrappedToken.contract.UnpackLog(event, "RoleRevoked", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// WrappedTokenTransferIterator is returned from FilterTransfer and is used to iterate over the raw logs and unpacked data for Transfer events raised by the WrappedToken contract.
type WrappedTokenTransferIterator struct {
	Event *WrappedTokenTransfer // Event containing the contract specifics and raw log

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
func (it *WrappedTokenTransferIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(WrappedTokenTransfer)
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
		it.Event = new(WrappedTokenTransfer)
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
func (it *WrappedTokenTransferIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *WrappedTokenTransferIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// WrappedTokenTransfer represents a Transfer event raised by the WrappedToken contract.
type WrappedTokenTransfer struct {
	From  common.Address
	To    common.Address
	Value *big.Int
	Raw   types.Log // Blockchain specific contextual infos
}

// FilterTransfer is a free log retrieval operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_WrappedToken *WrappedTokenFilterer) FilterTransfer(opts *bind.FilterOpts, from []common.Address, to []common.Address) (*WrappedTokenTransferIterator, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _WrappedToken.contract.FilterLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &WrappedTokenTransferIterator{contract: _WrappedToken.contract, event: "Transfer", logs: logs, sub: sub}, nil
}

// WatchTransfer is a free log subscription operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(address indexed from, address indexed to, uint256 value)
func (_WrappedToken *WrappedTokenFilterer) WatchTransfer(opts *bind.WatchOpts, sink chan<- *WrappedTokenTransfer, from []common.Address, to []common.Address) (event.Subscription, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _WrappedToken.contract.WatchLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(WrappedTokenTransfer)
				if err := _WrappedToken.contract.UnpackLog(event, "Transfer", log); err != nil {
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
func (_WrappedToken *WrappedTokenFilterer) ParseTransfer(log types.Log) (*WrappedTokenTransfer, error) {
	event := new(WrappedTokenTransfer)
	if err := _WrappedToken.contract.UnpackLog(event, "Transfer", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
