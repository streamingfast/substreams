package service

import (
	"context"
	"io"
	"net/url"

	"github.com/streamingfast/dstore"
	"github.com/streamingfast/substreams/reqctx"
)

func NewMeteredStore(ctx context.Context, store dstore.Store) dstore.Store {
	meter := reqctx.BytesMeter(ctx)
	return &meteredStore{
		store: store,
		Meter: meter,
	}
}

type meteredReadCloser struct {
	r io.ReadCloser
	f func(n int)
}

func (m *meteredReadCloser) Read(p []byte) (n int, err error) {
	n, err = m.r.Read(p)
	if err == nil || err == io.EOF {
		m.f(n)
	}
	return n, err
}

func (m *meteredReadCloser) Close() error {
	return m.r.Close()
}

type meteredStore struct {
	store dstore.Store
	Meter reqctx.IBytesMeter
}

func (m *meteredStore) OpenObject(ctx context.Context, name string) (out io.ReadCloser, err error) {
	out, err = m.store.OpenObject(ctx, name)
	if err != nil {
		return nil, err
	}

	return &meteredReadCloser{r: out, f: m.Meter.AddBytesRead}, nil
}

func (m *meteredStore) WriteObject(ctx context.Context, base string, f io.Reader) (err error) {
	mf := &meteredReadCloser{r: io.NopCloser(f), f: m.Meter.AddBytesWritten}
	return m.store.WriteObject(ctx, base, mf)
}

func (m *meteredStore) SubStore(subFolder string) (dstore.Store, error) {
	s, err := m.store.SubStore(subFolder)
	if err != nil {
		return nil, err
	}

	ms := &meteredStore{
		store: s,
		Meter: m.Meter,
	}

	return ms, nil
}

///rest of dstore.Store methods simply forward to underlying store

func (m *meteredStore) FileExists(ctx context.Context, base string) (bool, error) {
	return m.store.FileExists(ctx, base)
}
func (m *meteredStore) ObjectPath(base string) string { return m.store.ObjectPath(base) }
func (m *meteredStore) ObjectURL(base string) string  { return m.store.ObjectURL(base) }
func (m *meteredStore) ObjectAttributes(ctx context.Context, base string) (*dstore.ObjectAttributes, error) {
	return m.store.ObjectAttributes(ctx, base)
}
func (m *meteredStore) PushLocalFile(ctx context.Context, localFile, toBaseName string) (err error) {
	return m.store.PushLocalFile(ctx, localFile, toBaseName)
}
func (m *meteredStore) CopyObject(ctx context.Context, src, dest string) error {
	return m.store.CopyObject(ctx, src, dest)
}
func (m *meteredStore) Overwrite() bool           { return m.store.Overwrite() }
func (m *meteredStore) SetOverwrite(enabled bool) { m.store.SetOverwrite(enabled) }
func (m *meteredStore) WalkFrom(ctx context.Context, prefix, startingPoint string, f func(filename string) (err error)) error {
	return m.store.WalkFrom(ctx, prefix, startingPoint, f)
}
func (m *meteredStore) Walk(ctx context.Context, prefix string, f func(filename string) (err error)) error {
	return m.store.Walk(ctx, prefix, f)
}
func (m *meteredStore) ListFiles(ctx context.Context, prefix string, max int) ([]string, error) {
	return m.store.ListFiles(ctx, prefix, max)
}
func (m *meteredStore) DeleteObject(ctx context.Context, base string) error {
	return m.store.DeleteObject(ctx, base)
}
func (m *meteredStore) BaseURL() *url.URL { return m.store.BaseURL() }
