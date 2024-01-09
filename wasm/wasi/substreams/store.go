package substreams

import (
	"fmt"
	"math/big"
	"strings"

	"google.golang.org/protobuf/proto"
)

type StoreGet[T any] interface {
	GetAt(ord uint64, key string) (T, error)
	GetLast(key string) (T, error)
	GetFirst(key string) (T, error)
	HasAt(ord uint64, key string) bool
	HasLast(key string) bool
	HasFirst(key string) bool
}

type StoreSet[T any] interface {
	Set(ord uint64, key string, value T) error
	SetMany(ord uint64, keys []string, value T) error
}

type StoreSetIfNotExists[T any] interface {
	SetIfNotExists(ord uint64, key string, value T) error
	SetIfNotExistsMany(ord uint64, keys []string, value T) error
}

type StoreDelete interface {
	DeletePrefix(ord uint64, prefix string) error
}

type StoreAdd[T any] interface {
	Add(ord uint64, key string, value T) error
	AddMany(ord uint64, keys []string, value T) error
}

type StoreMin[T any] interface {
	Min(ord uint64, key string, value T) error
}

type StoreMax[T any] interface {
	Max(ord uint64, key string, value T) error
}

type StoreAppend[T any] interface {
	Append(ord uint64, key string, item T) error
	AppendAll(ord uint64, key string, items []T) error
}

type baseStore struct {
	idx uint32

	fileReadWriter FileReadWriter
}

func newBaseStore(idx uint32) *baseStore {
	return &baseStore{
		idx:            idx,
		fileReadWriter: &OSFileReadWriter{},
	}
}

func NewGetbaseStore(idx uint32) StoreGet[[]byte] {
	return newBaseStore(idx)
}

func NewSetbaseStore(idx uint32) StoreSet[[]byte] {
	return newBaseStore(idx)
}

func NewDeletebaseStore(idx uint32) StoreDelete {
	return newBaseStore(idx)
}

func (s *baseStore) GetAt(ord uint64, key string) ([]byte, error) {
	return s.readFromFile(fmt.Sprintf("/sys/stores/%d/read/at/%d/%s", s.idx, ord, key))
}

func (s *baseStore) GetLast(key string) ([]byte, error) {
	return s.readFromFile(fmt.Sprintf("/sys/stores/%d/read/last/%s", s.idx, key))
}

func (s *baseStore) GetFirst(key string) ([]byte, error) {
	return s.readFromFile(fmt.Sprintf("/sys/stores/%d/read/first/%s", s.idx, key))
}

func (s *baseStore) HasAt(ord uint64, key string) bool {
	data, err := s.readFromFile(fmt.Sprintf("/sys/stores/%d/check/at/%d/%s", s.idx, ord, key))
	if err != nil {
		return false
	}
	return isFound(data)
}

func (s *baseStore) HasLast(key string) bool {
	data, err := s.readFromFile(fmt.Sprintf("/sys/stores/%d/check/last/%s", s.idx, key))
	if err != nil {
		return false
	}
	return isFound(data)
}

func (s *baseStore) HasFirst(key string) bool {
	data, err := s.readFromFile(fmt.Sprintf("/sys/stores/%d/check/first/%s", s.idx, key))
	if err != nil {
		return false
	}
	return isFound(data)
}

func (s *baseStore) Set(ord uint64, key string, value []byte) error {
	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/write/at/%d/%s", s.idx, ord, key), value)
}

