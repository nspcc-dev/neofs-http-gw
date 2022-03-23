package model

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nspcc-dev/neofs-sdk-go/token"

	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neofs-sdk-go/eacl"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	"github.com/stretchr/testify/require"
)

func TestBearerToNative(t *testing.T) {
	fmt.Println(base64.StdEncoding.EncodeToString([]byte("content of file")))

	bearerStr := `
{
  "records": [
    {
      "operation": "GET",
      "action": "ALLOW",
      "filters": [
 		{
          "headerType": "OBJECT",
          "matchType": "STRING_EQUAL",
          "key": "$Object:objectType",
          "value": "REGULAR"
        }
      ],
      "targets": [
        {
          "role": "USER",
          "keys": [
            "021dc56fc6d81d581ae7605a8e00e0e0bab6cbad566a924a527339475a97a8e38e"
          ]
        }
      ]
    },
    {
      "operation": "HEAD",
      "action": "DENY",
      "filters": [
		{
          "headerType": "OBJECT",
          "matchType": "STRING_NOT_EQUAL",
          "key": "FileName",
          "value": "myfile"
        }
      ],
      "targets": [
        {
          "role": "OTHERS",
          "keys": []
        }
      ]
    }
  ]
}
`

	bearer := new(Bearer)
	err := json.Unmarshal([]byte(bearerStr), bearer)
	require.NoError(t, err)

	key, err := keys.NewPublicKeyFromString("021dc56fc6d81d581ae7605a8e00e0e0bab6cbad566a924a527339475a97a8e38e")
	require.NoError(t, err)

	var target eacl.Target
	target.SetRole(eacl.RoleUser)
	target.SetBinaryKeys([][]byte{key.Bytes()})
	var rec eacl.Record
	rec.SetOperation(eacl.OperationGet)
	rec.SetAction(eacl.ActionAllow)
	rec.SetTargets(target)
	rec.AddObjectTypeFilter(eacl.MatchStringEqual, object.TypeRegular)

	var target2 eacl.Target
	target2.SetRole(eacl.RoleOthers)
	target2.SetBinaryKeys([][]byte{})
	var rec2 eacl.Record
	rec2.SetOperation(eacl.OperationHead)
	rec2.SetAction(eacl.ActionDeny)
	rec2.SetTargets(target2)
	rec2.AddFilter(eacl.HeaderFromObject, eacl.MatchStringNotEqual, "FileName", "myfile")

	var table eacl.Table
	table.AddRecord(&rec)
	table.AddRecord(&rec2)

	expectedToken := new(token.BearerToken)
	expectedToken.SetEACLTable(&table)

	actualToken, err := bearer.ToNative()
	require.NoError(t, err)

	require.Equal(t, expectedToken, actualToken)
}
