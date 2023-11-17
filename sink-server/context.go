package server

import "context"

type deployStatusPrinterKeyType int

const deployStatusPrinterKey = deployStatusPrinterKeyType(0)

func WithDeployStatusPrinter(ctx context.Context, printer *DeployStatusPrinter) context.Context {
	return context.WithValue(ctx, deployStatusPrinterKey, printer)
}

func GetDeployStatusPrinter(ctx context.Context) interface {
	Printf(format string, args ...interface{})
} {
	if printer, ok := ctx.Value(deployStatusPrinterKey).(*DeployStatusPrinter); ok {
		return printer
	}
	return defaultDeployStatusPrinter
}
