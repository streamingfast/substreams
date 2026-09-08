package wasm

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/mr-tron/base58"
	"github.com/shopspring/decimal"
	"github.com/streamingfast/dauth"
	"github.com/streamingfast/substreams/metrics"
	pbmodel "github.com/streamingfast/substreams/pb/sf/substreams/foundational-store/model/v2"
	pbservice "github.com/streamingfast/substreams/pb/sf/substreams/foundational-store/service/v2"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/reqctx"
	"github.com/streamingfast/substreams/storage/store"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

var ErrWasmDeterministicExec = errors.New("wasm execution failed deterministically")
var ErrFoundationalStoreCanceled = errors.New("foundational store request canceled")

// ErrFoundationalStoreFatal indicates a foundational store call failed in a way
// that retrying will not fix (authentication failure, organization id mismatch,
// or the store being unreachable). It is non-deterministic so it bubbles up to
// the user without being cached.
var ErrFoundationalStoreFatal = errors.New("foundational store request failed")

// Foundational store retry configuration constants
const (
	foundationalStoreRetryDelay  = 100 * time.Millisecond
	foundationalStoreMaxWaitTime = 30 * time.Second
)

// foundationalStoreMaxUnreachableRetries bounds how many consecutive
// "unreachable" (codes.Unavailable) responses we tolerate before giving up, so a
// transient blip or rolling restart is absorbed but a truly down or
// misconfigured endpoint fails fast instead of retrying until the global
// deadline. At foundationalStoreRetryDelay each, this is ~30s of budget. It is a
// var (not a const) so tests can lower it.
var foundationalStoreMaxUnreachableRetries = 300

type Call struct {
	ctx        context.Context
	Clock      *pbsubstreams.Clock // Used by WASM extensions
	ModuleName string
	Entrypoint string

	inputStores  []store.Reader
	outputStore  store.Store
	updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy

	foundationalStores []pbservice.StoreClient

	valueType string

	returnValue        []byte
	skipEmptyOutput    bool
	canSkipEmptyOutput bool // if false, takes precedence over skipEmptyOutput and disables that feature
	PanicError         *PanicError

	Logs           []string
	LogsByteCount  uint64
	logsTruncated  bool
	ExecutionStack []string
	stats          *metrics.Stats
}

func NewCall(ctx context.Context, clock *pbsubstreams.Clock, moduleName string, entrypoint string, stats *metrics.Stats, arguments []Argument, canSkipEmptyOutput bool, foundationalStores []pbservice.StoreClient) *Call {
	call := &Call{
		ctx:                ctx,
		Clock:              clock,
		ModuleName:         moduleName,
		Entrypoint:         entrypoint,
		stats:              stats,
		canSkipEmptyOutput: canSkipEmptyOutput,
		foundationalStores: foundationalStores,
	}

	for _, input := range arguments {
		switch v := input.(type) {
		case *StoreWriterOutput:
			v.Store.Reset()
			call.outputStore = v.Store
			call.updatePolicy = v.UpdatePolicy
			call.valueType = v.ValueType
		case *StoreReaderInput:
			call.inputStores = append(call.inputStores, v.Store)
		case *MapInput, *StoreDeltaInput, *ParamsInput, *SourceInput:
			// Handled in ExecuteNewCall()
		case *FoundationalStoreInput:
		default:
			panic(fmt.Sprintf("unknown wasm argument type %T", v))
		}
	}

	return call
}

func (c *Call) Err() error {
	if c.PanicError != nil {
		return c.PanicError
	}
	return nil
}

func (c *Call) Output() []byte {
	return c.returnValue
}
func (c *Call) SetReturnValue(msg []byte) {
	c.returnValue = make([]byte, len(msg))
	copy(c.returnValue, msg)
}

func (c *Call) SkipEmptyOutput() {
	c.skipEmptyOutput = true
}

func (c *Call) CanSkipOutput() bool {
	return c.canSkipEmptyOutput && c.skipEmptyOutput && len(c.returnValue) == 0
}

func (c *Call) SetPanicError(message string, filename string, lineNo int, colNo int) {
	c.PanicError = NewPanicError(message, filename, lineNo, colNo)
}

func (c *Call) AppendLog(message string) {

	// sanity: skipping logs after MaxTotalLogsByteCount
	if c.ReachedLogsMaxByteCount() {
		if !c.logsTruncated {
			c.Logs = append(c.Logs, fmt.Sprintf("some logs were truncated (above %s bytes) ...", maxTotalLogsByteCountHumanized))
			c.logsTruncated = true
		}
		return
	}

	// len(<string>) in Go count number of bytes and not characters, so we are good here
	if len(message) > maxLogByteCount {
		message = message[:maxLogByteCount] + fmt.Sprintf(" (... %d bytes)", len(message)-maxLogByteCount)
	}

	c.Logs = append(c.Logs, message)

	c.LogsByteCount += uint64(len(message))

}

