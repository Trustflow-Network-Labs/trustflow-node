package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/adgsm/trustflow-node/cmd/cmd_helpers"
	"github.com/adgsm/trustflow-node/node_types"
)

// Job processor type
type JobProcessor func(node_types.Job) error

func processJob(job node_types.Job, processor JobProcessor) error {
	return processor(job)
}

// Worker represents a managed goroutine
type Worker struct {
	ID        int
	ctx       context.Context
	cancel    context.CancelFunc
	finished  chan struct{}
	isRunning bool
	mu        sync.RWMutex
}

// WorkerManager handles multiple workers
type WorkerManager struct {
	workers map[int]*Worker
	mu      sync.RWMutex
}

func NewWorkerManager() *WorkerManager {
	return &WorkerManager{
		workers: make(map[int]*Worker),
	}
}

// StartWorker creates and starts a new worker
func (wm *WorkerManager) StartWorker(id int, job node_types.Job) error {
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
	}

	wm.workers[id] = worker
	worker.Start(job)
	return nil
}

// StopWorker stops a specific worker
func (wm *WorkerManager) StopWorker(id int) error {
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
func (wm *WorkerManager) ListWorkers() []int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	ids := make([]int, 0, len(wm.workers))
	for id := range wm.workers {
		ids = append(ids, id)
	}
	return ids
}

func (w *Worker) Start(job node_types.Job) {
	w.mu.Lock()
	w.isRunning = true
	w.mu.Unlock()

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
				err := processJob(job, cmd_helpers.StreamData)
				if err != nil {
					// TODO
				}
			}
		}
	}()
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
