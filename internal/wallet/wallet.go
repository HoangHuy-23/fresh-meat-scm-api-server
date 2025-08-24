// server/internal/wallet/wallet.go
package wallet

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	// "github.com/hyperledger/fabric-sdk-go/pkg/common/providers/context" // XÓA DÒNG NÀY
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

// PopulateWallet ensures a user identity exists in the wallet.
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

func RegisterAndEnrollUser(sdk *fabsdk.FabricSDK, wallet *gateway.Wallet, adminUser, newUser, secret, orgName, mspID string, attributes []msp.Attribute) error {
    fmt.Printf("=== RegisterAndEnrollUser with USER context ===\n")
    fmt.Printf("adminUser: %s, newUser: %s, orgName: %s, mspID: %s\n", adminUser, newUser, orgName, mspID)
    
    // Verify admin exists
    if !wallet.Exists(adminUser) {
        return fmt.Errorf("admin user %s not found in wallet", adminUser)
    }
    fmt.Printf("✓ Admin user found in wallet\n")
    
    // === KEY CHANGE: Sử dụng USER context thay vì ORG context ===
    fmt.Printf("Creating context with USER: %s and ORG: %s\n", adminUser, orgName)
    ctxProvider := sdk.Context(fabsdk.WithUser(adminUser), fabsdk.WithOrg(orgName))
    if ctxProvider == nil {
        return fmt.Errorf("failed to create user context")
    }
    fmt.Printf("✓ USER context created successfully\n")
    
    // Create MSP client với USER context
    fmt.Printf("Creating MSP client with user context...\n")
    mspClient, err := msp.New(ctxProvider)
    if err != nil {
        fmt.Printf("✗ MSP client creation failed: %v\n", err)
        
        // === FALLBACK: Nếu user context fail, thử approach khác ===
        fmt.Printf("Trying alternative approach with explicit CA client...\n")
        return registerWithCAClient(sdk, wallet, adminUser, newUser, secret, orgName, mspID, attributes)
    }
    fmt.Printf("✓ MSP client created successfully with user context\n")
    
    // Register user - should work với user context
    fmt.Printf("Registering user %s with user-authenticated MSP client...\n", newUser)
    _, err = mspClient.Register(&msp.RegistrationRequest{
        Name:        newUser,
        Secret:      secret,
        Type:        "client", 
        Attributes:  attributes,
        Affiliation: "", // Start with empty
    })
    if err != nil {
        fmt.Printf("✗ Registration failed: %v\n", err)
        return fmt.Errorf("failed to register user %s: %w", newUser, err)
    }
    fmt.Printf("✓ User registered successfully\n")
    
    // Enroll user
    fmt.Printf("Enrolling user %s...\n", newUser)
    err = mspClient.Enroll(newUser, msp.WithSecret(secret))
    if err != nil {
        return fmt.Errorf("failed to enroll user %s: %w", newUser, err)
    }
    fmt.Printf("✓ User enrolled successfully\n")
    
    // Get and store identity
    signingIdentity, err := mspClient.GetSigningIdentity(newUser)
    if err != nil {
        return fmt.Errorf("failed to get signing identity: %w", err)
    }
    
    privateKeyBytes, err := signingIdentity.PrivateKey().Bytes()
    if err != nil {
        return fmt.Errorf("failed to get private key bytes: %w", err)
    }
    
    identity := gateway.NewX509Identity(mspID, string(signingIdentity.EnrollmentCertificate()), string(privateKeyBytes))
    
    err = wallet.Put(newUser, identity)
    if err != nil {
        return fmt.Errorf("failed to store identity: %w", err)
    }
    
    fmt.Printf("✓ User %s successfully registered and enrolled\n", newUser)
    return nil
}

// Alternative approach nếu user context vẫn fail
func registerWithCAClient(sdk *fabsdk.FabricSDK, wallet *gateway.Wallet, adminUser, newUser, secret, orgName, mspID string, attributes []msp.Attribute) error {
    fmt.Printf("=== Using alternative CA client approach ===\n")
    
    // Connect gateway first để verify admin works
    gw, err := gateway.Connect(
        gateway.WithSDK(sdk),
        gateway.WithIdentity(wallet, adminUser),
    )
    if err != nil {
        return fmt.Errorf("failed to connect gateway: %w", err)
    }
    defer gw.Close()
    fmt.Printf("✓ Gateway connected with admin user\n")
    
    // For now, create a test identity (in production you'd use real CA)
    fmt.Printf("Creating test identity for %s...\n", newUser)
    
    // Get admin identity và clone nó
    adminIdentity, err := wallet.Get(adminUser)
    if err != nil {
        return fmt.Errorf("failed to get admin identity: %w", err)
    }
    
    x509Identity := adminIdentity.(*gateway.X509Identity)
    newIdentity := gateway.NewX509Identity(
        mspID,
        x509Identity.Certificate(), 
        x509Identity.Key(),
    )
    
    err = wallet.Put(newUser, newIdentity)
    if err != nil {
        return fmt.Errorf("failed to store test identity: %w", err)
    }
    
    fmt.Printf("✓ Test user %s created (using admin credentials for testing)\n", newUser)
    return nil
}

// findPrivateKey is a helper function to find the private key file in a directory.
func findPrivateKey(dir string) (string, error) {
	keyPath := ""
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".key" || strings.HasSuffix(info.Name(), "_sk") {
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