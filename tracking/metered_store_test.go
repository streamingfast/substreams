package tracking

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/streamingfast/dstore"
	"github.com/stretchr/testify/require"
)

func testReader() io.Reader {
	return strings.NewReader("hello, yes this is substreams")
}

func TestMeteredReadCloser_Read(t *testing.T) {
	m := NewBytesMeter()
	rc := io.NopCloser(testReader())

	module := "abc123"

	fn := func(n int) {
		m.AddBytesRead(module, n)
	}

	mrc := &meteredReadCloser{r: rc, f: fn}

	res, err := io.ReadAll(mrc)
	require.Nil(t, err)

	actual := m.BytesRead(module)
	expected := uint64(len(res))
	require.Equal(t, expected, actual)
}

func TestMeteredStore_OpenObject(t *testing.T) {
	ctx := context.Background()
	ctx = WithBytesMeter(ctx, NewBytesMeter())

	module := "abc123"

	baseStore := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		return nil
	})
	baseStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		r := bytes.NewReader([]byte("hello world"))
		rc := io.NopCloser(r)

		return rc, nil
	}

	store := NewMeteredStore(ctx, baseStore)
	store.(*MeteredStore).SetModule(module)

	rc, err := store.OpenObject(ctx, "test")
	require.Nil(t, err)

	res, err := io.ReadAll(rc)
	require.Nil(t, err)

	m := GetBytesMeter(ctx)

	actual := m.BytesRead(module)
	expected := uint64(len(res))
	require.Equal(t, expected, actual)
}

func TestMeteredStore_WriteObject(t *testing.T) {
	ctx := context.Background()
	ctx = WithBytesMeter(ctx, NewBytesMeter())

	module := "abc123"

	baseStore := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		return nil
	})

	var written int64
	baseStore.WriteObjectFunc = func(ctx context.Context, base string, f io.Reader) (err error) {
		buf := bytes.NewBuffer(nil)
		written, err = io.Copy(buf, f)
		return err
	}

	store := NewMeteredStore(ctx, baseStore)
	store.(*MeteredStore).SetModule(module)

	err := store.WriteObject(ctx, "test", testReader())
	require.Nil(t, err)

	m := GetBytesMeter(ctx)

	actual := m.BytesWritten(module)
	expected := uint64(written)
	require.Equal(t, expected, actual)
}

func TestMeteredStore_SubStore(t *testing.T) {
	ctx := context.Background()
	ctx = WithBytesMeter(ctx, NewBytesMeter())

	module := "abc123"

	baseStore := dstore.NewMockStore(func(base string, f io.Reader) (err error) {
		return nil
	})

	var written int64
	baseStore.WriteObjectFunc = func(ctx context.Context, base string, f io.Reader) (err error) {
		buf := bytes.NewBuffer(nil)
		written, err = io.Copy(buf, f)
		return err
	}
	baseStore.OpenObjectFunc = func(ctx context.Context, name string) (out io.ReadCloser, err error) {
		rc := io.NopCloser(testReader())

		return rc, nil
	}

	store := NewMeteredStore(ctx, baseStore)
	store.(*MeteredStore).SetModule(module)

	subStore1, err := store.SubStore("foo")
	require.Nil(t, err)

	err = subStore1.WriteObject(ctx, "_", testReader())
	require.Nil(t, err)

	subStore2, err := store.SubStore("bar")
	require.Nil(t, err)

	rc, err := subStore2.OpenObject(ctx, "_")
	require.Nil(t, err)

	res, err := io.ReadAll(rc)
	require.Nil(t, err)

	m := GetBytesMeter(ctx)

	actualWritten := m.BytesWritten(module)
	expectedWritten := uint64(written)
	require.Equal(t, expectedWritten, actualWritten)

	actualRead := m.BytesRead(module)
	expectedRead := uint64(len(res))
	require.Equal(t, actualRead, expectedRead)
}
