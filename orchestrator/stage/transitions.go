package stage

import (
	"fmt"
	"slices"
)

/*
Transitions:

Put that in https://mermaid.live/ :

stateDiagram-v2
    [*] --> Pending
    Pending --> Scheduled: via NextJob()
    %%  the scheduler claims the next job that can be processed, we don't have files for
    %%  it, and its dependencies are all met
    Pending --> PartialPresent: init or polling
    %%  two ways to get there:
    %%  1. initial storage state fetcher found a partial, without spinning up a job for it,
    %%  it's waiting and we'll want to watch for the existence of this file, because it
    %%  could disappear. In this case, it would return to Pending
    %%  2. some polling mechanism discovers a partial file without a job having been scheduled
	%%
    %%  when there's a newly discovered partial, we'll want to ask the Squasher if something
    %%  can be done with that partial next.
    %%NO: Pending --> Merging
    %%  we'll leave the messaging schedule the Squasher and take it from PartialPresent --> Merging
    Pending --> Completed: init storage fetch
    %%  the initial storage state fetcher found a file with the complete store for this
    %%  exact segment. Only seen files are marked: older snapshots may have been pruned.
    Pending --> NoOp: init storage fetch
    %%  the segment is below the resume segment (a later complete store covers it), so
    %%  nothing will ever read its files.

    %%NO: PartialPresent --> Pending: squasher didn't find partial
    %%  we'll let PartialPresent go to the squasher, and if it doesn't find it
    %%  then, it can make it Pending back from the Merging state.
    %%  eventually, we could optimize and look out for those partial files
    %%  pre-emptively, and schedule the job sooner, instead of waiting
    %%  for the moment the Squasher needs it before making it Pending again.
    %%NO: PartialPresent --> Scheduled:
    %%  wouldn't make sense to schedule anything if the Partial is present, merge it instead
    PartialPresent --> Merging: was next in line\nsquasher started work
    %%  a check with the merger confirms that this segment is the next in line, so we
    %%  schedule it for merging. One at a time, means that we still have the polling
    %%  mechanism free to adjust for the next partials, and schedule something if,
    %%  when this one is done, the next one returned to Pending (the partial was deleted).
    %%  A message at the end of merging will come back to the scheduler, so that it schedules
    %%  the next contiguous one for squashing.
    %%  QUESTION: if there's nothing for squashing on the next run, which message will kick
    %%  back the squashing loop?
    %%    * the moment a job finishes, or a Partial is discovered, we'll question the Squasher
    %%      for its squashing state, and schedule the next contiguous one.
    %%NO: PartialPresent --> Completed
    %%  Could it be that we discover a complete store, and so before we launch the squasher
    %%  on the present partial, we'll rather reload the complete store (if its the next one)
    %%  For now, we'll require that partials be merged linearly, and not support the discovery
    %%  of a complete store, when the Partial has chances to be merged in just a moment.

    %%NO: Scheduled --> Pending
    %%  if a job has been scheduled, it either completes, or retries on its own, but doesn't
    %%  come back to Pending.
    Scheduled --> PartialPresent: job done,\npartial on disk
    %%  two ways we get to a PartialPresent state:
    %%  1. the job finishes and reports that a new Partial is awaiting merging
    %%  2. a polling mechanism discovers a new Partial (in which case, we'll preemptively cancel the job if one was running
    %%NO: Scheduled --> Merging:
    %%  we don't go directly from Scheduled to Merging, we'll leave the PartialPresent state
    %%  flow through to Merging.
    %%NO: Scheduled --> Completed:
    %%  it's the squasher's responsibility to take the output of a job, and make it complete
    %%  not the job scheduler, so again, transit through PartialPresent --> Merging --> Completed

    Merging --> Pending: squasher didn't\nfind partial
    %%NO: Merging --> PartialPresent
    %%  no, you're merging that partial, if you can't, we go back to Pending
    %%NO: Merging --> Scheduled
    %%  no
    Merging --> Completed: squasher finished\nfinal store ready for segment
    %%  we have successfully merged the partials into a complete store
    %%NO: Completed --> Pending: no need
    %%NO: Completed --> PartialPresent: you're done the work already, so don't do that
    %%NO: Completed --> Scheduled: all work is done, shut up
    %%NO: Completed --> Merging: all work is done, shut up

    Completed --> [*]

    [*] --> NoOp: no processing needed
    NoOp --> [*]
*/

