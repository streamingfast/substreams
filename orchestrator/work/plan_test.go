package work

import (
	"context"
	"fmt"
	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
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
		name        string
		waitingJobs []*Job
		readyJobs   []*Job
		expectJob   *Job
		expectMore  bool
	}{
		{
			name:        "one ready,one waiting",
			waitingJobs: mkJobs("A"),
			readyJobs:   mkJobs("B"),
			expectJob:   &Job{ModuleName: "B"},
			expectMore:  true,
		},
		{
			name:        "none ready,one waiting",
			waitingJobs: mkJobs("A"),
			readyJobs:   mkJobs(""),
			expectJob:   nil,
			expectMore:  true,
		},
		{
			name:        "one ready,none waiting",
			waitingJobs: mkJobs(""),
			readyJobs:   mkJobs("A"),
			expectJob:   &Job{ModuleName: "A"},
			expectMore:  false,
		},
		{
			name:        "none ready,none waiting",
			waitingJobs: mkJobs(""),
			readyJobs:   mkJobs(""),
			expectJob:   nil,
			expectMore:  false,
		},
		{
			name:        "two ready,none waiting",
			waitingJobs: mkJobs(""),
			readyJobs:   mkJobs("A,B,C"),
			expectJob:   &Job{ModuleName: "A"},
			expectMore:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
	type fields struct {
		modulesReadyUpToBlock map[string]uint64
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
		{
			name: "all deps met",
			fields: fields{
				modulesReadyUpToBlock: map[string]uint64{
					"foo": 100,
					"bar": 50,
				},
			},
			args: args{
				job: &Job{
					ModuleName: "foo",
					RequestRange: &block.Range{
						StartBlock:        10,
						ExclusiveEndBlock: 20,
					},
					requiredModules: []string{
						"foo",
						"bar",
					},
				},
			},
			want: true,
		},
		{
			name: "some deps met",
			fields: fields{
				modulesReadyUpToBlock: map[string]uint64{
					"foo": 100,
					"bar": 0,
				},
			},
			args: args{
				job: &Job{
					ModuleName: "foo",
					RequestRange: &block.Range{
						StartBlock:        10,
						ExclusiveEndBlock: 20,
					},
					requiredModules: []string{
						"foo",
						"bar",
					},
				},
			},
			want: false,
		},
		{
			name: "no deps met",
			fields: fields{
				modulesReadyUpToBlock: map[string]uint64{
					"foo": 0,
					"bar": 0,
				},
			},
			args: args{
				job: &Job{
					ModuleName: "foo",
					RequestRange: &block.Range{
						StartBlock:        10,
						ExclusiveEndBlock: 20,
					},
					requiredModules: []string{
						"foo",
						"bar",
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
			}
			assert.Equalf(t, tt.want, p.allDependenciesMet(tt.args.job), "allDependenciesMet(%v)", tt.args.job)
		})
	}
}

func TestPlan_prioritize(t *testing.T) {
	tests := []struct {
		name      string
		readyJobs []*Job
		expected  []*Job
	}{
		{
			name:      "no jobs",
			readyJobs: []*Job{},
			expected:  []*Job{},
		},
		{
			name:      "one job",
			readyJobs: []*Job{{ModuleName: "A", priority: 1}},
			expected:  []*Job{{ModuleName: "A", priority: 1}},
		},
		{
			name:      "sorted highest priority to lowest",
			readyJobs: []*Job{{ModuleName: "B", priority: 2}, {ModuleName: "C", priority: 1}, {ModuleName: "A", priority: 3}},
			expected:  []*Job{{ModuleName: "A", priority: 3}, {ModuleName: "B", priority: 2}, {ModuleName: "C", priority: 1}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				readyJobs: tt.readyJobs,
			}
			p.prioritize()
			assert.Equalf(t, tt.expected, p.readyJobs, "prioritize()")
		})
	}
}

func TestPlan_initModulesReadyUpToBlock(t *testing.T) {
	type fields struct {
		ModulesStateMap       ModuleStorageStateMap
		modulesReadyUpToBlock map[string]uint64
	}
	tests := []struct {
		name     string
		fields   fields
		expected map[string]uint64
	}{
		{
			name: "no modules",
			fields: fields{
				ModulesStateMap: ModuleStorageStateMap{},
			},
			expected: map[string]uint64{},
		},
		{
			name: "one module,initial block",
			fields: fields{
				ModulesStateMap: ModuleStorageStateMap{
					"A": &ModuleStorageState{
						ModuleInitialBlock: 1,
					},
				},
			},
			expected: map[string]uint64{
				"A": 1,
			},
		},
		{
			name: "one module,complete range",
			fields: fields{
				ModulesStateMap: ModuleStorageStateMap{
					"A": &ModuleStorageState{
						InitialCompleteRange: &FullStoreFile{StartBlock: 1, ExclusiveEndBlock: 20},
					},
				},
			},
			expected: map[string]uint64{
				"A": 20,
			},
		},
		{
			name: "mixed modules",
			fields: fields{
				ModulesStateMap: ModuleStorageStateMap{
					"A": &ModuleStorageState{
						InitialCompleteRange: &FullStoreFile{StartBlock: 1, ExclusiveEndBlock: 20},
					},
					"B": &ModuleStorageState{
						ModuleInitialBlock: 1,
					},
				},
			},
			expected: map[string]uint64{
				"A": 20,
				"B": 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				ModulesStateMap:       tt.fields.ModulesStateMap,
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
			}
			p.initModulesReadyUpToBlock()
			assert.Equalf(t, tt.expected, p.modulesReadyUpToBlock, "initModulesReadyUpToBlock()")
		})
	}
}

