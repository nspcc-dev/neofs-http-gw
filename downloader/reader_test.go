package downloader

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetector(t *testing.T) {
	txtContentType := "text/plain; charset=utf-8"
	sb := strings.Builder{}
	for i := 0; i < 10; i++ {
		sb.WriteString("Some txt content. Content-Type must be detected properly by detector.")
	}

	for _, tc := range []struct {
		Name        string
		ContentType string
		Expected    string
	}{
		{
			Name:        "less than 512b",
			ContentType: txtContentType,
			Expected:    sb.String()[:256],
		},
		{
			Name:        "more than 512b",
			ContentType: txtContentType,
			Expected:    sb.String(),
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			contentType, data, err := readContentType(uint64(len(tc.Expected)),
				func(sz uint64) (io.Reader, error) {
					return strings.NewReader(tc.Expected), nil
				},
			)

			require.NoError(t, err)
			require.Equal(t, tc.ContentType, contentType)
			require.True(t, strings.HasPrefix(tc.Expected, string(data)))
		})
	}
}
