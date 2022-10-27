package work

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"sync"
	"testing"
)

func TestWorkPlanning(t *testing.T) {
	tests := []struct {
		name              string
		plan              *Plan
		subreqSplit       int
		expectWaitingJobs []*Job
	}{
		{
			name: "simple",
			plan: &Plan{
				upToBlock: 85,
				ModulesStateMap: TestModStateMap(
					TestModState("A", "0-10,10-20,30-40,40-50,50-60"),
					TestModState("B", "0-10"),
				),
			},
			subreqSplit: 20,
			expectWaitingJobs: []*Job{
				TestJob("A", "0-20", 4),
				TestJob("A", "30-50", 3),
				TestJob("A", "50-60", 2),
				TestJob("B", "0-10", 4),
			},
		},
		{
			name: "test relative priority",
			plan: &Plan{
				upToBlock: 60,
				ModulesStateMap: TestModStateMap(
					TestModState("A", "0-10,30-40,50-60"),
					TestModState("D", "10-20,50-60"),
					TestModState("G", "0-10,50-60"),
				),
			},
			subreqSplit: 10,
			expectWaitingJobs: []*Job{
				TestJob("A", "0-10", 6),
				TestJob("A", "30-40", 3),
				TestJob("A", "50-60", 1),
				TestJobDeps("D", "10-20", 4, "B"),
				TestJobDeps("D", "50-60", 0, "B"),
				TestJobDeps("G", "0-10", 4, "B,E"),
				TestJobDeps("G", "50-60", -1, "B,E"),
			},
		},
	}
	mods := manifest.NewTestModules()
	graph, err := manifest.NewModuleGraph(mods)
	require.NoError(t, err)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, test.plan.splitWorkIntoJobs(uint64(test.subreqSplit), graph))
			assert.Equal(t, len(test.expectWaitingJobs), len(test.plan.waitingJobs))
			for i, job := range test.expectWaitingJobs {
				assert.Equal(t, test.plan.waitingJobs[i].String(), job.String())
			}
		})
	}
}

//
//func TestBuildNewPlan(t *testing.T) {
//	type args struct {
//		ctx                        context.Context
//		storeConfigMap             store.ConfigMap
//		storeSnapshotsSaveInterval uint64
//		subrequestSplitSize        uint64
//		upToBlock                  uint64
//		graph                      *manifest.ModuleGraph
//	}
//	tests := []struct {
//		name    string
//		args    args
//		want    *Plan
//		wantErr assert.ErrorAssertionFunc
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			got, err := BuildNewPlan(tt.args.ctx, tt.args.storeConfigMap, tt.args.storeSnapshotsSaveInterval, tt.args.subrequestSplitSize, tt.args.upToBlock, tt.args.graph)
//			if !tt.wantErr(t, err, fmt.Sprintf("BuildNewPlan(%v, %v, %v, %v, %v, %v)", tt.args.ctx, tt.args.storeConfigMap, tt.args.storeSnapshotsSaveInterval, tt.args.subrequestSplitSize, tt.args.upToBlock, tt.args.graph)) {
//				return
//			}
//			assert.Equalf(t, tt.want, got, "BuildNewPlan(%v, %v, %v, %v, %v, %v)", tt.args.ctx, tt.args.storeConfigMap, tt.args.storeSnapshotsSaveInterval, tt.args.subrequestSplitSize, tt.args.upToBlock, tt.args.graph)
//		})
//	}
//}

func TestPlan_MarkDependencyComplete(t *testing.T) {
	t.Skip("not implemented")
	type fields struct {
		ModulesStateMap       ModuleStorageStateMap
		upToBlock             uint64
		waitingJobs           []*Job
		readyJobs             []*Job
		modulesReadyUpToBlock map[string]uint64
		mu                    sync.Mutex
	}
	type args struct {
		modName   string
		upToBlock uint64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				ModulesStateMap:       tt.fields.ModulesStateMap,
				upToBlock:             tt.fields.upToBlock,
				waitingJobs:           tt.fields.waitingJobs,
				readyJobs:             tt.fields.readyJobs,
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
				mu:                    tt.fields.mu,
			}
			p.MarkDependencyComplete(tt.args.modName, tt.args.upToBlock)
		})
	}
}

func TestPlan_NextJob(t *testing.T) {
	mkJobs := func(rng string) (out []*Job) {
		for _, el := range strings.Split(rng, ",") {
			if el != "" {
				out = append(out, &Job{ModuleName: el})
			}
		}
		return
	}

	tests := []struct {
		waitingJobs []*Job
		readyJobs   []*Job
		expectJob   *Job
		expectMore  bool
	}{
		{
			waitingJobs: mkJobs("A"),
			readyJobs:   mkJobs("B"),
			expectJob:   &Job{ModuleName: "B"},
			expectMore:  true,
		},
		{
			waitingJobs: mkJobs("A"),
			readyJobs:   mkJobs(""),
			expectJob:   nil,
			expectMore:  true,
		},
		{
			waitingJobs: mkJobs(""),
			readyJobs:   mkJobs("B"),
			expectJob:   &Job{ModuleName: "B"},
			expectMore:  false,
		},
		{
			waitingJobs: mkJobs(""),
			readyJobs:   mkJobs(""),
			expectJob:   nil,
			expectMore:  false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			p := &Plan{
				waitingJobs: test.waitingJobs,
				readyJobs:   test.readyJobs,
			}

			gotJob, gotMore := p.NextJob()

			assert.Equalf(t, test.expectJob, gotJob, "NextJob()")
			assert.Equalf(t, test.expectMore, gotMore, "NextJob()")
		})
	}
}

