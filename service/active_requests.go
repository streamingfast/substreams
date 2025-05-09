package service

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type activeRequestRecords struct {
	reqs map[string]*ActiveRequestRecord
	sync.RWMutex
}

type ActiveRequestRecord struct {
	StartTime        time.Time
	cancelFunc       context.CancelFunc
	TraceID          string
	OutputModuleHash string
	SegmentNumber    uint64
	SegmentSize      uint64
	Stage            uint32
}

func (arr *activeRequestRecords) Add(cancel context.CancelFunc, traceID string, outputModuleHash string, segmentNumber, segmentSize uint64, stage uint32) {
	uniqueID := fmt.Sprintf("%s-%d-%d-%d", traceID, segmentNumber, segmentSize, stage)
	arr.Lock()
	arr.reqs[uniqueID] = &ActiveRequestRecord{
		StartTime:        time.Now(),
		cancelFunc:       cancel,
		TraceID:          traceID,
		OutputModuleHash: outputModuleHash,
		SegmentNumber:    segmentNumber,
		SegmentSize:      segmentSize,
		Stage:            stage,
	}
	arr.Unlock()
}

func (arr *activeRequestRecords) Remove(traceID string, segmentNumber, segmentSize uint64, stage uint32) {
	uniqueID := fmt.Sprintf("%s-%d-%d-%d", traceID, segmentNumber, segmentSize, stage)
	arr.Lock()
	delete(arr.reqs, uniqueID)
	arr.Unlock()
}

func (arr *activeRequestRecords) List() []*ActiveRequestRecord {
	var out []*ActiveRequestRecord
	arr.RLock()
	for _, req := range arr.reqs {
		out = append(out, req)
	}
	arr.RUnlock()
	return out
}

func (arr *activeRequestRecords) cancelRequest(traceID string, outputModuleHash string, segmentNumber, segmentSize *uint64, stage *uint32) (out []string) {
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
		req.cancelFunc()
		out = append(out, k)
	}
	arr.RUnlock()
	return out
}
