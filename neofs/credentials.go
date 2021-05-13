package neofs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"math/big"

	"github.com/nspcc-dev/neofs-api-go/pkg/owner"
	crypto "github.com/nspcc-dev/neofs-crypto"
)

type (
	// Credentials contains methods that needed to work with NeoFS.
	Credentials interface {
		Owner() *owner.ID
		PublicKey() *ecdsa.PublicKey
		PrivateKey() *ecdsa.PrivateKey
	}

	credentials struct {
		key     *ecdsa.PrivateKey
		ownerID *owner.ID
	}
)

// NewCredentials creates an instance of Credentials through string
// representation of secret. It allows passing WIF, path, hex-encoded and others.
func NewCredentials(secret string) (Credentials, error) {
	key, err := crypto.LoadPrivateKey(secret)
	if err != nil {
		return nil, err
	}
	return setFromPrivateKey(key)
}

// NewEphemeralCredentials creates new private key and Credentials based on that
// key.
func NewEphemeralCredentials() (Credentials, error) {
	c := elliptic.P256()
	priv, x, y, err := elliptic.GenerateKey(c, rand.Reader)
	if err != nil {
		return nil, err
	}
	key := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: c,
			X:     x,
			Y:     y,
		},
		D: new(big.Int).SetBytes(priv),
	}
	return setFromPrivateKey(key)
}

// PrivateKey returns ecdsa.PrivateKey.
func (c *credentials) PrivateKey() *ecdsa.PrivateKey {
	return c.key
}

// PublicKey returns ecdsa.PublicKey.
func (c *credentials) PublicKey() *ecdsa.PublicKey {
	return &c.key.PublicKey
}

// Owner returns owner.ID.
func (c *credentials) Owner() *owner.ID {
	return c.ownerID
}

func setFromPrivateKey(key *ecdsa.PrivateKey) (*credentials, error) {
	wallet, err := owner.NEO3WalletFromPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	ownerID := owner.NewIDFromNeo3Wallet(wallet)
	return &credentials{key: key, ownerID: ownerID}, nil
}
