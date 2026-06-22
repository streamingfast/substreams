package store

import (
	"os"
	"strings"

	"go.uber.org/zap"
)

// KVImplType specifies the storage implementation
type KVImplType string

const (
	KVImplTypeMmap   KVImplType = "mmap"
	KVImplTypeMemory KVImplType = "memory"
)

// KVImplBackendConfig holds backend-specific configuration.
// Each backend implements this interface with its own options.
type KVImplBackendConfig interface {
	implBackendConfig()
}

// MmapBackendConfig holds mmap (bbolt) specific options.
type MmapBackendConfig struct {
	ScratchSpace string // base directory for bbolt files; uses OS temp dir if empty
}

// MemoryBackendConfig holds in-memory backend options (none currently).
type MemoryBackendConfig struct{}

func (MmapBackendConfig) implBackendConfig()   {}
func (MemoryBackendConfig) implBackendConfig() {}

// KVImplConfig holds shared configuration for KVImpl creation.
// Backend holds implementation-specific options; if nil, defaults are used.
type KVImplConfig struct {
	Type       KVImplType
	StoreName  string
	ModuleHash string
	Backend    KVImplBackendConfig
}

// NewKVImpl creates the KVImpl described by this config.
func (cfg *KVImplConfig) NewKVImpl(logger *zap.Logger) (KVImpl, error) {
	switch cfg.Type {
	case KVImplTypeMemory:
		logger.Info("using in-memory KV store")
		return newMemoryKVImplWithStoreName(cfg.StoreName), nil
	default: // KVImplTypeMmap
		mmapCfg, _ := cfg.Backend.(*MmapBackendConfig)
		if mmapCfg == nil {
			mmapCfg = &MmapBackendConfig{}
		}
		logger.Info("using mmap KV store",
			zap.String("scratch_space", mmapCfg.ScratchSpace),
			zap.String("store_name", cfg.StoreName))
		impl, err := newMmapKVImplWithConfig(cfg.StoreName, cfg.ModuleHash, mmapCfg)
		if err != nil {
			logger.Warn("failed to create mmap KV impl, falling back to in-memory", zap.Error(err))
			return newMemoryKVImplWithStoreName(cfg.StoreName), nil
		}
		return impl, nil
	}
}

// DefaultKVImplConfig returns a KVImplConfig whose Type is derived from the backend
// argument when provided, falling back to the SUBSTREAMS_STORE_BACKEND env var.
func DefaultKVImplConfig(storeName, moduleHash string, backend KVImplBackendConfig) *KVImplConfig {
	var t KVImplType
	switch backend.(type) {
	case *MemoryBackendConfig:
		t = KVImplTypeMemory
	case *MmapBackendConfig:
		t = KVImplTypeMmap
	default:
		// backend is nil or unknown — fall back to env var
		t = getKVImplTypeFromEnv()
		if backend == nil {
			switch t {
			case KVImplTypeMemory:
				backend = &MemoryBackendConfig{}
			default:
				backend = &MmapBackendConfig{}
			}
		}
	}
	return &KVImplConfig{
		Type:       t,
		StoreName:  storeName,
		ModuleHash: moduleHash,
		Backend:    backend,
	}
}

// getKVImplTypeFromEnv reads SUBSTREAMS_STORE_BACKEND env var
func getKVImplTypeFromEnv() KVImplType {
	switch strings.ToLower(os.Getenv("SUBSTREAMS_STORE_BACKEND")) {
	case "memory", "mem":
		return KVImplTypeMemory
	default:
		return KVImplTypeMmap
	}
}
