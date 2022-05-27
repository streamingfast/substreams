package orchestrator

import (
	"sync"
	"testing"
)

func TestWaiterItem_Close_Once(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("close once panicked. %x", r)
		}
	}()

	wi := waiterItem{
		waitChan: make(chan interface{}),
	}

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		wi.Close()
	}()

	go func() {
		defer wg.Done()
		wi.Close()
	}()

	wg.Wait()
}