func (s *baseStore) SetMany(ord uint64, keys []string, value []byte) error {
	for _, key := range keys {
		err := s.writeToFile(fmt.Sprintf("/sys/stores/%d/write/at/%d/%s", s.idx, ord, key), value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *baseStore) SetIfNotExists(ord uint64, key string, value []byte) error {
	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/conditionalwrite/at/%d/%s", s.idx, ord, key), value)
}

func (s *baseStore) SetIfNotExistsMany(ord uint64, keys []string, value []byte) error {
	for _, key := range keys {
		err := s.writeToFile(fmt.Sprintf("/sys/stores/%d/conditionalwrite/at/%d/%s", s.idx, ord, key), value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *baseStore) DeletePrefix(ord uint64, prefix string) error {
	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/delete/prefix/%d/%s", s.idx, ord, prefix), []byte{})
}

func (s *baseStore) into(data []byte) ([]byte, error) {
	return data, nil
}

func (s *baseStore) from(item []byte) ([]byte, error) {
	return item, nil
}

func (s *baseStore) writeToFile(path string, value []byte) error {
	return s.fileReadWriter.WriteFile(path, value, 0644)
}

func (s *baseStore) readFromFile(path string) ([]byte, error) {
	return s.fileReadWriter.ReadFile(path)
}

type protoStore[M proto.Message] struct {
	baseStore *baseStore //for some reason, it does not like to be embedded for this struct. perhaps because of the generic type? not sure. no time to investigate at the moment
}

func NewGetProtoStore[M proto.Message](idx uint32) StoreGet[M] {
	return &protoStore[M]{
		baseStore: newBaseStore(idx),
	}
}

func NewSetProtoStore[M proto.Message](idx uint32) StoreSet[M] {
	return &protoStore[M]{
		baseStore: newBaseStore(idx),
	}
}

func NewDeleteProtoStore[M proto.Message](idx uint32) StoreDelete {
	return &protoStore[M]{
		baseStore: newBaseStore(idx),
	}
}

func newM[M proto.Message]() M {
	var m M // create a zero value of M
	return m
}

func (s *protoStore[M]) GetAt(ord uint64, key string) (M, error) {
	data, err := s.baseStore.GetAt(ord, key)
	if err != nil {
		return newM[M](), fmt.Errorf("reading key %q at %d: %w", key, ord, err)
	}
	return s.into(data)
}

func (s *protoStore[M]) GetLast(key string) (M, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return newM[M](), fmt.Errorf("reading last key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *protoStore[M]) GetFirst(key string) (M, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return newM[M](), fmt.Errorf("reading first key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *protoStore[M]) HasAt(ord uint64, key string) bool {
	return s.baseStore.HasAt(ord, key)
}

func (s *protoStore[M]) HasLast(key string) bool {
	return s.baseStore.HasLast(key)
}

func (s *protoStore[M]) HasFirst(key string) bool {
	return s.baseStore.HasFirst(key)
}

func (s *protoStore[M]) Set(ord uint64, key string, value M) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}
	return s.baseStore.Set(ord, key, v)
}

func (s *protoStore[M]) SetMany(ord uint64, keys []string, value M) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}
	return s.baseStore.SetMany(ord, keys, v)
}

func (s *protoStore[M]) SetIfNotExists(ord uint64, key string, value M) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}
	return s.baseStore.SetIfNotExists(ord, key, v)
}

func (s *protoStore[M]) SetIfNotExistsMany(ord uint64, keys []string, value M) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}
	return s.baseStore.SetIfNotExistsMany(ord, keys, v)
}

func (s *protoStore[M]) DeletePrefix(ord uint64, prefix string) error {
	return s.baseStore.DeletePrefix(ord, prefix)
}

func (s *protoStore[M]) into(data []byte) (M, error) {
	m := newM[M]()
	err := proto.Unmarshal(data, m)
	if err != nil {
		return m, fmt.Errorf("unmarshalling proto: %w", err)
	}
	return m, nil
}

func (s *protoStore[M]) from(item M) ([]byte, error) {
	return proto.Marshal(item)
}

type stringStore struct {
	*baseStore
}

func NewGetStringStore(idx uint32) StoreGet[string] {
	return &stringStore{
		baseStore: newBaseStore(idx),
	}
}

func NewSetStringStore(idx uint32) StoreSet[string] {
	return &stringStore{
		baseStore: newBaseStore(idx),
	}
}

