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
	ABI: "[{\"inputs\":[{\"internalType\":\"address\",\"name\":\"_relayer\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"_maxTransferAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"_minTransferAmount\",\"type\":\"uint256\"}],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"target\",\"type\":\"address\"}],\"name\":\"AddressEmptyCode\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"AddressInsufficientBalance\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"EnforcedPause\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ExpectedPause\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"FailedInnerCall\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"owner\",\"type\":\"address\"}],\"name\":\"OwnableInvalidOwner\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"OwnableUnauthorizedAccount\",\"type\":\"error\"},{\"inputs\":[],\"name\":\"ReentrancyGuardReentrantCall\",\"type\":\"error\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"}],\"name\":\"SafeERC20FailedOperation\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"maxTransferAmount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"minTransferAmount\",\"type\":\"uint256\"}],\"name\":\"BridgeLimitsUpdated\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"sender\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"cantonRecipient\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"isWrapped\",\"type\":\"bool\"}],\"name\":\"DepositToCanton\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"previousOwner\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"OwnershipTransferred\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"Paused\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"oldRelayer\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"newRelayer\",\"type\":\"address\"}],\"name\":\"RelayerUpdated\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"ethereumToken\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"cantonTokenId\",\"type\":\"bytes32\"},{\"indexed\":false,\"internalType\":\"bool\",\"name\":\"isWrapped\",\"type\":\"bool\"}],\"name\":\"TokenMappingAdded\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"ethereumToken\",\"type\":\"address\"}],\"name\":\"TokenMappingRemoved\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"address\",\"name\":\"account\",\"type\":\"address\"}],\"name\":\"Unpaused\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"recipient\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"bytes32\",\"name\":\"cantonTxHash\",\"type\":\"bytes32\"}],\"name\":\"WithdrawFromCanton\",\"type\":\"event\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"ethereumToken\",\"type\":\"address\"},{\"internalType\":\"bytes32\",\"name\":\"cantonTokenId\",\"type\":\"bytes32\"},{\"internalType\":\"bool\",\"name\":\"wrapped\",\"type\":\"bool\"}],\"name\":\"addTokenMapping\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"cantonToEthereumToken\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"depositNonce\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"cantonRecipient\",\"type\":\"bytes32\"}],\"name\":\"depositToCanton\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"emergencyWithdraw\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"ethereumToCantonToken\",\"outputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"}],\"name\":\"getBridgeBalance\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"name\":\"isWrappedToken\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"maxTransferAmount\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"minTransferAmount\",\"outputs\":[{\"internalType\":\"uint256\",\"name\":\"\",\"type\":\"uint256\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"owner\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"pause\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"paused\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"processedCantonTxs\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"relayer\",\"outputs\":[{\"internalType\":\"address\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"ethereumToken\",\"type\":\"address\"}],\"name\":\"removeTokenMapping\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"renounceOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newOwner\",\"type\":\"address\"}],\"name\":\"transferOwnership\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[],\"name\":\"unpause\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"uint256\",\"name\":\"_maxTransferAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"_minTransferAmount\",\"type\":\"uint256\"}],\"name\":\"updateLimits\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"newRelayer\",\"type\":\"address\"}],\"name\":\"updateRelayer\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"address\",\"name\":\"token\",\"type\":\"address\"},{\"internalType\":\"address\",\"name\":\"recipient\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"nonce\",\"type\":\"uint256\"},{\"internalType\":\"bytes32\",\"name\":\"cantonTxHash\",\"type\":\"bytes32\"}],\"name\":\"withdrawFromCanton\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"}]",
	Bin: "0x60806040523480156200001157600080fd5b506040516200162938038062001629833981016040819052620000349162000190565b33806200005c57604051631e4fbdf760e01b8152600060048201526024015b60405180910390fd5b620000678162000140565b506000805460ff60a01b19169055600180556001600160a01b038316620000d15760405162461bcd60e51b815260206004820152601760248201527f496e76616c69642072656c617965722061646472657373000000000000000000604482015260640162000053565b808211620001135760405162461bcd60e51b815260206004820152600e60248201526d496e76616c6964206c696d69747360901b604482015260640162000053565b600280546001600160a01b0319166001600160a01b039490941693909317909255600455600555620001d5565b600080546001600160a01b038381166001600160a01b0319831681178455604051919092169283917f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e09190a35050565b600080600060608486031215620001a657600080fd5b83516001600160a01b0381168114620001be57600080fd5b602085015160409095015190969495509392505050565b61144480620001e56000396000f3fe608060405234801561001057600080fd5b50600436106101425760003560e01c80638406c079116100b85780639ec4eaa81161007c5780639ec4eaa8146102b5578063a2240e19146102c8578063a9e75723146102db578063d1569b4c146102e4578063de35f5cb14610307578063f2fde38b1461031057600080fd5b80638406c079146102635780638456cb59146102765780638da5cb5b1461027e5780638f83ab131461028f57806395ccea67146102a257600080fd5b80635c975abb1161010a5780635c975abb146101eb57806362cc30d01461020957806364fb065b1461021c578063688414311461023f578063715018a614610248578063828a54fb1461025057600080fd5b8063058c055f146101475780630cc761ae1461016d5780631a9bd38d1461018d5780633f4ba83a146101ce5780635a94d032146101d8575b600080fd5b61015a610155366004611205565b610323565b6040519081526020015b60405180910390f35b61015a61017b366004611205565b60066020526000908152604090205481565b6101b661019b366004611220565b6008602052600090815260409020546001600160a01b031681565b6040516001600160a01b039091168152602001610164565b6101d6610394565b005b6101d66101e6366004611239565b6103a6565b600054600160a01b900460ff165b6040519015158152602001610164565b6101d6610217366004611286565b61071b565b6101f961022a366004611205565b60076020526000908152604090205460ff1681565b61015a60055481565b6101d66109a2565b6101d661025e366004611205565b6109b4565b6002546101b6906001600160a01b031681565b6101d6610a8b565b6000546001600160a01b03166101b6565b6101d661029d366004611205565b610a9b565b6101d66102b03660046112b9565b610b4b565b6101d66102c33660046112f1565b610bee565b6101d66102d6366004611331565b610d50565b61015a60045481565b6101f96102f2366004611220565b60096020526000908152604090205460ff1681565b61015a60035481565b6101d661031e366004611205565b610ddf565b6040516370a0823160e01b81523060048201526000906001600160a01b038316906370a0823190602401602060405180830381865afa15801561036a573d6000803e3d6000fd5b505050506040513d601f19601f8201168201806040525081019061038e9190611353565b92915050565b61039c610e1d565b6103a4610e4a565b565b6103ae610e9f565b6103b6610ec9565b6002546001600160a01b0316331461040d5760405162461bcd60e51b815260206004820152601560248201527413db9b1e481c995b185e595c8818d85b8818d85b1b605a1b60448201526064015b60405180910390fd5b6001600160a01b0385166104335760405162461bcd60e51b81526004016104049061136c565b6001600160a01b03841661047d5760405162461bcd60e51b8152602060048201526011602482015270125b9d985b1a59081c9958da5c1a595b9d607a1b6044820152606401610404565b6005548310156104c65760405162461bcd60e51b8152602060048201526014602482015273416d6f756e742062656c6f77206d696e696d756d60601b6044820152606401610404565b6004548311156105115760405162461bcd60e51b8152602060048201526016602482015275416d6f756e742065786365656473206d6178696d756d60501b6044820152606401610404565b806105575760405162461bcd60e51b8152602060048201526016602482015275092dcecc2d8d2c84086c2dce8dedc40e8f040d0c2e6d60531b6044820152606401610404565b60008181526009602052604090205460ff16156105aa5760405162461bcd60e51b8152602060048201526011602482015270105b1c9958591e481c1c9bd8d95cdcd959607a1b6044820152606401610404565b6001600160a01b0385166000908152600660205260409020546106055760405162461bcd60e51b8152602060048201526013602482015272151bdad95b881b9bdd081cdd5c1c1bdc9d1959606a1b6044820152606401610404565b6000818152600960209081526040808320805460ff191660011790556001600160a01b0388168352600790915290205460ff1680156106a5576040516340c10f1960e01b81526001600160a01b038681166004830152602482018690528716906340c10f1990604401600060405180830381600087803b15801561068857600080fd5b505af115801561069c573d6000803e3d6000fd5b505050506106b9565b6106b96001600160a01b0387168686610ef4565b60408051858152602081018590529081018390526001600160a01b0380871691908816907f08f0eddc3c8a0bc5c0fb02dfcd2a51f46e8d93a0473f21c11715b6efce3380a09060600160405180910390a35061071460018055565b5050505050565b610723610e9f565b61072b610ec9565b6001600160a01b0383166107515760405162461bcd60e51b81526004016104049061136c565b60055482101561079a5760405162461bcd60e51b8152602060048201526014602482015273416d6f756e742062656c6f77206d696e696d756d60601b6044820152606401610404565b6004548211156107e55760405162461bcd60e51b8152602060048201526016602482015275416d6f756e742065786365656473206d6178696d756d60501b6044820152606401610404565b806108325760405162461bcd60e51b815260206004820152601860248201527f496e76616c69642043616e746f6e20726563697069656e7400000000000000006044820152606401610404565b6001600160a01b03831660009081526006602052604090205461088d5760405162461bcd60e51b8152602060048201526013602482015272151bdad95b881b9bdd081cdd5c1c1bdc9d1959606a1b6044820152606401610404565b6001600160a01b03831660009081526007602052604090205460ff1680156109145760405163079cc67960e41b8152336004820152602481018490526001600160a01b038516906379cc679090604401600060405180830381600087803b1580156108f757600080fd5b505af115801561090b573d6000803e3d6000fd5b50505050610929565b6109296001600160a01b038516333086610f53565b60035460408051858152602081019290925282151590820152829033906001600160a01b038716907ff19e6675fc318ec130986145276c2dc0ab30490a0db2eb761db8adf3e9e1decf9060600160405180910390a46003805490600061098e8361139b565b91905055505061099d60018055565b505050565b6109aa610e1d565b6103a46000610f92565b6109bc610e1d565b6001600160a01b038116600090815260066020526040902054610a145760405162461bcd60e51b815260206004820152601060248201526f151bdad95b881b9bdd081b585c1c195960821b6044820152606401610404565b6001600160a01b03811660008181526006602090815260408083208054908490558084526008835281842080546001600160a01b03191690558484526007909252808320805460ff19169055519092917f3cc70907a9d2be7ea4e6b746d9739cad4d110ebb6c622b34380fee32d6cbed7091a25050565b610a93610e1d565b6103a4610fe2565b610aa3610e1d565b6001600160a01b038116610af95760405162461bcd60e51b815260206004820152601760248201527f496e76616c69642072656c6179657220616464726573730000000000000000006044820152606401610404565b600280546001600160a01b038381166001600160a01b0319831681179093556040519116919082907f605ca4e43489fb38b91aa63dd9147cd3847957694b080b9285ec898b34269f0c90600090a35050565b610b53610e1d565b610b5b611025565b6001600160a01b03821660009081526007602052604090205460ff1615610bc45760405162461bcd60e51b815260206004820152601e60248201527f43616e6e6f74207769746864726177207772617070656420746f6b656e7300006044820152606401610404565b610bea610bd96000546001600160a01b031690565b6001600160a01b0384169083610ef4565b5050565b610bf6610e1d565b6001600160a01b038316610c1c5760405162461bcd60e51b81526004016104049061136c565b81610c695760405162461bcd60e51b815260206004820152601760248201527f496e76616c69642043616e746f6e20746f6b656e2049440000000000000000006044820152606401610404565b6001600160a01b03831660009081526006602052604090205415610cc65760405162461bcd60e51b8152602060048201526014602482015273151bdad95b88185b1c9958591e481b585c1c195960621b6044820152606401610404565b6001600160a01b03831660008181526006602090815260408083208690558583526008825280832080546001600160a01b031916851790558383526007825291829020805460ff191685151590811790915591519182528492917f07be94684bb65d3348033a8eac0731d5185b647638357b3d8b9dd5045aa96a08910160405180910390a3505050565b610d58610e1d565b808211610d985760405162461bcd60e51b815260206004820152600e60248201526d496e76616c6964206c696d69747360901b6044820152606401610404565b6004829055600581905560408051838152602081018390527fcfc62648bfe6689bfdf06f50d3233f77d9e91b33d62e0b52350865779cd83151910160405180910390a15050565b610de7610e1d565b6001600160a01b038116610e1157604051631e4fbdf760e01b815260006004820152602401610404565b610e1a81610f92565b50565b6000546001600160a01b031633146103a45760405163118cdaa760e01b8152336004820152602401610404565b610e52611025565b6000805460ff60a01b191690557f5db9ee0a495bf2e6ff9c91a7834c1ba4fdd244a5e8aa4e537bd38aeae4b073aa335b6040516001600160a01b03909116815260200160405180910390a1565b600260015403610ec257604051633ee5aeb560e01b815260040160405180910390fd5b6002600155565b600054600160a01b900460ff16156103a45760405163d93c066560e01b815260040160405180910390fd5b6040516001600160a01b0383811660248301526044820183905261099d91859182169063a9059cbb906064015b604051602081830303815290604052915060e01b6020820180516001600160e01b03838183161783525050505061104f565b6040516001600160a01b038481166024830152838116604483015260648201839052610f8c9186918216906323b872dd90608401610f21565b50505050565b600080546001600160a01b038381166001600160a01b0319831681178455604051919092169283917f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e09190a35050565b610fea610ec9565b6000805460ff60a01b1916600160a01b1790557f62e78cea01bee320cd4e420270b5ea74000d11b0c9f74754ebdbfc544b05a258610e823390565b600054600160a01b900460ff166103a457604051638dfc202b60e01b815260040160405180910390fd5b60006110646001600160a01b038416836110b2565b9050805160001415801561108957508080602001905181019061108791906113c2565b155b1561099d57604051635274afe760e01b81526001600160a01b0384166004820152602401610404565b60606110c0838360006110c7565b9392505050565b6060814710156110ec5760405163cd78605960e01b8152306004820152602401610404565b600080856001600160a01b0316848660405161110891906113df565b60006040518083038185875af1925050503d8060008114611145576040519150601f19603f3d011682016040523d82523d6000602084013e61114a565b606091505b509150915061115a868383611164565b9695505050505050565b60608261117957611174826111c0565b6110c0565b815115801561119057506001600160a01b0384163b155b156111b957604051639996b31560e01b81526001600160a01b0385166004820152602401610404565b50806110c0565b8051156111d05780518082602001fd5b604051630a12f52160e11b815260040160405180910390fd5b80356001600160a01b038116811461120057600080fd5b919050565b60006020828403121561121757600080fd5b6110c0826111e9565b60006020828403121561123257600080fd5b5035919050565b600080600080600060a0868803121561125157600080fd5b61125a866111e9565b9450611268602087016111e9565b94979496505050506040830135926060810135926080909101359150565b60008060006060848603121561129b57600080fd5b6112a4846111e9565b95602085013595506040909401359392505050565b600080604083850312156112cc57600080fd5b6112d5836111e9565b946020939093013593505050565b8015158114610e1a57600080fd5b60008060006060848603121561130657600080fd5b61130f846111e9565b9250602084013591506040840135611326816112e3565b809150509250925092565b6000806040838503121561134457600080fd5b50508035926020909101359150565b60006020828403121561136557600080fd5b5051919050565b602080825260159082015274496e76616c696420746f6b656e206164647265737360581b604082015260600190565b6000600182016113bb57634e487b7160e01b600052601160045260246000fd5b5060010190565b6000602082840312156113d457600080fd5b81516110c0816112e3565b6000825160005b8181101561140057602081860181015185830152016113e6565b50600092019182525091905056fea264697066735822122059f44217053923550dd0ba53973fa0fe8b158f900987e2062df6ca12892c443764736f6c63430008140033",
}

