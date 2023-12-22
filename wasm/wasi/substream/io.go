package substream

import (
	"io"
	"log"
	"os"
)

func ReadAll(r io.Reader) ([]byte, error) {
	b := make([]byte, 0, 1024*1024)
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

func ReadInput() ([]byte, error) {
	return ReadAll(os.Stdin)
}

func ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