func NewSetStringIfNotExistsStore(idx uint32) StoreSetIfNotExists[string] {
	return &stringStore{
		baseStore: newBaseStore(idx),
	}
}

func NewDeleteStringStore(idx uint32) StoreDelete {
	return &stringStore{
		baseStore: newBaseStore(idx),
	}
}

// GetAt retrieves a string value at the given ordinal and key.
func (s *stringStore) GetAt(ord uint64, key string) (string, error) {
	data, err := s.baseStore.GetAt(ord, key)
	if err != nil {
		return "", fmt.Errorf("reading key %q at %d: %w", key, ord, err)
	}
	return s.into(data)
}

// GetLast retrieves the last string value for the given key.
func (s *stringStore) GetLast(key string) (string, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return "", fmt.Errorf("reading last key %q: %w", key, err)
	}
	return s.into(data)
}

// GetFirst retrieves the first string value for the given key.
func (s *stringStore) GetFirst(key string) (string, error) {
	data, err := s.baseStore.GetFirst(key)
	if err != nil {
		return "", fmt.Errorf("reading last key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *stringStore) Set(ord uint64, key string, value string) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.Set(ord, key, v)
}

func (s *stringStore) SetMany(ord uint64, keys []string, value string) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetMany(ord, keys, v)
}

func (s *stringStore) SetIfNotExists(ord uint64, key string, value string) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetIfNotExists(ord, key, v)
}

func (s *stringStore) SetIfNotExistsMany(ord uint64, keys []string, value string) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetIfNotExistsMany(ord, keys, v)
}

func (s *stringStore) into(data []byte) (string, error) {
	return string(data), nil
}

func (s *stringStore) from(item string) ([]byte, error) {
	return []byte(item), nil
}

type int64Store struct {
	*baseStore
}

