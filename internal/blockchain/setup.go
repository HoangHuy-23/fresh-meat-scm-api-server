// server/internal/blockchain/setup.go
package blockchain

import (
	"fmt"
	"os"
	"path/filepath"
	"fresh-meat-scm-api-server/config"
	// "fresh-meat-scm-api-server/internal/wallet" // Không cần import wallet nữa

	fabconfig "github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

type FabricSetup struct {
	Gateway  *gateway.Gateway
	Network  *gateway.Network
	Contract *gateway.Contract
}

// Initialize tạo kết nối gateway cho một user cụ thể.
func Initialize(cfg config.Config, wallet *gateway.Wallet, userName string) (*FabricSetup, error) {
	os.Setenv("DISCOVERY_AS_LOCALHOST", "true")

	// Không cần tạo wallet ở đây nữa, nó được truyền vào từ main

	// Không cần populate wallet ở đây nữa, nó được thực hiện ở main

	gw, err := gateway.Connect(
		gateway.WithConfig(fabconfig.FromFile(filepath.Clean(cfg.Fabric.ConnectionProfile))),
		gateway.WithIdentity(wallet, userName),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gateway as %s: %w", userName, err)
	}

	network, err := gw.GetNetwork(cfg.Fabric.ChannelName)
	if err != nil {
		gw.Close()
		return nil, fmt.Errorf("failed to get network: %w", err)
	}

	contract := network.GetContract(cfg.Fabric.ChaincodeName)

	return &FabricSetup{
		Gateway:  gw,
		Network:  network,
		Contract: contract,
	}, nil
}