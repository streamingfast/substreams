package context

import (
	"context"
	"net/http"

	"github.com/streamingfast/substreams/sink-server/printer"
)

type sinkContextKey int

const deployStatusPrinterKey = sinkContextKey(0)

func WithDeployStatusPrinter(ctx context.Context, printer *printer.DeployStatusPrinter) context.Context {
	return context.WithValue(ctx, deployStatusPrinterKey, printer)
}

func GetDeployStatusPrinter(ctx context.Context) interface {
	Printf(format string, args ...interface{})
} {
	if printer, ok := ctx.Value(deployStatusPrinterKey).(*printer.DeployStatusPrinter); ok {
		return printer
	}
	return printer.DefaultDeployStatusPrinter
}

const productionModeKey = sinkContextKey(1)

func SetProductionMode(ctx context.Context, productionMode bool) context.Context {
	return context.WithValue(ctx, productionModeKey, productionMode)
}

func GetProductionMode(ctx context.Context) bool {
	productionModeValue := ctx.Value(productionModeKey)
	if productionMode, ok := productionModeValue.(bool); ok {
		return productionMode
	}
	return false
}

const parameterKey = sinkContextKey(2)

func SetParameterMap(ctx context.Context, env map[string]string) context.Context {
	return context.WithValue(ctx, parameterKey, env)
}

func GetParameterMap(ctx context.Context) map[string]string {
	if env, ok := ctx.Value(parameterKey).(map[string]string); ok {
		return env
	}
	return map[string]string{}
}

const headerKey = sinkContextKey(3)

func SetHeader(ctx context.Context, header http.Header) context.Context {
	return context.WithValue(ctx, headerKey, header)
}

func GetHeader(ctx context.Context) http.Header {
	val := ctx.Value(headerKey)
	switch out := val.(type) {
	case http.Header:
		return out
	}
	return nil
}
