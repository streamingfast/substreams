package service

import (
	"testing"

	"github.com/streamingfast/substreams/metrics"
	"github.com/stretchr/testify/assert"
)

func TestTier1Service_getOverloadedStatus(t *testing.T) {
	type fields struct {
		activeRequestCount uint64
		softLimit          int
		hardLimit          int
	}

	type wanted struct {
		softLimitWouldBeReached   bool
		hardLimitReached          bool
		canAcceptUpcomingRequests bool
	}

	tests := []struct {
		name       string
		fields     fields
		wantStatus wanted
	}{
		{
			"no limit, 0 active requests is 0",
			fields{activeRequestCount: 0, softLimit: 0, hardLimit: 0},
			wanted{softLimitWouldBeReached: false, hardLimitReached: false, canAcceptUpcomingRequests: true},
		},
		{
			"no limit, 0 active requests is 100",
			fields{activeRequestCount: 100, softLimit: 0, hardLimit: 0},
			wanted{softLimitWouldBeReached: false, hardLimitReached: false, canAcceptUpcomingRequests: true},
		},

		{
			"soft limit would be reached on next request",
			fields{activeRequestCount: 1, softLimit: 2, hardLimit: 0},
			wanted{softLimitWouldBeReached: true, hardLimitReached: false, canAcceptUpcomingRequests: true},
		},
		{
			"soft limit cannot accept more requests",
			fields{activeRequestCount: 2, softLimit: 2, hardLimit: 0},
			wanted{softLimitWouldBeReached: true, hardLimitReached: false, canAcceptUpcomingRequests: false},
		},
		{
			"soft limit much above active requests",
			fields{activeRequestCount: 20, softLimit: 2, hardLimit: 0},
			wanted{softLimitWouldBeReached: true, hardLimitReached: false, canAcceptUpcomingRequests: false},
		},

		{
			"hard limit not reached",
			fields{activeRequestCount: 1, softLimit: 0, hardLimit: 2},
			wanted{softLimitWouldBeReached: false, hardLimitReached: false, canAcceptUpcomingRequests: true},
		},
		{
			"hard limit reached",
			fields{activeRequestCount: 2, softLimit: 0, hardLimit: 2},
			wanted{softLimitWouldBeReached: false, hardLimitReached: true, canAcceptUpcomingRequests: false},
		},
		{
			"hard limit above active requests",
			fields{activeRequestCount: 20, softLimit: 0, hardLimit: 2},
			wanted{softLimitWouldBeReached: false, hardLimitReached: true, canAcceptUpcomingRequests: false},
		},

		{
			"soft & hard limit not reached",
			fields{activeRequestCount: 0, softLimit: 2, hardLimit: 2},
			wanted{softLimitWouldBeReached: false, hardLimitReached: false, canAcceptUpcomingRequests: true},
		},
		{
			"soft limit would be reached, hard limit not reached",
			fields{activeRequestCount: 1, softLimit: 2, hardLimit: 2},
			wanted{softLimitWouldBeReached: true, hardLimitReached: false, canAcceptUpcomingRequests: true},
		},
		{
			"soft limit reached, hard limit not reached",
			fields{activeRequestCount: 2, softLimit: 2, hardLimit: 4},
			wanted{softLimitWouldBeReached: true, hardLimitReached: false, canAcceptUpcomingRequests: false},
		},
		{
			"soft limit reached && hard limit reached",
			fields{activeRequestCount: 4, softLimit: 2, hardLimit: 4},
			wanted{softLimitWouldBeReached: true, hardLimitReached: true, canAcceptUpcomingRequests: false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Tier1Service{
				activeRequestsSoftLimit: tt.fields.softLimit,
				activeRequestsHardLimit: tt.fields.hardLimit,
			}

			metrics.ActiveRequests.SetUint64(tt.fields.activeRequestCount)
			gotStatus := s.getOverloadedStatus()

			assert.Equal(t, tt.wantStatus.softLimitWouldBeReached, gotStatus.softLimitWouldBeReached())
			assert.Equal(t, tt.wantStatus.hardLimitReached, gotStatus.hardLimitReached())
			assert.Equal(t, tt.wantStatus.canAcceptUpcomingRequests, gotStatus.canAcceptUpcomingRequests())
		})
	}
}
