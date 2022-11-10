package work

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/streamingfast/substreams/block"
	"github.com/streamingfast/substreams/manifest"
	"github.com/streamingfast/substreams/orchestrator/outputmodules"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
	"github.com/streamingfast/substreams/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkPlanning(t *testing.T) {
	tests := []struct {
		name        string
		upToBlock   uint64
		subreqSplit int
		state       storage.ModuleStorageStateMap
		outMods     string

		expectWaitingJobs []*Job
		expectReadyJobs   []*Job
	}{
		{
			name:        "single",
			upToBlock:   85,
			subreqSplit: 20,
			state: TestModStateMap(
				TestStoreState("B", "0-10"),
			),
			outMods: "B",
			expectReadyJobs: []*Job{
				TestJob("B", "0-10", 4),
			},
		},
		{
			name:        "double",
			upToBlock:   85,
			subreqSplit: 20,
			state: TestModStateMap(
				TestStoreState("As", "0-10,10-20,30-40,40-50,50-60"),
				TestStoreState("B", "0-10"),
			),
			outMods: "As,B",
			expectReadyJobs: []*Job{
				TestJob("As", "0-20", 4),
				TestJob("B", "0-10", 4),
				TestJob("As", "30-50", 3),
				TestJob("As", "50-60", 2),
			},
		},
		{
			name:        "test relative priority",
			upToBlock:   60,
			subreqSplit: 10,
			state: TestModStateMap(
				TestStoreState("G", "0-10,50-60"),
				TestMapState("B", "0-60"),
				TestStoreState("As", "0-10,30-40,50-60"),
				TestStoreState("D", "10-20,50-60"),
			),
			outMods: "As,D,G",
			expectWaitingJobs: []*Job{
				TestJobDeps("G", "0-10", 3, "As,B,E"),
				TestJobDeps("G", "50-60", -2, "As,B,E"),
				TestJobDeps("D", "10-20", 4, "B"),
				TestJobDeps("D", "50-60", 0, "B"),
			},
			expectReadyJobs: []*Job{
				TestJob("As", "0-10", 6),
				TestJob("B", "0-60", 6),
				TestJob("As", "30-40", 3),
				TestJob("As", "50-60", 1),
			},
		},
	}

	// TODO(abourget): il faut donner à splitWorkIntoJobs() de quoi de plus slim
	// et moins loin.. une `Request` est trop gros, faudrait lui préparer plutôt
	// un OutputModuleGraph avec le data nécessaire à tester `splitWorkIntoJobs`.

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mods := manifest.NewTestModules()
			outputGraph, err := outputmodules.NewOutputModuleGraph(&pbsubstreams.Request{
				Modules: &pbsubstreams.Modules{
					Modules:  mods,
					Binaries: []*pbsubstreams.Binary{{}},
				},
				OutputModules: strings.Split(test.outMods, ","),
			})
			require.NoError(t, err)

			plan, err := BuildNewPlan(test.state, uint64(test.subreqSplit), test.upToBlock, outputGraph)
			require.NoError(t, err)

			assert.Equal(t, jobList(test.expectWaitingJobs), jobList(plan.waitingJobs), "waiting jobs")
			assert.Equal(t, jobList(test.expectReadyJobs), jobList(plan.readyJobs), "ready jobs")
		})
	}
}

func jobList(jobs []*Job) string {
	var out []string
	for _, job := range jobs {
		out = append(out, job.String())
	}
	return strings.Join(out, "\n")
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
	type fields struct {
		ModulesStateMap       storage.ModuleStorageStateMap
		upToBlock             uint64
		waitingJobs           []*Job
		readyJobs             []*Job
		modulesReadyUpToBlock map[string]uint64
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
			waitingJobs: mkJobs("As"),
			readyJobs:   mkJobs("B"),
			expectJob:   &Job{ModuleName: "B"},
			expectMore:  true,
		},
		{
			name:        "none ready,one waiting",
			waitingJobs: mkJobs("As"),
			readyJobs:   mkJobs(""),
			expectJob:   nil,
			expectMore:  true,
		},
		{
			name:        "one ready,none waiting",
			waitingJobs: mkJobs(""),
			readyJobs:   mkJobs("As"),
			expectJob:   &Job{ModuleName: "As"},
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
			readyJobs:   mkJobs("As,B,C"),
			expectJob:   &Job{ModuleName: "As"},
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
		ModulesStateMap       storage.ModuleStorageStateMap
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
				ModulesStateMap: storage.ModuleStorageStateMap{},
			},
			expected: map[string]uint64{},
		},
		{
			name: "one module,initial block",
			fields: fields{
				ModulesStateMap: storage.ModuleStorageStateMap{
					"A": &storage.StoreStorageState{
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
				ModulesStateMap: storage.ModuleStorageStateMap{
					"A": &storage.StoreStorageState{
						InitialCompleteRange: &storage.FullStoreFile{StartBlock: 1, ExclusiveEndBlock: 20},
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
				ModulesStateMap: storage.ModuleStorageStateMap{
					"A": &storage.StoreStorageState{
						InitialCompleteRange: &storage.FullStoreFile{StartBlock: 1, ExclusiveEndBlock: 20},
					},
					"B": &storage.StoreStorageState{
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
		ModulesStateMap       storage.ModuleStorageStateMap
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
			tt.wantErr(t, p.splitWorkIntoJobs(tt.args.subrequestSplitSize), fmt.Sprintf("splitWorkIntoJobs(%v, %v)", tt.args.subrequestSplitSize, tt.args.graph))
		})
	}
}

func TestPlan_initialProgressMessages(t *testing.T) {
	tests := []struct {
		name             string
		modState         storage.ModuleStorageState
		expectedProgress string
	}{
		{
			modState: &storage.StoreStorageState{
				ModuleName:           "A",
				InitialCompleteRange: block.ParseRange("1-10"),
				PartialsPresent:      block.ParseRanges("20-30,40-50,50-60"),
			},
			expectedProgress: "A:r1-10,20-30,40-60",
		},
		{
			modState: &storage.StoreStorageState{
				ModuleName:           "A",
				InitialCompleteRange: block.ParseRange("1-10"),
			},
			expectedProgress: "A:r1-10",
		},
		{
			modState: &storage.StoreStorageState{
				ModuleName:      "A",
				PartialsPresent: block.ParseRanges("10-20"),
			},
			expectedProgress: "A:r10-20",
		},
		{
			modState: &storage.StoreStorageState{
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
