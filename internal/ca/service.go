package ca

import (
	"fmt"

	"github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
)

type CAService struct {
	sdk       *fabsdk.FabricSDK
	caName    string
	orgName   string
	adminUser string
}

// NewCAService tạo CA service với thông tin về admin user có quyền registrar
func NewCAService(sdk *fabsdk.FabricSDK, caName, orgName, adminUser string) (*CAService, error) {
	return &CAService{
		sdk:       sdk,
		caName:    caName,
		orgName:   orgName,
		adminUser: adminUser,
	}, nil
}

// ============================
// Helper: check và tạo affiliation
// ============================
func (s *CAService) ensureAffiliation(mspClient *msp.Client, target string) error {
	affiliations, err := mspClient.GetAllAffiliations()
	if err != nil {
		return fmt.Errorf("failed to get affiliations: %w", err)
	}

	// Duyệt cây affiliation (struct, không phải con trỏ)
	var exists func(target string, aff msp.AffiliationInfo) bool
	exists = func(target string, aff msp.AffiliationInfo) bool {
		if aff.Name == target {
			return true
		}
		for _, child := range aff.Affiliations {
			if exists(target, child) {
				return true
			}
		}
		return false
	}

	for _, aff := range affiliations.Affiliations {
		if exists(target, aff) {
			fmt.Printf("✓ Affiliation %s already exists\n", target)
			return nil
		}
	}

	// Nếu chưa có thì tạo mới
	req := &msp.AffiliationRequest{
		Name:  target,
		Force: true,
	}
	_, err = mspClient.AddAffiliation(req) // trả về (resp, err)
	if err != nil {
		return fmt.Errorf("failed to add affiliation %s: %w", target, err)
	}
	fmt.Printf("✓ Affiliation %s created successfully\n", target)
	return nil
}


// ============================
// Register user
// ============================
func (s *CAService) RegisterUser(enrollmentID, affiliation string, attributes []msp.Attribute) (string, error) {
	// === Tạo MSP client với context của admin user ===
	fmt.Printf("Creating MSP client with admin user: %s, org: %s, ca: %s\n", s.adminUser, s.orgName, s.caName)

	ctxProvider := s.sdk.Context(fabsdk.WithUser(s.adminUser), fabsdk.WithOrg(s.orgName))
	mspClient, err := msp.New(ctxProvider, msp.WithCAInstance(s.caName))
	if err != nil {
		return "", fmt.Errorf("failed to create msp client: %w", err)
	}
	fmt.Printf("✓ MSP client created successfully\n")

	// === KEY FIX: check và tạo affiliation nếu chưa có ===
	if err := s.ensureAffiliation(mspClient, affiliation); err != nil {
		return "", fmt.Errorf("failed to ensure affiliation %s: %w", affiliation, err)
	}

	fmt.Printf("Registering user: %s with affiliation: %s\n", enrollmentID, affiliation)
	secret, err := mspClient.Register(&msp.RegistrationRequest{
		Name:        enrollmentID,
		Type:        "client",
		Affiliation: affiliation,
		Attributes:  attributes,
	})
	if err != nil {
		return "", fmt.Errorf("failed to register user %s: %w", enrollmentID, err)
	}

	fmt.Printf("✓ User %s registered successfully with secret\n", enrollmentID)
	return secret, nil
}

// ============================
// Enroll user
// ============================
func (s *CAService) EnrollUser(enrollmentID, secret string) (cert []byte, key []byte, err error) {
	ctxProvider := s.sdk.Context(fabsdk.WithUser(s.adminUser), fabsdk.WithOrg(s.orgName))
	mspClient, err := msp.New(ctxProvider, msp.WithCAInstance(s.caName))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create msp client for enrollment: %w", err)
	}

	fmt.Printf("Enrolling user: %s\n", enrollmentID)
	err = mspClient.Enroll(enrollmentID, msp.WithSecret(secret))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to enroll user %s: %w", enrollmentID, err)
	}

	signingIdentity, err := mspClient.GetSigningIdentity(enrollmentID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get signing identity: %w", err)
	}

	privateKeyBytes, err := signingIdentity.PrivateKey().Bytes()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get private key bytes: %w", err)
	}

	fmt.Printf("✓ User %s enrolled successfully\n", enrollmentID)
	return signingIdentity.EnrollmentCertificate(), privateKeyBytes, nil
}
