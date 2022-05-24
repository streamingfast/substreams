package orchestrator

import (
	"container/heap"
	pbsubstreams "github.com/streamingfast/substreams/pb/sf/substreams/v1"
)

// An Item is something we manage in a priority queue.
type Item struct {
	value    *pbsubstreams.Request // The value of the item; arbitrary.
	priority int                   // The priority of the item in the queue.
	// The index is needed by update and is maintained by the heap.Interface methods.
	index int // The index of the item in the heap.
}

// A PriorityQueue implements heap.Interface and holds Items.
type PriorityQueue []*Item

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].priority < pq[j].priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

func (pq *PriorityQueue) QPush(r *pbsubstreams.Request, priority int) {
	item := &Item{
		value:    r,
		priority: priority,
	}
	heap.Push(pq, item)
}

func (pq *PriorityQueue) QPop() *pbsubstreams.Request {
	if len(*pq) == 0 {
		return nil
	}

	i := heap.Pop(pq)
	if i == nil {
		return nil
	}
	return i.(*Item).value
}

func (pq *PriorityQueue) QInit() {
	heap.Init(pq)
}