func NewGetInt64Store(idx uint32) StoreGet[int64] {
	return &int64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewSetInt64Store(idx uint32) StoreSet[int64] {
	return &int64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewSetInt64IfNotExistsStore(idx uint32) StoreSetIfNotExists[int64] {
	return &int64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewDeleteInt64Store(idx uint32) StoreDelete {
	return &int64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewAddInt64Store(idx uint32) StoreAdd[int64] {
	return &int64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewMinInt64Store(idx uint32) StoreMin[int64] {
	return &int64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewMaxInt64Store(idx uint32) StoreMax[int64] {
	return &int64Store{
		baseStore: newBaseStore(idx),
	}
}

func (s *int64Store) GetAt(ord uint64, key string) (int64, error) {
	data, err := s.baseStore.GetAt(ord, key)
	if err != nil {
		return 0, fmt.Errorf("reading key %q at %d: %w", key, ord, err)
	}
	return s.into(data)
}

func (s *int64Store) GetLast(key string) (int64, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return 0, fmt.Errorf("reading last key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *int64Store) GetFirst(key string) (int64, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return 0, fmt.Errorf("reading first key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *int64Store) Set(ord uint64, key string, value int64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.Set(ord, key, v)
}

func (s *int64Store) SetMany(ord uint64, keys []string, value int64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetMany(ord, keys, v)
}

func (s *int64Store) SetIfNotExists(ord uint64, key string, value int64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetIfNotExists(ord, key, v)
}

func (s *int64Store) SetIfNotExistsMany(ord uint64, keys []string, value int64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetIfNotExistsMany(ord, keys, v)
}

func (s *int64Store) Add(ord uint64, key string, value int64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/add/int64/%d/%s", s.idx, ord, key), v)
}

func (s *int64Store) AddMany(ord uint64, keys []string, value int64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	for _, key := range keys {
		err = s.writeToFile(fmt.Sprintf("/sys/stores/%d/add/int64/%d/%s", s.idx, ord, key), v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *int64Store) Min(ord uint64, key string, value int64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/min/int64/%d/%s", s.idx, ord, key), v)
}

func (s *int64Store) Max(ord uint64, key string, value int64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/max/int64/%d/%s", s.idx, ord, key), v)
}

func (s *int64Store) from(item int64) ([]byte, error) {
	return []byte(fmt.Sprintf("%d", item)), nil
}

func (s *int64Store) into(data []byte) (int64, error) {
	var i int64
	_, err := fmt.Sscanf(string(data), "%d", &i)
	if err != nil {
		return 0, fmt.Errorf("parsing int64 from string %v: %w", s, err)
	}
	return i, nil
}

type float64Store struct {
	*baseStore
}

func NewGetFloat64Store(idx uint32) StoreGet[float64] {
	return &float64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewSetFloat64Store(idx uint32) StoreSet[float64] {
	return &float64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewSetFloat64IfNotExistsStore(idx uint32) StoreSetIfNotExists[float64] {
	return &float64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewDeleteFloat64Store(idx uint32) StoreDelete {
	return &float64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewAddFloat64Store(idx uint32) StoreAdd[float64] {
	return &float64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewMinFloat64Store(idx uint32) StoreMin[float64] {
	return &float64Store{
		baseStore: newBaseStore(idx),
	}
}

func NewMaxFloat64Store(idx uint32) StoreMax[float64] {
	return &float64Store{
		baseStore: newBaseStore(idx),
	}
}

func (s *float64Store) GetAt(ord uint64, key string) (float64, error) {
	data, err := s.baseStore.GetAt(ord, key)
	if err != nil {
		return 0, fmt.Errorf("reading key %q at %d: %w", key, ord, err)
	}
	return s.into(data)
}

func (s *float64Store) GetLast(key string) (float64, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return 0, fmt.Errorf("reading last key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *float64Store) GetFirst(key string) (float64, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return 0, fmt.Errorf("reading first key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *float64Store) Set(ord uint64, key string, value float64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.Set(ord, key, v)
}

func (s *float64Store) SetMany(ord uint64, keys []string, value float64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetMany(ord, keys, v)
}

func (s *float64Store) SetIfNotExists(ord uint64, key string, value float64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetIfNotExists(ord, key, v)
}

func (s *float64Store) SetIfNotExistsMany(ord uint64, keys []string, value float64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetIfNotExistsMany(ord, keys, v)
}

func (s *float64Store) Add(ord uint64, key string, value float64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/add/float64/%d/%s", s.idx, ord, key), v)
}

func (s *float64Store) AddMany(ord uint64, keys []string, value float64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	for _, key := range keys {
		err = s.writeToFile(fmt.Sprintf("/sys/stores/%d/add/float64/%d/%s", s.idx, ord, key), v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *float64Store) Min(ord uint64, key string, value float64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/min/float64/%d/%s", s.idx, ord, key), v)
}

func (s *float64Store) Max(ord uint64, key string, value float64) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/max/float64/%d/%s", s.idx, ord, key), v)
}

func (s *float64Store) into(data []byte) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(string(data), "%f", &f)
	if err != nil {
		return 0, fmt.Errorf("parsing float64 from string %v: %w", s, err)
	}
	return f, nil
}

func (s *float64Store) from(item float64) ([]byte, error) {
	return []byte(fmt.Sprintf("%f", item)), nil
}

type bigIntStore struct {
	*baseStore
}

func NewGetBigIntStore(idx uint32) StoreGet[*big.Int] {
	return &bigIntStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewSetBigIntStore(idx uint32) StoreSet[*big.Int] {
	return &bigIntStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewSetBigIntIfNotExistsStore(idx uint32) StoreSetIfNotExists[*big.Int] {
	return &bigIntStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewDeleteBigIntStore(idx uint32) StoreDelete {
	return &bigIntStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewAddBigIntStore(idx uint32) StoreAdd[*big.Int] {
	return &bigIntStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewMinBigIntStore(idx uint32) StoreMin[*big.Int] {
	return &bigIntStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewMaxBigIntStore(idx uint32) StoreMax[*big.Int] {
	return &bigIntStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}

}

func (s *bigIntStore) GetAt(ord uint64, key string) (*big.Int, error) {
	data, err := s.baseStore.GetAt(ord, key)
	if err != nil {
		return big.NewInt(0), fmt.Errorf("reading key %q at %d: %w", key, ord, err)
	}
	return s.into(data)
}

func (s *bigIntStore) GetLast(key string) (*big.Int, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return big.NewInt(0), fmt.Errorf("reading last key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *bigIntStore) GetFirst(key string) (*big.Int, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return big.NewInt(0), fmt.Errorf("reading first key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *bigIntStore) Set(ord uint64, key string, value *big.Int) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.Set(ord, key, v)
}

func (s *bigIntStore) SetMany(ord uint64, keys []string, value *big.Int) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetMany(ord, keys, v)
}

func (s *bigIntStore) SetIfNotExists(ord uint64, key string, value *big.Int) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}
	return s.baseStore.SetIfNotExists(ord, key, v)
}

func (s *bigIntStore) SetIfNotExistsMany(ord uint64, keys []string, value *big.Int) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}
	return s.baseStore.SetIfNotExistsMany(ord, keys, v)
}

func (s *bigIntStore) Add(ord uint64, key string, value *big.Int) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/add/bigint/%d/%s", s.idx, ord, key), v)
}

func (s *bigIntStore) AddMany(ord uint64, keys []string, value *big.Int) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	for _, key := range keys {
		err = s.writeToFile(fmt.Sprintf("/sys/stores/%d/add/bigint/%d/%s", s.idx, ord, key), v)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *bigIntStore) Min(ord uint64, key string, value *big.Int) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/min/bigint/%d/%s", s.idx, ord, key), v)
}

func (s *bigIntStore) Max(ord uint64, key string, value *big.Int) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/max/bigint/%d/%s", s.idx, ord, key), v)
}

func (s *bigIntStore) into(data []byte) (*big.Int, error) {
	return new(big.Int).SetBytes(data), nil
}

func (s *bigIntStore) from(item *big.Int) ([]byte, error) {
	return item.Bytes(), nil
}

type bigFloatStore struct {
	*baseStore
}

func NewGetBigFloatStore(idx uint32) StoreGet[*big.Float] {
	return &bigFloatStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewSetBigFloatStore(idx uint32) StoreSet[*big.Float] {
	return &bigFloatStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewSetBigFloatIfNotExistsStore(idx uint32) StoreSetIfNotExists[*big.Float] {
	return &bigFloatStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewDeleteBigFloatStore(idx uint32) StoreDelete {
	return &bigFloatStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewAddBigFloatStore(idx uint32) StoreAdd[*big.Float] {
	return &bigFloatStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewMinBigFloatStore(idx uint32) StoreMin[*big.Float] {
	return &bigFloatStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewMaxBigFloatStore(idx uint32) StoreMax[*big.Float] {
	return &bigFloatStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func (s *bigFloatStore) GetAt(ord uint64, key string) (*big.Float, error) {
	data, err := s.baseStore.GetAt(ord, key)
	if err != nil {
		return big.NewFloat(0), fmt.Errorf("reading key %q at %d: %w", key, ord, err)
	}

	return s.into(data)
}

func (s *bigFloatStore) GetLast(key string) (*big.Float, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return big.NewFloat(0), fmt.Errorf("reading last key %q: %w", key, err)
	}

	return s.into(data)
}

func (s *bigFloatStore) GetFirst(key string) (*big.Float, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return big.NewFloat(0), fmt.Errorf("reading first key %q: %w", key, err)
	}

	return s.into(data)
}

func (s *bigFloatStore) Set(ord uint64, key string, value *big.Float) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.Set(ord, key, v)
}

func (s *bigFloatStore) SetMany(ord uint64, keys []string, value *big.Float) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.baseStore.SetMany(ord, keys, v)
}

func (s *bigFloatStore) SetIfNotExists(ord uint64, key string, value *big.Float) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}
	return s.baseStore.SetIfNotExists(ord, key, v)

}

func (s *bigFloatStore) SetIfNotExistsMany(ord uint64, keys []string, value *big.Float) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}
	return s.baseStore.SetIfNotExistsMany(ord, keys, v)
}

func (s *bigFloatStore) Add(ord uint64, key string, value *big.Float) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/add/bigfloat/%d/%s", s.idx, ord, key), v)
}

func (s *bigFloatStore) AddMany(ord uint64, keys []string, value *big.Float) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	for _, key := range keys {
		err = s.writeToFile(fmt.Sprintf("/sys/stores/%d/add/bigfloat/%d/%s", s.idx, ord, key), v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *bigFloatStore) Min(ord uint64, key string, value *big.Float) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/min/bigfloat/%d/%s", s.idx, ord, key), v)
}

func (s *bigFloatStore) Max(ord uint64, key string, value *big.Float) error {
	v, err := s.from(value)
	if err != nil {
		return fmt.Errorf("converting value %v: %w", value, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/max/bigfloat/%d/%s", s.idx, ord, key), v)
}

func (s *bigFloatStore) into(data []byte) (*big.Float, error) {
	f, ok := new(big.Float).SetString(string(data))
	if !ok {
		return nil, fmt.Errorf("unable to parse big float %q", string(data))
	}
	return f, nil
}

func (s *bigFloatStore) from(item *big.Float) ([]byte, error) {
	return []byte(item.String()), nil
}

type arrayStore struct {
	*baseStore
}

func NewGetArrayStore(idx uint32) StoreGet[[]string] {
	return &arrayStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func NewAppendArrayStore(idx uint32) StoreAppend[string] {
	return &arrayStore{
		baseStore: &baseStore{
			idx: idx,
		},
	}
}

func (s *arrayStore) GetAt(ord uint64, key string) ([]string, error) {
	data, err := s.baseStore.GetAt(ord, key)
	if err != nil {
		return nil, fmt.Errorf("reading key %q at %d: %w", key, ord, err)
	}
	return s.into(data)
}

func (s *arrayStore) GetLast(key string) ([]string, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return nil, fmt.Errorf("reading last key %q: %w", key, err)
	}
	return s.into(data)
}

func (s *arrayStore) GetFirst(key string) ([]string, error) {
	data, err := s.baseStore.GetLast(key)
	if err != nil {
		return nil, fmt.Errorf("reading first key %q: %w", key, err)
	}
	return s.into(data)
}

// todo: check this
func (s *arrayStore) Append(ord uint64, key string, item string) error {
	v, err := s.from([]string{item})
	if err != nil {
		return fmt.Errorf("converting value %v: %w", item, err)
	}

	return s.writeToFile(fmt.Sprintf("/sys/stores/%d/append/at/%d/%s", s.idx, ord, key), v)
}

// todo: check this
func (s *arrayStore) AppendAll(ord uint64, key string, items []string) error {
	for _, item := range items {
		vi, err := s.from([]string{item})
		if err != nil {
			return fmt.Errorf("converting value %v: %w", item, err)
		}

		err = s.writeToFile(fmt.Sprintf("/sys/stores/%d/append/at/%d/%s", s.idx, ord, key), vi)
		if err != nil {
			return fmt.Errorf("appending item %q: %w", item, err)
		}
	}

	return nil
}

func (s *arrayStore) into(data []byte) ([]string, error) {
	if len(data) == 0 {
		return []string{}, nil
	}
	if len(data) == 1 {
		return []string{string(data)}, nil
	}

	return strings.Split(string(data), ","), nil
}

func (s *arrayStore) from(item []string) ([]byte, error) {
	if len(item) == 0 {
		return []byte{}, nil
	}
	if len(item) == 1 {
		return []byte(item[0]), nil
	}

	return []byte(strings.Join(item, ",")), nil
}

func isFound(data []byte) bool {
	return data[0] == 1
}
