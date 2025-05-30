package debugapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"sync"

	"go.uber.org/zap"
)

// Option is a functional option for configuring the DebugAPI
type Option func(*DebugAPI)

// WithListenAddr sets the listen address for the debug API
func WithListenAddr(addr string) Option {
	return func(api *DebugAPI) {
		api.listenAddr = addr
	}
}

// DebugAPI provides HTTP endpoints for inspecting and simulating state of a service
type DebugAPI struct {
	server *http.Server
	lock   sync.RWMutex

	listenAddr            string
	logger                *zap.Logger
	simulateOverloaded    func(bool)
	getSimulateOverloaded func() bool
	listActiveRequests    func() string
	cancelRequest         func(string, string, *uint64, *uint64, *uint32) []string
}

func New(
	listenAddr string,
	logger *zap.Logger,
	simulateOverloaded func(bool),
	getSimulateOverloaded func() bool,
	listActiveRequests func() string,
	cancelRequest func(string, string, *uint64, *uint64, *uint32) []string,
) *DebugAPI {
	api := &DebugAPI{
		listenAddr:            listenAddr,
		logger:                logger,
		getSimulateOverloaded: getSimulateOverloaded,
		simulateOverloaded:    simulateOverloaded,
		listActiveRequests:    listActiveRequests,
		cancelRequest:         cancelRequest,
	}

	return api
}

func (api *DebugAPI) Start() {
	mux := http.NewServeMux()

	// Refuse incoming requests as if we were overloaded
	mux.HandleFunc("/debug/simulate_overloaded", api.handleSimulateOverloaded)

	// Force GC
	mux.HandleFunc("/debug/gc", api.handleGC)

	// Force GC
	mux.HandleFunc("/debug/requests", api.handleRequests)

	api.server = &http.Server{
		Addr:    api.listenAddr,
		Handler: mux,
	}

	go func() {
		api.logger.Info("starting debug API HTTP server", zap.String("addr", api.listenAddr))
		if err := api.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			api.logger.Error("debug API server error", zap.Error(err))
		}
	}()

}

func (api *DebugAPI) handleSimulateOverloaded(w http.ResponseWriter, r *http.Request) {
	switch r.Method {

	case http.MethodGet:
		// Get current readiness status
		overloaded := api.getSimulateOverloaded()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"overloaded": overloaded})

	case http.MethodPost:
		// Update max concurrent requests
		var req struct {
			Overloaded bool `json:"overloaded"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			// Try to parse from query parameters
			if maxStr := r.URL.Query().Get("value"); maxStr != "" {
				val, err := strconv.ParseBool(maxStr)
				if err != nil {
					http.Error(w, "invalid value parameter", http.StatusBadRequest)
					return
				}
				req.Overloaded = val
			} else {
				http.Error(w, fmt.Sprintf("invalid request: %s", err), http.StatusBadRequest)
				return
			}
		}

		if api.simulateOverloaded != nil {
			api.simulateOverloaded(req.Overloaded)
			api.logger.Info("simulate overloaded",
				zap.Bool("simulate_overloaded", req.Overloaded))
		} else {
			http.Error(w, "overloaded cannot be set", http.StatusForbidden)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"overloaded": req.Overloaded})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGC forces the garbage collection on POST
func (api *DebugAPI) handleGC(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:

		var memStats runtime.MemStats

		runtime.ReadMemStats(&memStats)
		before := fmt.Sprintf("Alloc = %v MiB, Heap inuse = %v MiB", memStats.Alloc/1024/1024, memStats.HeapInuse/1024/1024)

		runtime.GC()

		runtime.ReadMemStats(&memStats)
		after := fmt.Sprintf("Alloc = %v MiB, Heap inuse = %v MiB", memStats.Alloc/1024/1024, memStats.HeapInuse/1024/1024)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"before": before,
			"after": after})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleRequests lists active requests on GET or cancel them on DELETE
func (api *DebugAPI) handleRequests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {

	case http.MethodGet:
		if api.listActiveRequests == nil {
			http.Error(w, "cannot list active requests", http.StatusForbidden)
			return
		}
		ar := api.listActiveRequests()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(ar))

	case http.MethodDelete:
		if api.cancelRequest == nil {
			http.Error(w, "cannot cancel active requests", http.StatusForbidden)
			return
		}

		var req struct {
			TraceID          string  `json:"trace_id"`
			OutputModuleHash string  `json:"output_module_hash"`
			SegmentNumber    *uint64 `json:"segment_number"`
			SegmentSize      *uint64 `json:"segment_size"`
			Stage            *uint32 `json:"stage"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %s", err), http.StatusBadRequest)
			return
		}
		out := api.cancelRequest(req.TraceID, req.OutputModuleHash, req.SegmentNumber, req.SegmentSize, req.Stage)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string][]string{"matches": out})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
