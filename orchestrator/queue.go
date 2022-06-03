package orchestrator

import (
	"container/heap"
	"context"
)

type QueueItem struct {
	job      *Job // The value of the item; arbitrary.
	Priority int  // The Priority of the item in the queue.

	// The index is needed by update and is maintained by the heap.Interface methods.
	index int // The index of the item in the heap.
}

// A PriorityQueue implements heap.Interface and holds Items.
type PriorityQueue []*QueueItem

func (pq PriorityQueue) Len() int { return len(pq) }

func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].Priority > pq[j].Priority
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*QueueItem)
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

func StartQueue(ctx context.Context, in <-chan *QueueItem, out chan<- *QueueItem) {
	go func() {
		pq := make(PriorityQueue, 0)
		heap.Init(&pq)

		var currentIn = in
		var currentOut chan<- *QueueItem
		var currentItem *QueueItem

		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-in:
				if !ok {
					// The input has been closed. Do not keep trying to read it
					currentIn = nil

					// If there is nothing pending to write, we are done
					if currentItem == nil {
						return
					}
					continue
				}

				// Were we holding something to write? Put it back.
				if currentItem != nil {
					heap.Push(&pq, currentItem)
				}

				// Put our new item on the queue
				heap.Push(&pq, item)

				// Activate the output queue if it is nil
				currentOut = out

				// Pop item from the heap. We know there is at least one because we just put it there.
				currentItem = heap.Pop(&pq).(*QueueItem)

				// Write to the output
			case currentOut <- currentItem:
				// OK, we wrote the current item to the queue output. Is there anything else?
				if len(pq) > 0 {
					// Hold onto it for next time
					currentItem = heap.Pop(&pq).(*QueueItem)
				} else {
					// Nothing to write. Is the input stream done?
					if currentIn == nil {
						// Then we are done
						return
					}

					// Otherwise, turn off the output stream for now.
					currentItem = nil
					currentOut = nil
				}
			}
		}
	}()
}
