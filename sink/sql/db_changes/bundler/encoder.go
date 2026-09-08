package bundler

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/golang/protobuf/proto"
)

type Encoder func(proto.Message) ([]byte, error)

func JSONLEncode(message proto.Message) ([]byte, error) {
	buf := []byte{}
	data, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	buf = append(buf, data...)
	buf = append(buf, byte('\n'))
	return buf, nil
}

func CSVEncode(message map[string]string) ([]byte, error) {
	keys := make([]string, 0, len(message))
	for k := range message {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	row := make([]string, 0, len(keys))
	for _, key := range keys {
		row = append(row, message[key])
	}

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(row); err != nil {
		return nil, err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
