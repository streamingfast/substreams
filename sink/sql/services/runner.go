package services

import (
	"time"

	pbsql "github.com/streamingfast/substreams/sink/sql/pb/sf/substreams/sink/sql/services/v1"
	"go.uber.org/zap"
)

func Run(service *pbsql.Service, logger *zap.Logger) error {
	if service.HasuraFrontend != nil {
		logUnsupportedServiceMessage("Hasura front end", logger)
	}
	if service.PostgraphileFrontend != nil {
		logUnsupportedServiceMessage("Postgraphile front end", logger)
	}
	if service.RestFrontend != nil {
		logUnsupportedServiceMessage("Rest front end", logger)
	}

	if service.DbtConfig != nil && service.DbtConfig.Enabled {
		go func() {
			for {
				err := runDBT(service.DbtConfig, logger)
				if err != nil {
					logger.Error("running dbt", zap.Error(err))
					time.Sleep(30 * time.Second)
				}
			}
		}()
	}

	return nil
}

func logUnsupportedServiceMessage(serviceName string, logger *zap.Logger) {
	logger.Warn(
		"This package has " + serviceName + " service defined, however " + serviceName + " is not " +
			"supported yet when using relational mappings mode (e.g. 'substreams-sink-sql from-proto ...', " +
			"skipping it",
	)
}
