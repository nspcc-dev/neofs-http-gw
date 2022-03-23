package handlers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neofs-http-gw/rest/v1/model"
	"github.com/nspcc-dev/neofs-sdk-go/owner"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

const devenvPrivateKey = "1dd37fba80fec4e6a6f13fd708d8dcb3b29def768017052f6c930fa1c5d90bbb"

func TestSign(t *testing.T) {
	key, err := keys.NewPrivateKeyFromHex(devenvPrivateKey)
	require.NoError(t, err)

	pubKeyHex := hex.EncodeToString(key.PublicKey().Bytes())

	b := model.Bearer{
		Records: []model.Record{{
			Operation: model.OperationPut,
			Action:    model.ActionAllow,
			Filters:   []model.Filter{},
			Targets: []model.Target{{
				Role: model.RoleOthers,
				Keys: []string{},
			}},
		}},
	}

	btoken, err := b.ToNative()
	require.NoError(t, err)

	ownerKey, err := keys.NewPublicKeyFromString(pubKeyHex)
	require.NoError(t, err)

	btoken.SetOwner(owner.NewIDFromPublicKey((*ecdsa.PublicKey)(ownerKey)))

	binaryBearer, err := btoken.ToV2().GetBody().StableMarshal(nil)
	require.NoError(t, err)

	bearerBase64 := base64.StdEncoding.EncodeToString(binaryBearer)

	h := sha512.Sum512(binaryBearer)
	x, y, err := ecdsa.Sign(rand.Reader, &key.PrivateKey, h[:])
	if err != nil {
		panic(err)
	}
	signatureData := elliptic.Marshal(elliptic.P256(), x, y)

	var h1 fasthttp.RequestHeader
	h1.Set("Authorization", "Bearer "+bearerBase64)
	h1.Set(XNeofsBearerSignature, base64.StdEncoding.EncodeToString(signatureData))
	h1.Set(XNeofsBearerOwnerKey, pubKeyHex)

	_, err = prepareBearerToken(&h1)
	require.NoError(t, err)
}
