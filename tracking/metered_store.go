package tracking

import (
	"context"
	"github.com/streamingfast/dstore"
	"io"
)

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

func NewMeteredStore(ctx context.Context, store dstore.Store) dstore.Store {
	meter := GetBytesMeter(ctx)
	return &MeteredStore{
		Store: store,
		Meter: meter,
	}
}

type MeteredStore struct {
	dstore.Store

	module string
	Meter  BytesMeter
}

func (m *MeteredStore) SetModule(module string) {
	m.module = module
}

func (m *MeteredStore) OpenObject(ctx context.Context, name string) (out io.ReadCloser, err error) {
	out, err = m.Store.OpenObject(ctx, name)
	if err != nil {
		return nil, err
	}

	fn := func(n int) {
		m.Meter.AddBytesRead(m.module, n)
	}

	return &meteredReadCloser{r: out, f: fn}, nil
}

func (m *MeteredStore) WriteObject(ctx context.Context, base string, f io.Reader) (err error) {
	fn := func(n int) {
		m.Meter.AddBytesWritten(m.module, n)
	}

	mf := &meteredReadCloser{r: io.NopCloser(f), f: fn}
	return m.Store.WriteObject(ctx, base, mf)
}

func (m *MeteredStore) SubStore(subFolder string) (dstore.Store, error) {
	s, err := m.Store.SubStore(subFolder)
	if err != nil {
		return nil, err
	}

	ms := &MeteredStore{
		Store: s,
		Meter: m.Meter,
	}
	if m.module != "" {
		ms.module = m.module
	}

	return ms, nil
}
