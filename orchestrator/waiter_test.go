package orchestrator

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestBlockWaiter_Signal(t *testing.T) {
	item1 := &waiterItem{
		StoreName: "test_store_1",
		BlockNum:  100,
		waitChan:  make(chan interface{}),
	}
	item2 := &waiterItem{
		StoreName: "test_store_2",
		BlockNum:  300,
		waitChan:  make(chan interface{}),
	}

	waiter := &BlockWaiter{
		items: []*waiterItem{item1, item2},
		setup: sync.Once{},
		done:  make(chan interface{}),
	}

	require.Equal(t, 2, waiter.Size())

	waiter.Signal("test_store_1", 50)
	select {
	case <-waiter.Wait(context.TODO()):
		t.Errorf("waiter should not be done waiting yet")
	default:
		//
	}

	waiter.Signal("test_store_1", 150)
	select {
	case <-waiter.Wait(context.TODO()):
		t.Errorf("waiter should not be done waiting yet")
	default:
		//
	}

	waiter.Signal("test_store_2", 150)
	select {
	case <-waiter.Wait(context.TODO()):
		t.Errorf("waiter should not be done waiting yet")
	default:
		//
	}

	waiter.Signal("test_store_2", 300)

	waitDone := false
	select {
	case <-waiter.Wait(context.TODO()):
		waitDone = true
	}

	if !waitDone {
		t.Errorf("waiter should be done waiting")
	}
}
