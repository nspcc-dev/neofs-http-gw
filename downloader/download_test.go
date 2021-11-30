package downloader

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSystemBackwardTranslator(t *testing.T) {
	input := []string{
		"__NEOFS__EXPIRATION_EPOCH",
		"__NEOFS__RANDOM_ATTR",
	}
	expected := []string{
		"Neofs-Expiration-Epoch",
		"Neofs-Random-Attr",
	}

	for i, str := range input {
		res := systemBackwardTranslator(str)
		require.Equal(t, expected[i], res)
	}
}
