package sinksql

import (
	"fmt"
	"strings"

	pbsql "github.com/streamingfast/substreams/sink/sql/pb/sf/substreams/sink/sql/services/v1"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"google.golang.org/protobuf/proto"
)

var (
	supportedDeployableUnits              []string
	deprecated_supportedDeployableService = "sf.substreams.sink.sql.v1.Service"
	supportedDeployableService            = "sf.substreams.sink.sql.service.v1.Service"
)

func init() {
	supportedDeployableUnits = []string{
		deprecated_supportedDeployableService,
	}
}

const typeUrlPrefix = "type.googleapis.com/"

func ExtractSinkService(pkg *pbsubstreams.Package) (*pbsql.Service, error) {
	if pkg.SinkConfig == nil {
		return nil, fmt.Errorf("no sink config found in spkg")
	}

	configPackageID := strings.TrimPrefix(pkg.SinkConfig.TypeUrl, typeUrlPrefix)

	switch configPackageID {
	case deprecated_supportedDeployableService, supportedDeployableService:
		service := &pbsql.Service{}

		if err := proto.Unmarshal(pkg.SinkConfig.Value, service); err != nil {
			return nil, fmt.Errorf("failed to proto unmarshal: %w", err)
		}
		return service, nil
	}

	return nil, fmt.Errorf("invalid config type %q, supported configs are %q", pkg.SinkConfig.TypeUrl, strings.Join(supportedDeployableUnits, ", "))
}
