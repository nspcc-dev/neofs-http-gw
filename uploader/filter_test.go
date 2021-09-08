package uploader

import (
	"testing"

	"github.com/nspcc-dev/neofs-sdk-go/pkg/logger"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestFilter(t *testing.T) {
	log, err := logger.New()
	require.NoError(t, err)

	req := &fasthttp.RequestHeader{}
	req.DisableNormalizing()
	req.Set("X-Attribute-Neofs-Expiration-Epoch1", "101")
	req.Set("X-Attribute-NEOFS-Expiration-Epoch2", "102")
	req.Set("X-Attribute-neofs-Expiration-Epoch3", "103")
	req.Set("X-Attribute-MyAttribute", "value")

	expected := map[string]string{
		"__NEOFS__EXPIRATION_EPOCH1": "101",
		"MyAttribute":                "value",
		"__NEOFS__EXPIRATION_EPOCH3": "103",
		"__NEOFS__EXPIRATION_EPOCH2": "102",
	}

	result := filterHeaders(log, req)

	require.Equal(t, expected, result)
}
