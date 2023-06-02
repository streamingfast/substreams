package app

import (
	dauth "github.com/streamingfast/dauth"
	"github.com/streamingfast/dmetrics"
)

type Modules struct {
	// Required dependencies
	Authenticator         dauth.Authenticator
	HeadTimeDriftMetric   *dmetrics.HeadTimeDrift
	HeadBlockNumberMetric *dmetrics.HeadBlockNum
}
