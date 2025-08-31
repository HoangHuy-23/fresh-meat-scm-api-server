// server/internal/blockchain/setup.go
package blockchain

import (
	"fmt"
	"os"
	"path/filepath"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/wallet"

	fabconfig "github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

type FabricSetup struct {
	Gateway  *gateway.Gateway
	Contract *gateway.Contract
	SDK      *fabsdk.FabricSDK
	Wallet   *gateway.Wallet // <-- THÊM WALLET VÀO STRUCT
}

func Initialize(cfg config.Config) (*FabricSetup, error) {
	os.Setenv("DISCOVERY_AS_LOCALHOST", "true")

	fsWallet, err := gateway.NewFileSystemWallet("wallet")
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	err = wallet.PopulateWallet(fsWallet, cfg.OrgName, cfg.UserName, cfg.UserCertPath, cfg.UserKeyDir)
	if err != nil {
		return nil, fmt.Errorf("failed to populate wallet for admin: %w", err)
	}

	sdk, err := fabsdk.New(fabconfig.FromFile(filepath.Clean(cfg.ConnectionProfile)))
	if err != nil {
		return nil, fmt.Errorf("failed to create fabsdk instance: %w", err)
	}

	gw, err := gateway.Connect(
		gateway.WithSDK(sdk),
		gateway.WithIdentity(fsWallet, cfg.UserName),
	)
	if err != nil {
		sdk.Close()
		return nil, fmt.Errorf("failed to connect to gateway: %w", err)
	}

	network, err := gw.GetNetwork(cfg.ChannelName)
	if err != nil {
		gw.Close()
		sdk.Close()
		return nil, fmt.Errorf("failed to get network: %w", err)
	}

	contract := network.GetContract(cfg.ChaincodeName)

	return &FabricSetup{
		Gateway:  gw,
		Contract: contract,
		SDK:      sdk,
		Wallet:   fsWallet, // <-- TRẢ VỀ WALLET
	}, nil
}

// GetGatewayForUser creates a new Gateway connection using a specific user's identity.
func (fs *FabricSetup) GetGatewayForUser(userName string) (*gateway.Gateway, error) {
	// Tái sử dụng SDK và Wallet đã được khởi tạo
	gw, err := gateway.Connect(
		gateway.WithSDK(fs.SDK),
		gateway.WithIdentity(fs.Wallet, userName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gateway for user %s: %w", userName, err)
	}
	return gw, nil
}