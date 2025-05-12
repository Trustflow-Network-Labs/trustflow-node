package node

import (
	"context"
	"fmt"
	"sync"
)

// Worker represents a managed goroutine
type Worker struct {
	ID        int64
	ctx       context.Context
	cancel    context.CancelFunc
	finished  chan struct{}
	isRunning bool
	mu        sync.RWMutex
}

// WorkerManager handles multiple workers
type WorkerManager struct {
	workers map[int64]*Worker
	mu      sync.RWMutex
	p2pm    *P2PManager
}

func NewWorkerManager(p2pManager *P2PManager) *WorkerManager {
	return &WorkerManager{
		workers: make(map[int64]*Worker),
		p2pm:    p2pManager,
	}
}

// StartWorker creates and starts a new worker
func (wm *WorkerManager) StartWorker(mctx context.Context, id int64, jm *JobManager) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if _, exists := wm.workers[id]; exists {
		return fmt.Errorf("worker %d already exists", id)
	}

	ctx, cancel := context.WithCancel(mctx)
	worker := &Worker{
		ID:       id,
		ctx:      ctx,
		cancel:   cancel,
		finished: make(chan struct{}),
	}

	wm.workers[id] = worker
	err := worker.Start(wm.p2pm, jm)
	if err != nil {
		return err
	}

	return nil
}

// StopWorker stops a specific worker
func (wm *WorkerManager) StopWorker(id int64) error {
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
func (wm *WorkerManager) ListWorkers() []int64 {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	ids := make([]int64, 0, len(wm.workers))
	for id := range wm.workers {
		ids = append(ids, id)
	}
	return ids
}

func (w *Worker) Start(p2pm *P2PManager, jm *JobManager) error {
	w.mu.Lock()
	w.isRunning = true
	w.mu.Unlock()

	go func() {
		defer func() {
			msg := fmt.Sprintf("Worker %d: Stopping...\n", w.ID)
			p2pm.lm.Log("info", msg, "worker")
			w.mu.Lock()
			w.isRunning = false
			w.mu.Unlock()
			close(w.finished)
			msg = fmt.Sprintf("Worker %d: Stopped completely\n", w.ID)
			p2pm.lm.Log("info", msg, "worker")
		}()

		msg := fmt.Sprintf("Worker %d: Working...\n", w.ID)
		p2pm.lm.Log("info", msg, "worker")

		// Create a done channel to run job and listen for cancellation concurrently
		errCh := make(chan error, 1)

		go func() {
			// Set job status to RUNNING
			jm.UpdateJobStatus(w.ID, "RUNNING")

			// Send job status update to remote node
			go func() {
				jm.StatusUpdate(w.ID, "RUNNING")
			}()

			errCh <- jm.StartJob(w.ID)
		}()

		select {
		case <-w.ctx.Done():
			msg := fmt.Sprintf("Worker %d: Context cancelled\n", w.ID)
			p2pm.lm.Log("info", msg, "worker")

			// Set job status to COMPLETED
			jm.UpdateJobStatus(w.ID, "COMPLETED")

			// Send job status update to remote node
			go func() {
				jm.StatusUpdate(w.ID, "COMPLETED")
			}()

		case err := <-errCh:
			if err != nil {
				msg := fmt.Sprintf("Worker %d: Job error: %v\n", w.ID, err)
				p2pm.lm.Log("error", msg, "worker")

				// Set job status to ERRORED
				// Send job status update to remote node
				jm.logAndEmitJobError(w.ID, err)
			}
		}
	}()

	return nil
}

func (w *Worker) Stop() {
	w.cancel()
	<-w.finished // Wait for worker to finish
}

func (w *Worker) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.isRunning
}
