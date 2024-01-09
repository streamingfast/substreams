package substreams

import (
	"io"
	"log"
	"os"
)

func readAll(r io.Reader, allocSize int) ([]byte, error) {
	if allocSize <= 0 {
		allocSize = 1024
	}

	b := make([]byte, 0, allocSize)
	count := 0
	for {
		count++
		n, err := r.Read(b[len(b):cap(b)])
		log.Print("Read count: ", count)
		b = b[:len(b)+n]
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return b, err
		}

		if len(b) == cap(b) {
			// Add more capacity (let append pick how much).
			b = append(b, 0)[:len(b)]
		}
	}
}

func WriteOutput(data []byte) (int, error) {
	return os.Stdout.Write(data)
}

func ReadInput(allocSize int) ([]byte, error) {
	return readAll(os.Stdin, allocSize)
}

func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

type FileWriter interface {
	WriteFile(filename string, data []byte, perm os.FileMode) error
}

type FileReader interface {
	ReadFile(filename string) ([]byte, error)
}

type FileReadWriter interface {
	FileWriter
	FileReader
}

type OSFileReadWriter struct{}

func (r *OSFileReadWriter) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (r *OSFileReadWriter) ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}