func TestPlan_bumpModuleUpToBlock(t *testing.T) {
	type fields struct {
		modulesReadyUpToBlock map[string]uint64
	}
	type args struct {
		modName   string
		upToBlock uint64
	}
	tests := []struct {
		name          string
		initialFields fields
		expected      map[string]uint64
		args          args
	}{
		{
			name: "bump up to block",
			initialFields: fields{
				modulesReadyUpToBlock: map[string]uint64{
					"A": 10,
				},
			},
			args: args{
				modName:   "A",
				upToBlock: 20,
			},
			expected: map[string]uint64{
				"A": 20,
			},
		},
		{
			name: "bump up to block, no existing",
			initialFields: fields{
				modulesReadyUpToBlock: map[string]uint64{},
			},
			args: args{
				modName:   "A",
				upToBlock: 20,
			},
			expected: map[string]uint64{
				"A": 20,
			},
		},
		{
			name: "bump less than current",
			initialFields: fields{
				modulesReadyUpToBlock: map[string]uint64{
					"A": 20,
				},
			},
			args: args{
				modName:   "A",
				upToBlock: 10,
			},
			expected: map[string]uint64{
				"A": 20,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				modulesReadyUpToBlock: tt.initialFields.modulesReadyUpToBlock,
			}
			p.bumpModuleUpToBlock(tt.args.modName, tt.args.upToBlock)
			assert.Equal(t, tt.expected, p.modulesReadyUpToBlock)
		})
	}
}

func TestPlan_promoteWaitingJobs(t *testing.T) {
	type fields struct {
		waitingJobs           []*Job
		readyJobs             []*Job
		modulesReadyUpToBlock map[string]uint64
	}
	tests := []struct {
		name                   string
		fields                 fields
		expectedReadyJobsLen   int
		expectedWaitingJobsLen int
	}{
		{
			name: "one job,ready",
			fields: fields{
				modulesReadyUpToBlock: map[string]uint64{
					"B": 10,
				},
				waitingJobs: []*Job{
					{
						ModuleName: "A",
						RequestRange: &block.Range{
							StartBlock:        10,
							ExclusiveEndBlock: 20,
						},
						requiredModules: []string{"B"},
					},
				},
			},
			expectedReadyJobsLen:   1,
			expectedWaitingJobsLen: 0,
		},
		{
			name: "two jobs,one ready",
			fields: fields{
				modulesReadyUpToBlock: map[string]uint64{
					"B": 10,
					"C": 0,
				},
				waitingJobs: []*Job{
					{
						ModuleName: "A",
						RequestRange: &block.Range{
							StartBlock:        10,
							ExclusiveEndBlock: 20,
						},
						requiredModules: []string{"B"},
					},
					{
						ModuleName: "B",
						RequestRange: &block.Range{
							StartBlock:        10,
							ExclusiveEndBlock: 20,
						},
						requiredModules: []string{"C"},
					},
				},
			},
			expectedReadyJobsLen:   1,
			expectedWaitingJobsLen: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{
				waitingJobs:           tt.fields.waitingJobs,
				readyJobs:             tt.fields.readyJobs,
				modulesReadyUpToBlock: tt.fields.modulesReadyUpToBlock,
			}
			p.promoteWaitingJobs()
			assert.Equal(t, tt.expectedReadyJobsLen, len(p.readyJobs))
			assert.Equal(t, tt.expectedWaitingJobsLen, len(p.waitingJobs))
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

func TestPlan_initialProgressMessages(t *testing.T) {
	tests := []struct {
		name             string
		modState         *ModuleStorageState
		expectedProgress string
	}{
		{
			modState: &ModuleStorageState{
				ModuleName:           "A",
				InitialCompleteRange: block.ParseRange("1-10"),
				PartialsPresent:      block.ParseRanges("20-30,40-50,50-60"),
			},
			expectedProgress: "A:r1-10,20-30,40-60",
		},
		{
			modState: &ModuleStorageState{
				ModuleName:           "A",
				InitialCompleteRange: block.ParseRange("1-10"),
			},
			expectedProgress: "A:r1-10",
		},
		{
			modState: &ModuleStorageState{
				ModuleName:      "A",
				PartialsPresent: block.ParseRanges("10-20"),
			},
			expectedProgress: "A:r10-20",
		},
		{
			modState: &ModuleStorageState{
				ModuleName: "A",
			},
			expectedProgress: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &Plan{ModulesStateMap: TestModStateMap(test.modState)}

			out := p.initialProgressMessages()

			assert.Equal(t, test.expectedProgress, reduceProgressMessages(out))
		})
	}
}

func reduceProgressMessages(in []*pbsubstreams.ModuleProgress) string {
	var out []string
	for _, prog := range in {
		entry := fmt.Sprintf("%s:", prog.Name)
		switch t := prog.Type.(type) {
		case *pbsubstreams.ModuleProgress_ProcessedRanges:
			var rngs []string
			for _, rng := range t.ProcessedRanges.ProcessedRanges {
				rngs = append(rngs, fmt.Sprintf("%d-%d", rng.StartBlock, rng.EndBlock))
			}
			entry += "r" + strings.Join(rngs, ",")
		case *pbsubstreams.ModuleProgress_InitialState_:
			entry += "up" + fmt.Sprintf("%d", t.InitialState.AvailableUpToBlock)
		default:
			panic("unsupported here")
		}
		out = append(out, entry)
	}
	return strings.Join(out, ";")
}