// CantonBridgeABI is the input ABI used to generate the binding from.
// Deprecated: Use CantonBridgeMetaData.ABI instead.
var CantonBridgeABI = CantonBridgeMetaData.ABI

// CantonBridgeBin is the compiled bytecode used for deploying new contracts.
// Deprecated: Use CantonBridgeMetaData.Bin instead.
var CantonBridgeBin = CantonBridgeMetaData.Bin

// DeployCantonBridge deploys a new Ethereum contract, binding an instance of CantonBridge to it.
func DeployCantonBridge(auth *bind.TransactOpts, backend bind.ContractBackend, _relayer common.Address, _maxTransferAmount *big.Int, _minTransferAmount *big.Int) (common.Address, *types.Transaction, *CantonBridge, error) {
	parsed, err := CantonBridgeMetaData.GetAbi()
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	if parsed == nil {
		return common.Address{}, nil, nil, errors.New("GetABI returned nil")
	}

	address, tx, contract, err := bind.DeployContract(auth, *parsed, common.FromHex(CantonBridgeBin), backend, _relayer, _maxTransferAmount, _minTransferAmount)
	if err != nil {
		return common.Address{}, nil, nil, err
	}
	return address, tx, &CantonBridge{CantonBridgeCaller: CantonBridgeCaller{contract: contract}, CantonBridgeTransactor: CantonBridgeTransactor{contract: contract}, CantonBridgeFilterer: CantonBridgeFilterer{contract: contract}}, nil
}

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

