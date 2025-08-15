// internal/blockchain/setup.go
package blockchain

import (
	"fmt"
	"os"
	"path/filepath"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/wallet"

	fabconfig "github.com/hyperledger/fabric-sdk-go/pkg/core/config" 
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

type FabricSetup struct {
	Gateway *gateway.Gateway
	Network *gateway.Network
	Contract *gateway.Contract
}

func Initialize(cfg config.Config) (*FabricSetup, error) {
	os.Setenv("DISCOVERY_AS_LOCALHOST", "true")

	fsWallet, err := gateway.NewFileSystemWallet("wallet")
	if err != nil {
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}

	err = wallet.PopulateWallet(fsWallet, cfg.OrgName, cfg.UserName, cfg.UserCertPath, cfg.UserKeyDir)
	if err != nil {
		return nil, fmt.Errorf("failed to populate wallet: %w", err)
	}

	gw, err := gateway.Connect(
		gateway.WithConfig(fabconfig.FromFile(filepath.Clean(cfg.ConnectionProfile))),
		gateway.WithIdentity(fsWallet, cfg.UserName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gateway: %w", err)
	}

	network, err := gw.GetNetwork(cfg.ChannelName)
	if err != nil {
		gw.Close()
		return nil, fmt.Errorf("failed to get network: %w", err)
	}

	contract := network.GetContract(cfg.ChaincodeName)

	return &FabricSetup{
		Gateway: gw,
		Network: network,
		Contract: contract,
	}, nil
}