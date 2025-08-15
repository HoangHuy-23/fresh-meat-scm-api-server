// internal/wallet/wallet.go
package wallet

import (
	"fmt"
	"os"
	"path/filepath"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

func PopulateWallet(wallet *gateway.Wallet, orgName, userName, certPath, keyDir string) error {
	if wallet.Exists(userName) {
		return nil
	}

	cert, err := os.ReadFile(filepath.Clean(certPath))
	if err != nil {
		return err
	}

	keyPath, err := findPrivateKey(keyDir)
	if err != nil {
		return err
	}
	key, err := os.ReadFile(filepath.Clean(keyPath))
	if err != nil {
		return err
	}

	identity := gateway.NewX509Identity(orgName+"MSP", string(cert), string(key))

	return wallet.Put(userName, identity)
}

func findPrivateKey(dir string) (string, error) {
	keyPath := ""
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			keyPath = path
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if keyPath == "" {
		return "", fmt.Errorf("no private key found in directory %s", dir)
	}
	return keyPath, nil
}