// IsWrappedToken is a free data retrieval call binding the contract method 0x64fb065b.
//
// Solidity: function isWrappedToken(address ) view returns(bool)
func (_CantonBridge *CantonBridgeCaller) IsWrappedToken(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var out []interface{}
	err := _CantonBridge.contract.Call(opts, &out, "isWrappedToken", arg0)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsWrappedToken is a free data retrieval call binding the contract method 0x64fb065b.
//
// Solidity: function isWrappedToken(address ) view returns(bool)
func (_CantonBridge *CantonBridgeSession) IsWrappedToken(arg0 common.Address) (bool, error) {
	return _CantonBridge.Contract.IsWrappedToken(&_CantonBridge.CallOpts, arg0)
}

// IsWrappedToken is a free data retrieval call binding the contract method 0x64fb065b.
//
// Solidity: function isWrappedToken(address ) view returns(bool)
func (_CantonBridge *CantonBridgeCallerSession) IsWrappedToken(arg0 common.Address) (bool, error) {
	return _CantonBridge.Contract.IsWrappedToken(&_CantonBridge.CallOpts, arg0)
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

// AddTokenMapping is a paid mutator transaction binding the contract method 0x9ec4eaa8.
//
// Solidity: function addTokenMapping(address ethereumToken, bytes32 cantonTokenId, bool wrapped) returns()
func (_CantonBridge *CantonBridgeTransactor) AddTokenMapping(opts *bind.TransactOpts, ethereumToken common.Address, cantonTokenId [32]byte, wrapped bool) (*types.Transaction, error) {
	return _CantonBridge.contract.Transact(opts, "addTokenMapping", ethereumToken, cantonTokenId, wrapped)
}

// AddTokenMapping is a paid mutator transaction binding the contract method 0x9ec4eaa8.
//
// Solidity: function addTokenMapping(address ethereumToken, bytes32 cantonTokenId, bool wrapped) returns()
func (_CantonBridge *CantonBridgeSession) AddTokenMapping(ethereumToken common.Address, cantonTokenId [32]byte, wrapped bool) (*types.Transaction, error) {
	return _CantonBridge.Contract.AddTokenMapping(&_CantonBridge.TransactOpts, ethereumToken, cantonTokenId, wrapped)
}

// AddTokenMapping is a paid mutator transaction binding the contract method 0x9ec4eaa8.
//
// Solidity: function addTokenMapping(address ethereumToken, bytes32 cantonTokenId, bool wrapped) returns()
func (_CantonBridge *CantonBridgeTransactorSession) AddTokenMapping(ethereumToken common.Address, cantonTokenId [32]byte, wrapped bool) (*types.Transaction, error) {
	return _CantonBridge.Contract.AddTokenMapping(&_CantonBridge.TransactOpts, ethereumToken, cantonTokenId, wrapped)
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
	IsWrapped       bool
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterDepositToCanton is a free log retrieval operation binding the contract event 0xf19e6675fc318ec130986145276c2dc0ab30490a0db2eb761db8adf3e9e1decf.
//
// Solidity: event DepositToCanton(address indexed token, address indexed sender, bytes32 indexed cantonRecipient, uint256 amount, uint256 nonce, bool isWrapped)
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

// WatchDepositToCanton is a free log subscription operation binding the contract event 0xf19e6675fc318ec130986145276c2dc0ab30490a0db2eb761db8adf3e9e1decf.
//
// Solidity: event DepositToCanton(address indexed token, address indexed sender, bytes32 indexed cantonRecipient, uint256 amount, uint256 nonce, bool isWrapped)
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

// ParseDepositToCanton is a log parse operation binding the contract event 0xf19e6675fc318ec130986145276c2dc0ab30490a0db2eb761db8adf3e9e1decf.
//
// Solidity: event DepositToCanton(address indexed token, address indexed sender, bytes32 indexed cantonRecipient, uint256 amount, uint256 nonce, bool isWrapped)
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
	IsWrapped     bool
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterTokenMappingAdded is a free log retrieval operation binding the contract event 0x07be94684bb65d3348033a8eac0731d5185b647638357b3d8b9dd5045aa96a08.
//
// Solidity: event TokenMappingAdded(address indexed ethereumToken, bytes32 indexed cantonTokenId, bool isWrapped)
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

// WatchTokenMappingAdded is a free log subscription operation binding the contract event 0x07be94684bb65d3348033a8eac0731d5185b647638357b3d8b9dd5045aa96a08.
//
// Solidity: event TokenMappingAdded(address indexed ethereumToken, bytes32 indexed cantonTokenId, bool isWrapped)
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

// ParseTokenMappingAdded is a log parse operation binding the contract event 0x07be94684bb65d3348033a8eac0731d5185b647638357b3d8b9dd5045aa96a08.
//
// Solidity: event TokenMappingAdded(address indexed ethereumToken, bytes32 indexed cantonTokenId, bool isWrapped)
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