func TestPlan_allDependenciesMet(t *testing.T) {
	t.Skip("not implemented")
	type fields struct {
		ModulesStateMap       ModuleStorageStateMap
		upToBlock             uint64
		waitingJobs           []*Job
		readyJobs             []*Job
		modulesReadyUpToBlock map[string]uint64
		mu                    sync.Mutex
	}
	type args struct {
		job *Job
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				ModulesStateMap:       tt.fields.ModulesStateMap,
				upToBlock:             tt.fields.upToBlock,
				waitingJobs:           tt.fields.waitingJobs,
				readyJobs:             tt.fields.readyJobs,
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
				mu:                    tt.fields.mu,
			}
			assert.Equalf(t, tt.want, p.allDependenciesMet(tt.args.job), "allDependenciesMet(%v)", tt.args.job)
		})
	}
}

func TestPlan_initModulesReadyUpToBlock(t *testing.T) {
	t.Skip("not implemented")
	type fields struct {
		ModulesStateMap       ModuleStorageStateMap
		upToBlock             uint64
		waitingJobs           []*Job
		readyJobs             []*Job
		modulesReadyUpToBlock map[string]uint64
		mu                    sync.Mutex
	}
	tests := []struct {
		name   string
		fields fields
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				ModulesStateMap:       tt.fields.ModulesStateMap,
				upToBlock:             tt.fields.upToBlock,
				waitingJobs:           tt.fields.waitingJobs,
				readyJobs:             tt.fields.readyJobs,
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
				mu:                    tt.fields.mu,
			}
			p.initModulesReadyUpToBlock()
		})
	}
}

func TestPlan_prioritize(t *testing.T) {
	t.Skip("not implemented")
	type fields struct {
		ModulesStateMap       ModuleStorageStateMap
		upToBlock             uint64
		waitingJobs           []*Job
		readyJobs             []*Job
		modulesReadyUpToBlock map[string]uint64
		mu                    sync.Mutex
	}
	tests := []struct {
		name   string
		fields fields
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				ModulesStateMap:       tt.fields.ModulesStateMap,
				upToBlock:             tt.fields.upToBlock,
				waitingJobs:           tt.fields.waitingJobs,
				readyJobs:             tt.fields.readyJobs,
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
				mu:                    tt.fields.mu,
			}
			p.prioritize()
		})
	}
}

func TestPlan_promoteWaitingJobs(t *testing.T) {
	t.Skip("not implemented")
	type fields struct {
		ModulesStateMap       ModuleStorageStateMap
		upToBlock             uint64
		waitingJobs           []*Job
		readyJobs             []*Job
		modulesReadyUpToBlock map[string]uint64
		mu                    sync.Mutex
	}
	tests := []struct {
		name   string
		fields fields
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				ModulesStateMap:       tt.fields.ModulesStateMap,
				upToBlock:             tt.fields.upToBlock,
				waitingJobs:           tt.fields.waitingJobs,
				readyJobs:             tt.fields.readyJobs,
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
				mu:                    tt.fields.mu,
			}
			p.promoteWaitingJobs()
		})
	}
}

func TestPlan_splitWorkIntoJobs(t *testing.T) {
	t.Skip("not implemented")
	type fields struct {
		ModulesStateMap       ModuleStorageStateMap
		upToBlock             uint64
		waitingJobs           []*Job
		readyJobs             []*Job
		modulesReadyUpToBlock map[string]uint64
		mu                    sync.Mutex
	}
	type args struct {
		subrequestSplitSize uint64
		graph               *manifest.ModuleGraph
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				ModulesStateMap:       tt.fields.ModulesStateMap,
				upToBlock:             tt.fields.upToBlock,
				waitingJobs:           tt.fields.waitingJobs,
				readyJobs:             tt.fields.readyJobs,
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
				mu:                    tt.fields.mu,
			}
			tt.wantErr(t, p.splitWorkIntoJobs(tt.args.subrequestSplitSize, tt.args.graph), fmt.Sprintf("splitWorkIntoJobs(%v, %v)", tt.args.subrequestSplitSize, tt.args.graph))
		})
	}
}

func TestPlan_buildPlanFromStorageState(t *testing.T) {
	t.Skip("not implemented")
	type fields struct {
		ModulesStateMap       ModuleStorageStateMap
		upToBlock             uint64
		waitingJobs           []*Job
		readyJobs             []*Job
		modulesReadyUpToBlock map[string]uint64
		mu                    sync.Mutex
	}
	type args struct {
		ctx                        context.Context
		storageState               *StorageState
		storeConfigMap             store.ConfigMap
		storeSnapshotsSaveInterval uint64
		upToBlock                  uint64
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				ModulesStateMap:       tt.fields.ModulesStateMap,
				upToBlock:             tt.fields.upToBlock,
				waitingJobs:           tt.fields.waitingJobs,
				readyJobs:             tt.fields.readyJobs,
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
				mu:                    tt.fields.mu,
			}
			tt.wantErr(t, p.buildPlanFromStorageState(tt.args.ctx, tt.args.storageState, tt.args.storeConfigMap, tt.args.storeSnapshotsSaveInterval, tt.args.upToBlock), fmt.Sprintf("buildPlanFromStorageState(%v, %v, %v, %v, %v)", tt.args.ctx, tt.args.storageState, tt.args.storeConfigMap, tt.args.storeSnapshotsSaveInterval, tt.args.upToBlock))
		})
	}
}
