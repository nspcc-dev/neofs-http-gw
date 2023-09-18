package uploader

import (
	"crypto/rand"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func generateRandomFile(size int64) (string, error) {
	file, err := os.CreateTemp("", "data")
	if err != nil {
		return "", err
	}

	_, err = io.CopyN(file, rand.Reader, size)
	if err != nil {
		return "", err
	}

	return file.Name(), file.Close()
}

func BenchmarkAll(b *testing.B) {
	fileName, err := generateRandomFile(1024 * 1024 * 256)
	require.NoError(b, err)
	fmt.Println(fileName)
	defer os.Remove(fileName)

	b.Run("bare", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := bareRead(fileName)
			require.NoError(b, err)
		}
	})

	b.Run("default", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := defaultMultipart(fileName)
			require.NoError(b, err)
		}
	})

	b.Run("custom", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := customMultipart(fileName)
			require.NoError(b, err)
		}
	})
}

func defaultMultipart(filename string) error {
	r, bound := multipartFile(filename)

	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}

	file, err := fetchMultipartFileDefault(logger, r, bound)
	if err != nil {
		return err
	}

	_, err = io.Copy(io.Discard, file)
	return err
}

func TestName(t *testing.T) {
	fileName, err := generateRandomFile(1024 * 1024 * 256)
	require.NoError(t, err)
	fmt.Println(fileName)
	defer os.Remove(fileName)

	err = defaultMultipart(fileName)
	require.NoError(t, err)
}

func customMultipart(filename string) error {
	r, bound := multipartFile(filename)

	logger, err := zap.NewProduction()
	if err != nil {
		return err
	}

	file, err := fetchMultipartFile(logger, r, bound)
	if err != nil {
		return err
	}

	_, err = io.Copy(io.Discard, file)
	return err
}

type multiFile struct {
	*multipart.Part
}

func (*multiFile) ContentType() string {
	return "text/plain"
}

func fetchMultipartFileDefault(l *zap.Logger, r io.Reader, boundary string) (MultipartFile, error) {
	reader := multipart.NewReader(r, boundary)

	for {
		part, err := reader.NextPart()
		if err != nil {
			return nil, err
		}

		name := part.FormName()
		if name == "" {
			l.Debug("ignore part, empty form name")
			continue
		}

		filename := part.FileName()

		// ignore multipart/form-data values
		if filename == "" {
			l.Debug("ignore part, empty filename", zap.String("form", name))

			continue
		}

		return &multiFile{part}, nil
	}
}

func bareRead(filename string) error {
	r, _ := multipartFile(filename)

	_, err := io.Copy(io.Discard, r)
	return err
}

func multipartFile(filename string) (*io.PipeReader, string) {
	r, w := io.Pipe()
	m := multipart.NewWriter(w)
	go func() {
		defer w.Close()
		defer m.Close()
		part, err := m.CreateFormFile("myFile", "foo.txt")
		if err != nil {
			fmt.Println(err)
			return
		}

		file, err := os.Open(filename)
		if err != nil {
			fmt.Println(err)
			return
		}
		defer file.Close()
		if _, err = io.Copy(part, file); err != nil {
			fmt.Println(err)
			return
		}
	}()

	return r, m.Boundary()
}
