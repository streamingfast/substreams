package store

import (
	"bytes"
	"context"
	"io"
	"math/big"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/storage/store/marshaller"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestPartialKV_Save_Load_Empty_MapNotNil(t *testing.T) {
	var writtenBytes []byte
	store := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		writtenBytes, err = io.ReadAll(f)
		return err
	})
	store.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		return io.NopCloser(bytes.NewBuffer(writtenBytes)), nil
	}

	kvs := &PartialKV{
		baseStore: &baseStore{
			kv: map[string][]byte{},

			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),

			Config: &Config{
				moduleInitialBlock: 0,
				objStore:           store,
			},
		},
	}

	file, writer, err := kvs.Save(123)
	require.NoError(t, err)

	err = writer.Write(context.Background())
	require.NoError(t, err)

	kvl := &PartialKV{
		baseStore: &baseStore{
			kv: map[string][]byte{},

			logger:     zap.NewNop(),
			marshaller: marshaller.Default(),

			Config: &Config{
				moduleInitialBlock: 0,
				objStore:           store,
			},
		},
	}

	err = kvl.Load(context.Background(), file)
	require.NoError(t, err)
	require.NotNilf(t, kvl.kv, "kvl.kv is nil")
}

func TestBigIntConversion(t *testing.T) {
	cases := []struct {
		name  string
		value *big.Int
	}{
		{
			name:  "sunny path",
			value: big.NewInt(123),
		},
		{
			name:  "other big int",
			value: big.NewInt(2391793721937),
		},
		{
			name:  "other big int",
			value: big.NewInt(-312312391793721937),
		},
		{
			name:  "zero big int",
			value: big.NewInt(0),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := bigIntToBytes(c.value)
			require.Equal(t, c.value, bytesToBigInt(result))
		})
	}
}

func TestInt64Conversion(t *testing.T) {
	cases := []struct {
		name  string
		value int64
	}{
		{
			name:  "sunny path",
			value: int64(123),
		},
		{
			name:  "int64 ",
			value: int64(239179379723),
		},
		{
			name:  "negative int64 ",
			value: int64(-2391932131218),
		},
		{
			name:  "zero int64 ",
			value: int64(0),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := int64ToBytes(c.value)
			require.Equal(t, c.value, valueToInt64(result))
		})
	}
}

func TestFloat64Conversion(t *testing.T) {
	cases := []struct {
		name  string
		value float64
	}{
		{
			name:  "sunny path",
			value: float64(123),
		},
		{
			name:  "float with many decimals",
			value: float64(12.328137817391723712983798127398127),
		},
		{
			name:  "negative float",
			value: float64(-12.2319739128391823812938192389281938129),
		},
		{
			name:  "zero float",
			value: float64(0),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := float64ToBytes(c.value)
			require.Equal(t, c.value, valueToFloat64(result))
		})
	}
}

func TestBigDecimal(t *testing.T) {
	cases := []struct {
		name  string
		value decimal.Decimal
	}{
		{
			name:  "sunny path",
			value: decimal.New(123, 0),
		},
		{
			name:  "big big decimal",
			value: decimal.New(123, 29301),
		},
		{
			name:  "negative big decimal",
			value: decimal.New(-123, 29301),
		},
		{
			name:  "zero big decimal",
			value: decimal.New(0, 29301),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := bigDecimalToBytes(c.value)
			newBigDecimal, err := valueToBigDecimal(result)
			require.NoError(t, err)
			require.Equal(t, c.value, newBigDecimal)
		})
	}
}