// TODO: UnitCompleted might mean two things: the fact a FullKV is on disk,
//  or the fact that we've merged a partial and we're done. It might be
//  that we sent a goroutine to write to storage, but it hasn't reached the Bucket
//  yet.
//  We want to distinguish those to make sure merging happens as _quickly_ as
//  possible when there are many partials to merge, but we also want to know
//  when the files are indeed present on disk (being detected, or after the _write_
//  operation of the merging operation successfully finished).

func (s *Stages) MarkSegmentMerging(u Unit) {
	if !s.previousUnitComplete(u) {
		panic("can only merge segments if previous is complete")
	}
	s.transition(u, UnitMerging,
		UnitPartialPresent, // was next in line for Squasher to process
	)
}

func (s *Stages) MarkSegmentPending(u Unit) {
	s.transition(u, UnitPending,
		UnitMerging, // Squasher didn't find the partials, so asking for the job to re-run
	)
}

// ReprocessMapSegment sends a map-stage unit back to Pending so its job runs again,
// when the output file it was supposed to have written cannot be found. Returns false
// when the unit is not in a done state (a job may already be re-running it).
func (s *Stages) ReprocessMapSegment(segmentIdx int) bool {
	stageIdx := len(s.stages) - 1
	if s.stages[stageIdx].kind != KindMap {
		return false
	}
	u := Unit{Stage: stageIdx, Segment: segmentIdx}
	switch s.getState(u) {
	case UnitCompleted, UnitPartialPresent:
	default:
		return false
	}
	s.setState(u, UnitPending)
	// NextJob only scans from nextJobCursor; the cursor had moved past this segment
	// when the unit was done.
	if segmentIdx < s.nextJobCursor {
		s.nextJobCursor = segmentIdx
	}
	return true
}

func (s *Stages) MarkJobSuccess(u Unit) (shadowedUnits []Unit) {
	s.MarkSegmentPartialPresent(u)

	if s.shadowable(u.Segment) {
		for i := u.Stage - 1; i >= 0; i-- {
			u2 := Unit{Segment: u.Segment, Stage: i}
			if s.getState(u2) == UnitShadowed {
				s.transition(u2, UnitPartialPresent, UnitShadowed) // we let the squasher pretend it is partial, because squashing can occur from full or from partial
				shadowedUnits = append(shadowedUnits, u2)
			}
		}
	}
	return
}

func (s *Stages) MarkSegmentPartialPresent(u Unit) {
	s.transition(u, UnitPartialPresent,
		UnitScheduled, // reported by working completing its generation of a partial
		UnitPending,   // from initial storage state snapshot
	)
}

func (s *Stages) ReleaseJob(u Unit) {
	s.transition(u, UnitPending,
		UnitScheduled,
	)
}

func (s *Stages) markSegmentScheduled(u Unit) {
	s.transition(u, UnitScheduled,
		UnitPending, // after scheduling some work (NextJob())
	)
}

// markSegmentNoOp is used for store segments below the resume segment: a later fullKV
// covers them and nothing will ever read their (possibly pruned) files.
func (s *Stages) markSegmentNoOp(u Unit) {
	s.transition(u, UnitNoOp,
		UnitPending,
		UnitNoOp,
	)
}

func (s *Stages) markSegmentCompleted(u Unit) {
	s.transition(u, UnitCompleted,
		UnitPending, // from an initial storage state snapshot
		UnitMerging, // from the Squasher's merge operations completing
		UnitScheduled,
		UnitShadowed,
		UnitNoOp,
		UnitCompleted, // in case it got completed indirectly
	)
}

func (s *Stages) transition(u Unit, to UnitState, allowedPreviousStates ...UnitState) {
	s.allocSegments(u.Segment)
	prev := s.getState(u)
	if slices.Contains(allowedPreviousStates, prev) {
		s.setState(u, to)
		return
	}
	invalidTransition(prev, to, u)
}

func invalidTransition(prev, next UnitState, u Unit) {
	panic(fmt.Sprintf("invalid transition from %q to %q on unit [stage: %d, segment: %d]", prev.String(), next.String(), u.Stage, u.Segment))
}

func (s *Stages) forceTransition(segment int, stage int, to UnitState) {
	// For testing purposes:
	s.segmentStates[segment][stage] = to
}
