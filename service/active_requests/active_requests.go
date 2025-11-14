package active_requests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/dustin/go-humanize"
	"github.com/pbnjay/memory"
	"go.uber.org/zap"
)

var GB uint64 = 1024 * 1024 * 1024
var enforceStoreSizeLimitPerRequest = os.Getenv("SUBSTREAMS_ENFORCE_STORE_SIZE_LIMIT_PER_REQUEST") == "true"
var storeSizeLimitPerRequest = parseUint64EnvVar("SUBSTREAMS_STORE_SIZE_LIMIT_PER_REQUEST", 5*GB) // limit size of all loaded stores for a single request, in bytes

var enforceTotalStoreSizeLimit = os.Getenv("SUBSTREAMS_ENFORCE_TOTAL_STORE_SIZE_LIMIT") == "true"
var totalStoreSizeLimitPercent = parseUint64EnvVar("SUBSTREAMS_TOTAL_STORE_SIZE_LIMIT_PERCENT", 75) // limit size in-memory of all loaded stores concurrently, in percentage of usable memory (cgroup or system total -- regardless of free or available)

func parseUint64EnvVar(envVar string, defaultValue uint64) uint64 {
	if val := os.Getenv(envVar); val != "" {
		if parsed, err := strconv.ParseUint(val, 10, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

var usableMemoryBytesMemoized uint64

func usableMemoryBytes() uint64 {
	if usableMemoryBytesMemoized != 0 {
		return usableMemoryBytesMemoized
	}

	if mem, err := memlimit.FromCgroup(); err == nil {
		usableMemoryBytesMemoized = mem
	} else {
		usableMemoryBytesMemoized = memory.TotalMemory()
	}

	return usableMemoryBytesMemoized
}

func NewActiveRequestsManager(logger *zap.Logger) *ActiveRequestsManager {
	out := &ActiveRequestsManager{
		reqs:                            make(map[string]*activeRequestRecord),
		logger:                          logger,
		storeSizeLimitPerRequest:        storeSizeLimitPerRequest,
		totalStoreSizeLimitBytes:        usableMemoryBytes() * totalStoreSizeLimitPercent / 100,
		enforceTotalStoreSizeLimit:      enforceTotalStoreSizeLimit,
		enforceStoreSizeLimitPerRequest: enforceStoreSizeLimitPerRequest,
	}
	logger.Info("ActiveRequestsManager initialized", zap.Uint64("storeSizeLimitPerRequest", out.storeSizeLimitPerRequest), zap.Uint64("totalStoreSizeLimitBytes", out.totalStoreSizeLimitBytes), zap.Bool("enforceTotalStoreSizeLimit", out.enforceTotalStoreSizeLimit), zap.Bool("enforceStoreSizeLimitPerRequest", out.enforceStoreSizeLimitPerRequest))
	return out
}

type ActiveRequestsManager struct {
	reqs map[string]*activeRequestRecord
	sync.RWMutex
	storeSizeLimitPerRequest uint64
	totalStoreSizeLimitBytes uint64 // limit total store data loaded in memory (across all requests)

	enforceTotalStoreSizeLimit      bool
	enforceStoreSizeLimitPerRequest bool

	logger *zap.Logger
}

type ActiveRequestsHandler struct {
	uniqueID string
	manager  *ActiveRequestsManager
}

func NewActiveRequestsHandler(manager *ActiveRequestsManager) *ActiveRequestsHandler {
	return &ActiveRequestsHandler{
		manager: manager,
	}
}

type activeRequestRecord struct {
	StartTime              time.Time
	cancelFunc             context.CancelCauseFunc
	TraceID                string
	OutputModuleHash       string
	SegmentNumber          uint64
	SegmentSize            uint64
	Stage                  uint32
	FullKVStoreMemoryBytes uint64
}

var ErrInstanceOutOfMemory = errors.New("instance out of memory")

func (arh *ActiveRequestsHandler) totalLoadedSize() (totalSize uint64) {
	for _, req := range arh.manager.reqs {
		totalSize += req.FullKVStoreMemoryBytes
	}
	return totalSize
}

func (arh *ActiveRequestsHandler) AdjustFullKVSize(size uint64) {
	arh.manager.Lock()
	defer arh.manager.Unlock()
	if req := arh.manager.reqs[arh.uniqueID]; req != nil {
		req.FullKVStoreMemoryBytes = size
	}
}

// AllocateFullKVSizeOrForceCancelRequest will force-cancel the request using req.cancelFunc if the size exceeds the limit and if it is enforced
func (arh *ActiveRequestsHandler) AllocateFullKVSizeOrForceCancelRequest(size uint64) {
	if size == 0 {
		return
	}
	arh.manager.Lock()
	defer arh.manager.Unlock()
	if req := arh.manager.reqs[arh.uniqueID]; req != nil {

		if size > arh.manager.storeSizeLimitPerRequest {
			arh.manager.logger.Warn("sum of all stores used in this request is above maximum", zap.String("uniqueID", arh.uniqueID), zap.Uint64("size", size), zap.Uint64("totalBytes", arh.manager.storeSizeLimitPerRequest), zap.Bool("enforced", arh.manager.enforceStoreSizeLimitPerRequest))
			if arh.manager.enforceStoreSizeLimitPerRequest {
				req.cancelFunc(connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("sum of all stores used in this request have a size of %q, above maximum of: %q, (deterministic error)", humanize.IBytes(size), humanize.IBytes(arh.manager.storeSizeLimitPerRequest))))
				return
			}
		}

		availableMemory := arh.manager.totalStoreSizeLimitBytes - arh.totalLoadedSize()
		if size > availableMemory {
			arh.manager.logger.Warn("Cannot load KV stores: will go out of memory",
				zap.String("uniqueID", arh.uniqueID),
				zap.Uint64("requested_size", size),
				zap.Uint64("available_memory", availableMemory),
				zap.Bool("enforced", arh.manager.enforceTotalStoreSizeLimit),
			)
			if arh.manager.enforceTotalStoreSizeLimit {
				req.cancelFunc(connect.NewError(connect.CodeResourceExhausted, ErrInstanceOutOfMemory))
				return
			}
		}
		req.FullKVStoreMemoryBytes += size
		arh.manager.logger.Debug("LoadedFullKV", zap.String("uniqueID", arh.uniqueID), zap.Uint64("size", size), zap.Uint64("totalLoadedSize", arh.totalLoadedSize()))
	} else {
		arh.manager.logger.Warn("LoadedFullKV called for unknown request", zap.String("uniqueID", arh.uniqueID))
	}
}

func (arr *ActiveRequestsManager) Add(cancel context.CancelCauseFunc, traceID string, outputModuleHash string, segmentNumber, segmentSize uint64, stage uint32) *ActiveRequestsHandler {
	uniqueID := reqID(traceID, segmentNumber, segmentSize, stage)
	arr.Lock()
	arr.reqs[uniqueID] = &activeRequestRecord{
		StartTime:              time.Now(),
		cancelFunc:             cancel,
		TraceID:                traceID,
		OutputModuleHash:       outputModuleHash,
		SegmentNumber:          segmentNumber,
		SegmentSize:            segmentSize,
		Stage:                  stage,
		FullKVStoreMemoryBytes: 0,
	}
	arr.Unlock()

	return &ActiveRequestsHandler{
		manager:  arr,
		uniqueID: uniqueID,
	}
}

func (arr *ActiveRequestsManager) Remove(reqHandler *ActiveRequestsHandler) {
	arr.Lock()
	delete(arr.reqs, reqHandler.uniqueID)
	arr.Unlock()
}

func (arr *ActiveRequestsManager) List() []*activeRequestRecord {
	var out []*activeRequestRecord
	arr.RLock()
	for _, req := range arr.reqs {
		out = append(out, req)
	}
	arr.RUnlock()
	return out
}

func (arr *ActiveRequestsManager) CancelRequest(traceID string, outputModuleHash string, segmentNumber, segmentSize *uint64, stage *uint32) (out []string) {
	arr.RLock()
	for k, req := range arr.reqs {
		if traceID != "" && traceID != req.TraceID {
			continue
		}
		if outputModuleHash != "" && outputModuleHash != req.OutputModuleHash {
			continue
		}
		if segmentNumber != nil && *segmentNumber != req.SegmentNumber {
			continue
		}
		if segmentSize != nil && *segmentSize != req.SegmentSize {
			continue
		}
		if stage != nil && *stage != req.Stage {
			continue
		}
		req.cancelFunc(fmt.Errorf("request forcefully cancelled, please try again"))
		out = append(out, k)
	}
	arr.RUnlock()
	return out
}

func reqID(traceID string, segmentNumber, segmentSize uint64, stage uint32) string {
	return fmt.Sprintf("%s-%d-%d-%d", traceID, segmentNumber, segmentSize, stage)
}
