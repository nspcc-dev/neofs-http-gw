package downloader

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReader(t *testing.T) {
	data := []byte("test string")
	err := fmt.Errorf("something wrong")

	for _, tc := range []struct {
		err  error
		buff []byte
	}{
		{err: nil, buff: make([]byte, len(data)+1)},
		{err: nil, buff: make([]byte, len(data))},
		{err: nil, buff: make([]byte, len(data)-1)},
		{err: err, buff: make([]byte, len(data)+1)},
		{err: err, buff: make([]byte, len(data))},
		{err: err, buff: make([]byte, len(data)-1)},
	} {
		var res []byte
		var err error
		var n int

		r := newReader(data, tc.err)
		for err == nil {
			n, err = r.Read(tc.buff)
			res = append(res, tc.buff[:n]...)
		}

		if tc.err == nil {
			require.Equal(t, io.EOF, err)
		} else {
			require.Equal(t, tc.err, err)
		}
		require.Equal(t, data, res)
	}
}

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
			detector := newDetector()

			go func() {
				detector.SetReader(bytes.NewBufferString(tc.Expected))
				detector.Detect()
			}()

			detector.Wait()
			require.Equal(t, tc.ContentType, detector.contentType)

			data, err := io.ReadAll(detector.MultiReader())
			require.NoError(t, err)
			require.Equal(t, tc.Expected, string(data))
		})
	}
}
