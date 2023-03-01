package request

import pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"

type Summary struct {
	Manifest        string
	Endpoint        string
	ProductionMode  bool
	InitialSnapshot []string
	Docs            []*pbsubstreams.PackageMetadata
}
