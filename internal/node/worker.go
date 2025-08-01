package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/adgsm/trustflow-node/internal/utils"
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
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewWorkerManager(p2pManager *P2PManager) *WorkerManager {
	ctx, cancel := context.WithCancel(p2pManager.ctx)

	wm := &WorkerManager{
		workers: make(map[int64]*Worker),
		p2pm:    p2pManager,
		ctx:     ctx,
		cancel:  cancel,
	}

	// Start cleanup routine
	go wm.periodicCleanup()

	return wm
}

// Periodic cleanup for finished workers
func (wm *WorkerManager) periodicCleanup() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			wm.cleanupFinishedWorkers()
		case <-wm.ctx.Done():
			return
		}
	}
}

// Cleanup finished workers to prevent map growth
func (wm *WorkerManager) cleanupFinishedWorkers() {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	for id, worker := range wm.workers {
		if !worker.IsRunning() {
			delete(wm.workers, id)
			wm.p2pm.Lm.Log("debug", fmt.Sprintf("Cleaned up finished worker %d", id), "worker")
		}
	}
}

// Shutdown
func (wm *WorkerManager) Shutdown() {
	wm.mu.Lock()
	workers := make([]*Worker, 0, len(wm.workers))
	for _, worker := range wm.workers {
		workers = append(workers, worker)
	}
	wm.mu.Unlock()

	// Stop all workers
	for _, worker := range workers {
		worker.Stop()
	}

	wm.cancel() // Cancel cleanup routine
}

// StartWorker creates and starts a new worker
func (wm *WorkerManager) StartWorker(mctx context.Context, id int64, jm *JobManager, retry, maxRetries int) error {
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
	err := worker.Start(wm.p2pm, jm, retry, maxRetries)
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

func (w *Worker) Start(p2pm *P2PManager, jm *JobManager, retry, maxRetries int) error {
	w.mu.Lock()
	w.isRunning = true
	w.mu.Unlock()

	go func() {
		defer func() {
			msg := fmt.Sprintf("Worker %d: Stopping...\n", w.ID)
			p2pm.Lm.Log("info", msg, "worker")
			w.mu.Lock()
			w.isRunning = false
			w.mu.Unlock()
			close(w.finished)
			msg = fmt.Sprintf("Worker %d: Stopped completely\n", w.ID)
			p2pm.Lm.Log("info", msg, "worker")
		}()

		msg := fmt.Sprintf("Worker %d: Working...\n", w.ID)
		p2pm.Lm.Log("info", msg, "worker")

		// Set initial job status to RUNNING
		jm.UpdateJobStatus(w.ID, "RUNNING")
		w.sendStatusUpdateAsync(jm, "RUNNING")

		// Get an error channel from the pool to reduce allocations
		errCh := utils.GlobalErrorChannelPool.Get()
		defer utils.GlobalErrorChannelPool.Put(errCh)

		// Start job execution in separate goroutine
		go func() {
			// Execute the job and send result to error channel
			jobErr := jm.StartJob(w.ID)
			select {
			case errCh <- jobErr:
				// Successfully sent result
			case <-w.ctx.Done():
				// Context cancelled, don't send
			}
		}()

		select {
		case <-w.ctx.Done():
			msg := fmt.Sprintf("Worker %d: Context cancelled\n", w.ID)
			p2pm.Lm.Log("info", msg, "worker")

			// Set job status to COMPLETED
			jm.UpdateJobStatus(w.ID, "COMPLETED")
			w.sendStatusUpdateAsync(jm, "COMPLETED")

		case err := <-errCh:
			
			if err != nil {
				msg := fmt.Sprintf("Worker %d: Job error: %v\n", w.ID, err)
				p2pm.Lm.Log("error", msg, "worker")

				if retry < maxRetries-1 {
					// Set job status to READY for retry
					jm.UpdateJobStatus(w.ID, "READY")
					w.sendStatusUpdateAsync(jm, "READY")
				} else {
					// Set job status to ERRORED - no more retries
					jm.logAndEmitJobError(w.ID, err)
				}
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

// sendStatusUpdateAsync sends status updates without blocking
// Uses a bounded goroutine to prevent goroutine leaks
func (w *Worker) sendStatusUpdateAsync(jm *JobManager, status string) {
	select {
	case <-w.ctx.Done():
		// Context cancelled, don't send update
		return
	default:
	}

	// Use a goroutine with timeout to prevent blocking indefinitely
	go func() {
		// Create a timeout context for the status update
		ctx, cancel := context.WithTimeout(w.ctx, time.Second*10)
		defer cancel()

		// Create a channel to signal completion
		done := make(chan struct{})
		
		go func() {
			defer close(done)
			jm.StatusUpdate(w.ID, status)
		}()

		// Wait for completion or timeout
		select {
		case <-done:
			// Status update completed successfully
		case <-ctx.Done():
			// Timeout or context cancelled
			// Log but don't block worker execution
		}
	}()
}