func (c *Call) SetOutputStore(store store.Store) {
	c.outputStore = store
}

const maxLogByteCount = 512 * 1024            // 512 KiB
const maxTotalLogsByteCount = 2 * 1024 * 1024 // 2 MiB
var maxTotalLogsByteCountHumanized = humanize.IBytes(maxTotalLogsByteCount)

func (c *Call) ReachedLogsMaxByteCount() bool {
	return c.LogsByteCount >= maxTotalLogsByteCount
}

// DoClock backs the `context::clock` intrinsic, returning the block clock encoded as a
// `sf.substreams.v1.Clock` protobuf message.
func (c *Call) DoClock() []byte {
	out, err := proto.Marshal(c.Clock)
	if err != nil {
		c.PanicNonDeterministicError(fmt.Errorf("marshalling clock: %w", err))
	}

	return out
}

func (c *Call) DoSet(ord uint64, key string, value []byte) {
	now := time.Now()
	c.validateSimple("set", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET, key)
	c.outputStore.SetBytes(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetIfNotExists(ord uint64, key string, value []byte) {
	now := time.Now()
	c.validateSimple("set_if_not_exists", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS, key)
	c.outputStore.SetBytesIfNotExists(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAppend(ord uint64, key string, value []byte) {
	now := time.Now()
	c.validateSimple("append", pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND, key)
	c.outputStore.Append(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoDeletePrefix(ord uint64, prefix string) {
	now := time.Now()
	c.outputStore.DeletePrefix(ord, prefix)
	c.stats.RecordModuleWasmStoreDeletePrefix(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAddBigInt(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithValueType("add_bigint", pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigint", key)

	toAdd, _ := new(big.Int).SetString(value, 10)
	c.outputStore.SumBigInt(ord, key, toAdd)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAddBigDecimal(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithTwoValueTypes("add_bigdecimal", pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "bigdecimal", "bigfloat", key)

	toAdd, err := decimal.NewFromString(string(value))
	if err != nil {
		c.PanicDeterministicError("parsing bigdecimal: %w", err)
	}
	c.outputStore.SumBigDecimal(ord, key, toAdd.Truncate(34))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAddInt64(ord uint64, key string, value int64) {
	now := time.Now()
	c.validateWithValueType("add_int64", pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "int64", key)
	c.outputStore.SumInt64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoAddFloat64(ord uint64, key string, value float64) {
	now := time.Now()
	c.validateWithValueType("add_float64", pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD, "float64", key)
	c.outputStore.SumFloat64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetSumBigInt(ord uint64, key string, value string) {
	now := time.Now()
	c.validateSimple("set_sum_bigint", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM, key)

	c.outputStore.SetSumBigInt(ord, key, []byte(value))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetSumBigDecimal(ord uint64, key string, value string) {
	now := time.Now()
	c.validateSimple("set_sum_bigdecimal", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM, key)

	c.outputStore.SetSumBigDecimal(ord, key, []byte(value))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetSumInt64(ord uint64, key string, value string) {
	now := time.Now()
	c.validateSimple("set_sum_int64", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM, key)

	c.outputStore.SetSumInt64(ord, key, []byte(value))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetSumFloat64(ord uint64, key string, value string) {
	now := time.Now()
	c.validateSimple("set_sum_float64", pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_SUM, key)

	c.outputStore.SetSumFloat64(ord, key, []byte(value))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMinInt64(ord uint64, key string, value int64) {
	now := time.Now()
	c.validateWithValueType("set_min_int64", pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "int64", key)
	c.outputStore.SetMinInt64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMinBigInt(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithValueType("set_min_bigint", pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "bigint", key)
	toSet, _ := new(big.Int).SetString(value, 10)
	c.outputStore.SetMinBigInt(ord, key, toSet)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMinFloat64(ord uint64, key string, value float64) {
	now := time.Now()
	c.validateWithValueType("set_min_float64", pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "float64", key)
	c.outputStore.SetMinFloat64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMinBigDecimal(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithTwoValueTypes("set_min_bigdecimal", pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN, "bigdecimal", "bigfloat", key)
	toAdd, err := decimal.NewFromString(value)
	if err != nil {
		c.PanicDeterministicError("parsing bigdecimal: %w", err)
	}
	c.outputStore.SetMinBigDecimal(ord, key, toAdd.Truncate(34))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMaxInt64(ord uint64, key string, value int64) {
	now := time.Now()
	c.validateWithValueType("set_max_int64", pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "int64", key)
	c.outputStore.SetMaxInt64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMaxBigInt(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithValueType("set_max_bigint", pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigint", key)
	toSet, _ := new(big.Int).SetString(value, 10)
	c.outputStore.SetMaxBigInt(ord, key, toSet)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMaxFloat64(ord uint64, key string, value float64) {
	now := time.Now()
	c.validateWithValueType("set_max_float64", pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "float64", key)
	c.outputStore.SetMaxFloat64(ord, key, value)
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}
func (c *Call) DoSetMaxBigDecimal(ord uint64, key string, value string) {
	now := time.Now()
	c.validateWithTwoValueTypes("set_max_bigdecimal", pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX, "bigdecimal", "bigfloat", key)
	toAdd, err := decimal.NewFromString(value)
	if err != nil {
		c.PanicDeterministicError("parsing bigdecimal: %w", err)
	}
	c.outputStore.SetMaxBigDecimal(ord, key, toAdd.Truncate(34))
	c.stats.RecordModuleWasmStoreWrite(c.ModuleName, c.outputStore.SizeBytes(), time.Since(now))
}

func (c *Call) DoGetAt(storeIndex int, ord uint64, key string) (value []byte, found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "get_at")
	readStore := c.inputStores[storeIndex]
	return readStore.GetAt(ord, key)
}

func (c *Call) DoHasAt(storeIndex int, ord uint64, key string) (found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "has_at")
	readStore := c.inputStores[storeIndex]
	return readStore.HasAt(ord, key)
}

func (c *Call) DoGetFirst(storeIndex int, key string) (value []byte, found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "get_first")
	readStore := c.inputStores[storeIndex]
	return readStore.GetFirst(key)
}

func (c *Call) DoHasFirst(storeIndex int, key string) (found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "has_first")
	readStore := c.inputStores[storeIndex]
	return readStore.HasFirst(key)
}

func (c *Call) DoGetLast(storeIndex int, key string) (value []byte, found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "get_last")
	readStore := c.inputStores[storeIndex]
	return readStore.GetLast(key)
}

func (c *Call) DoHasLast(storeIndex int, key string) (found bool) {
	now := time.Now()
	defer func() { c.stats.RecordModuleWasmStoreRead(c.ModuleName, time.Since(now)) }()
	c.validateStoreIndex(storeIndex, "has_last")
	readStore := c.inputStores[storeIndex]
	return readStore.HasLast(key)
}

// foundationalStoreOutgoingContext builds the outgoing gRPC context used to call
// a (possibly hosted) foundational store.
//
// It forwards the trusted identity headers resolved for this request (via dauth)
// and, explicitly, the x-organization-id header so a hosted store's internal
// (trust-based) listener can authorize the request without requiring the
// end-user JWT, which is consumed upstream and never reaches tier1. For hosted
// stores whose public endpoint still expects the original Bearer token, the raw
// authorization header is forwarded when present in the incoming context.
func foundationalStoreOutgoingContext(ctx context.Context, logger *zap.Logger) context.Context {
	auth := dauth.FromContext(ctx)
	callCtx := auth.ToOutgoingGRPCContext(ctx)

	if orgID := auth.OrganizationID(); orgID != "" {
		callCtx = metadata.AppendToOutgoingContext(callCtx, dauth.HeaderNewOrganizationID, orgID)
	}

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if auths := md.Get("authorization"); len(auths) > 0 {
			if ce := logger.Check(zap.DebugLevel, "forwarding foundational store authorization header"); ce != nil {
				ce.Write(
					zap.Int("token_length", len(auths[0])),
					zap.String("organization_id", auth.OrganizationID()),
				)
			}
			callCtx = metadata.AppendToOutgoingContext(callCtx, "authorization", auths[0])
		}
	}

	return callCtx
}

func (c *Call) DoFoundationalStoreGet(index uint32, keys *pbmodel.Keys) *pbmodel.QueriedEntries {
	c.validateFoundationalStoreIndex(int(index), "foundational_store_get_all")

	logger := reqctx.Logger(c.ctx).With(zap.Uint32("foundational_store_index", index))
	logger = logger.Named("DoFoundationalStoreGet")

	unreachableRetries := 0
	for {
		callCtx := foundationalStoreOutgoingContext(c.ctx, logger)
		ctx, cancel := context.WithTimeoutCause(callCtx, foundationalStoreMaxWaitTime, fmt.Errorf("foundational store get_all timeout after %s", foundationalStoreMaxWaitTime))
		defer cancel()

		request := &pbservice.GetRequest{
			BlockNumber: c.Clock.Number,
			BlockHash:   decodeHashString(c.Clock.Id),
			Keys:        keys.Keys,
		}

		resp, err := c.foundationalStores[index].Get(ctx, request)

		logger.Debug("foundational store GetAll result", zap.Bool("block_reached", resp != nil && resp.BlockReached), zap.Error(err))
		if err != nil {
			// Check if parent call context was cancelled (we do not check the local ctx as a timeout for this one means retry)
			if c.ctx.Err() != nil {
				logger.Debug("foundational store get_all cancelled due to upstream disconnect",
					zap.Stringer("block", c.Clock.AsBlockRef()),
					zap.Error(context.Cause(c.ctx)),
				)
				c.PanicNonDeterministicError(ErrFoundationalStoreCanceled)
			}

			// Fatal errors (auth, org id mismatch, prolonged unreachability) bubble
			// up as non-deterministic instead of retrying until the global deadline.
			c.handleFoundationalStoreError(err, &unreachableRetries)

			logger.Warn("failed to call foundational store", zap.Error(err))
			time.Sleep(foundationalStoreRetryDelay)
			continue
		}

		if resp.BlockReached {
			return resp.Entries
		}

		time.Sleep(foundationalStoreRetryDelay)
	}
}
func (c *Call) DoFoundationalStoreGetFirst(index uint32, keys *pbmodel.Keys) *pbmodel.QueriedEntries {
	c.validateFoundationalStoreIndex(int(index), "foundational_store_get_all")

	logger := reqctx.Logger(c.ctx).With(zap.Uint32("foundational_store_index", index))

	unreachableRetries := 0
	for {
		callCtx := foundationalStoreOutgoingContext(c.ctx, logger)
		ctx, cancel := context.WithTimeoutCause(callCtx, foundationalStoreMaxWaitTime, fmt.Errorf("foundational store get_all timeout after %s", foundationalStoreMaxWaitTime))
		defer cancel()

		request := &pbservice.GetRequest{
			BlockNumber: c.Clock.Number,
			BlockHash:   decodeHashString(c.Clock.Id),
			Keys:        keys.Keys,
		}

		resp, err := c.foundationalStores[index].GetFirst(ctx, request)

		logger.Debug("foundational store GetAllFirst result", zap.Bool("block_reached", resp != nil && resp.BlockReached), zap.Error(err))
		if err != nil {
			// Check if parent call context was cancelled (we do not check the local ctx as a timeout for this one means retry)
			if c.ctx.Err() != nil {
				logger.Debug("foundational store get_all cancelled due to upstream disconnect",
					zap.Stringer("block", c.Clock.AsBlockRef()),
					zap.Error(context.Cause(c.ctx)),
				)
				c.PanicNonDeterministicError(ErrFoundationalStoreCanceled)
			}

			// Fatal errors (auth, org id mismatch, prolonged unreachability) bubble
			// up as non-deterministic instead of retrying until the global deadline.
			c.handleFoundationalStoreError(err, &unreachableRetries)

			logger.Warn("failed to call foundational store", zap.Error(err))
			time.Sleep(foundationalStoreRetryDelay)
			continue
		}

		if resp.BlockReached {
			return resp.Entries
		}

		time.Sleep(foundationalStoreRetryDelay)
	}
}

// handleFoundationalStoreError classifies a gRPC error returned by a foundational
// store call. Fatal errors that retrying cannot fix (authentication failure,
// organization id mismatch, or the store being unreachable for too long) panic
// with a non-deterministic error so they bubble up to the user uncached. Other
// errors return so the caller keeps retrying (e.g. a local request timeout, or
// the store not having reached the requested block yet). unreachableRetries
// accumulates consecutive codes.Unavailable responses across the retry loop.
func (c *Call) handleFoundationalStoreError(err error, unreachableRetries *int) {
	switch status.Code(err) {
	case codes.Unauthenticated, codes.PermissionDenied:
		c.PanicNonDeterministicError(fmt.Errorf("%w: %s", ErrFoundationalStoreFatal, err))
	case codes.Unavailable:
		*unreachableRetries++
		if *unreachableRetries > foundationalStoreMaxUnreachableRetries {
			c.PanicNonDeterministicError(fmt.Errorf("%w: store unreachable after %d attempts: %s", ErrFoundationalStoreFatal, *unreachableRetries, err))
		}
	}
}

func (c *Call) validateStoreIndex(storeIndex int, stateFunc string) {
	if storeIndex+1 > len(c.inputStores) {
		c.PanicDeterministicError("%q failed: invalid store index %d, %d stores declared", stateFunc, storeIndex, len(c.inputStores))
	}
}

func (c *Call) validateFoundationalStoreIndex(storeIndex int, stateFunc string) {
	if storeIndex+1 > len(c.foundationalStores) {
		c.PanicDeterministicError("%q failed: invalid foundational store index %d, %d stores declared", stateFunc, storeIndex, len(c.foundationalStores))
	}
}

type foundationalStoreRequest interface {
	GetBlockNumber() uint64
	GetBlockHash() []byte
}

// validateFoundationStoreRequest validates the received request to ensure the block_number and block_hash
// are not modified by the WASM module. If they are, it panics with a deterministic error as it's invalid for the
// users to try to query other blocks than the current one.
func (c *Call) validateFoundationStoreRequest(request foundationalStoreRequest, operation string) {
	if request.GetBlockNumber() != 0 {
		c.PanicDeterministicError("%s: block_number cannot be modified, must be 0, got %d", operation, request.GetBlockNumber())
	}
	if len(request.GetBlockHash()) != 0 {
		c.PanicDeterministicError("%s: block_hash cannot be modified, must be empty, got %d bytes", operation, len(request.GetBlockHash()))
	}
}

func (c *Call) validateSimple(stateFunc string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, key string) {
	if c.updatePolicy != updatePolicy {
		c.returnInvalidPolicy(stateFunc, fmt.Sprintf(`updatePolicy == %q`, policyMap[updatePolicy]))
	}
}

func (c *Call) validateWithValueType(stateFunc string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType string, key string) {
	if c.updatePolicy != updatePolicy || c.valueType != valueType {
		c.returnInvalidPolicy(stateFunc, fmt.Sprintf(`updatePolicy == %q and valueType == %q`, policyMap[updatePolicy], valueType))
	}
}

func (c *Call) validateWithTwoValueTypes(stateFunc string, updatePolicy pbsubstreams.Module_KindStore_UpdatePolicy, valueType1, valueType2 string, key string) {
	if c.updatePolicy != updatePolicy || (c.valueType != valueType1 && c.valueType != valueType2) {
		c.returnInvalidPolicy(stateFunc, fmt.Sprintf(`updatePolicy == %q and valueType == %q`, policyMap[updatePolicy], valueType1))
	}
}

func (c *Call) returnInvalidPolicy(stateFunc, policy string) {
	panic(fmt.Errorf("module %q: invalid store operation %q, only valid for stores with %s, %w", c.ModuleName, stateFunc, policy, ErrWasmDeterministicExec))
}

// deterministicError creates a wrapped error indicating a deterministic execution error
// and include the module name and the actual error.
func (c *Call) deterministicError(format string, args ...any) error {
	return fmt.Errorf("module %q: %w, %w", c.ModuleName, fmt.Errorf(format, args...), ErrWasmDeterministicExec)
}

// PanicDeterministicError panics with a wrapped error indicating a deterministic execution error
// and include the module name and the actual error.
func (c *Call) PanicDeterministicError(format string, args ...any) {
	panic(c.deterministicError(format, args...))
}

// PanicNonDeterministicError panics with a wrapped error indicating a non-deterministic execution error
// and include the module name and the actual error. To determine if the error is deterministic,
// one will need to inspect [err] itself.
//
// So it is the caller responsibility to decide if the error is deterministic or not.
func (c *Call) PanicNonDeterministicError(err error) {
	panic(fmt.Errorf("module %q: %w", c.ModuleName, err))
}

var policyMap = map[pbsubstreams.Module_KindStore_UpdatePolicy]string{
	pbsubstreams.Module_KindStore_UPDATE_POLICY_UNSET:             "unset",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_SET:               "set",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_SET_IF_NOT_EXISTS: "set_if_not_exists",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_ADD:               "add",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_MIN:               "min",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_MAX:               "max",
	pbsubstreams.Module_KindStore_UPDATE_POLICY_APPEND:            "append",
}

// Attempts to decode a hash string into bytes using multiple formats
func decodeHashString(hashStr string) []byte {
	if hashStr == "" {
		return nil
	}

	// hex with 0x prefix
	if strings.HasPrefix(hashStr, "0x") && len(hashStr) == 66 {
		if decoded, err := hex.DecodeString(hashStr[2:]); err == nil {
			return decoded
		}
	}

	//hex without prefix
	if len(hashStr) == 64 {
		if decoded, err := hex.DecodeString(strings.ToLower(hashStr)); err == nil {
			return decoded
		}
	}

	// base58
	if decoded, err := base58.Decode(hashStr); err == nil && len(decoded) == 32 {
		return decoded
	}

	// Fallback: return nil if no format worked
	return nil
}
