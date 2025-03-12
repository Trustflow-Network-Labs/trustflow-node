package shared

import (
	"context"
	"fmt"
	"sync"

	"github.com/adgsm/trustflow-node/node_types"
)

// Worker represents a managed goroutine
type Worker struct {
	ID        int32
	ctx       context.Context
	cancel    context.CancelFunc
	finished  chan struct{}
	isRunning bool
	job       node_types.Job
	mu        sync.RWMutex
}

// WorkerManager handles multiple workers
type WorkerManager struct {
	workers map[int32]*Worker
	mu      sync.RWMutex
	p2pm    *P2PManager
}

func NewWorkerManager(p2pManager *P2PManager) *WorkerManager {
	return &WorkerManager{
		workers: make(map[int32]*Worker),
		p2pm:    p2pManager,
	}
}

// StartWorker creates and starts a new worker
func (wm *WorkerManager) StartWorker(id int32, job node_types.Job) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, exists := wm.workers[id]; exists {
		return fmt.Errorf("worker %d already exists", id)
	}

	ctx, cancel := context.WithCancel(context.Background())
	worker := &Worker{
		ID:       id,
		ctx:      ctx,
		cancel:   cancel,
		finished: make(chan struct{}),
		job:      job,
	}

	wm.workers[id] = worker
	err := worker.Start(wm.p2pm)
	if err != nil {
		return err
	}

	return nil
}

// StopWorker stops a specific worker
func (wm *WorkerManager) StopWorker(id int32) error {
	wm.mu.RLock()
	worker, exists := wm.workers[id]
	wm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worker %d not found", id)
	}

	worker.Stop()

	wm.mu.Lock()
	delete(wm.workers, id)
	wm.mu.Unlock()

	return nil
}

// ListWorkers returns IDs of all active workers
func (wm *WorkerManager) ListWorkers() []int32 {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	ids := make([]int32, 0, len(wm.workers))
	for id := range wm.workers {
		ids = append(ids, id)
	}
	return ids
}

func (w *Worker) Start(p2pm *P2PManager) error {
	w.mu.Lock()
	w.isRunning = true
	w.mu.Unlock()

	var e error = nil

	go func() {
		defer close(w.finished)

		for {
			select {
			case <-w.ctx.Done():
				fmt.Printf("Worker %d: Stopping...\n", w.ID)
				w.mu.Lock()
				w.isRunning = false
				w.mu.Unlock()
				return
			default:
				fmt.Printf("Worker %d: Working...\n", w.ID)
				jobManager := NewJobManager(p2pm)
				err := jobManager.StartJob(w.job)
				if err != nil {
					e = err
				}
				w.Stop()
			}
		}
	}()

	return e
}

func (w *Worker) Stop() {
	w.cancel()
	<-w.finished // Wait for worker to finish
	fmt.Printf("Worker %d: Stopped completely\n", w.ID)
}

func (w *Worker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.isRunning
}
