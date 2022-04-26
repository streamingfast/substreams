package runtime

import "github.com/streamingfast/substreams"

type Config struct {
	ManifestPath     string
	OutputStreamName string

	StartBlock uint64
	StopBlock  uint64

	PrintMermaid bool

	ReturnHandler substreams.ReturnFunc
}

type LocalConfig struct {
	BlocksStoreUrl string
	StateStoreUrl  string
	IrrIndexesUrl  string

	ProtobufBlockType string

	RpcEndpoint string
	RpcCacheUrl string
	PartialMode bool

	ProtoUrl string

	*Config
}

type RemoteConfig struct {
	FirehoseEndpoint     string
	FirehoseApiKeyEnvVar string

	InsecureMode bool
	Plaintext    bool

	*Config
}
