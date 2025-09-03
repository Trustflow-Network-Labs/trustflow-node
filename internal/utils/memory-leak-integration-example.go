package utils

import (
	"context"
	"fmt"
	"io"
	"time"
)

// P2PResourceManager demonstrates how to integrate memory leak prevention
// utilities with the existing P2P manager
type P2PResourceManager struct {
	resourceTracker *ResourceTracker
	cleanupManager  *CleanupManager
	channelManager  *ChannelManager
	memoryMonitor   *MemoryPressureMonitor
	logger          Logger
	ctx             context.Context
}

// NewP2PResourceManager creates a new P2P resource manager with memory leak prevention
func NewP2PResourceManager(ctx context.Context, logger Logger) *P2PResourceManager {
	resourceTracker := NewResourceTracker(ctx, logger, nil) // nil for test compatibility
	cleanupManager := NewCleanupManager(ctx, logger)
	channelManager := NewChannelManager(ctx)

	// Create memory pressure monitor (trigger cleanup at 85% memory usage)
	memoryMonitor := NewMemoryPressureMonitor(85.0, cleanupManager, logger, nil) // nil for test compatibility
	memoryMonitor.Start(30 * time.Second) // Check every 30 seconds

	return &P2PResourceManager{
		resourceTracker: resourceTracker,
		cleanupManager:  cleanupManager,
		channelManager:  channelManager,
		memoryMonitor:   memoryMonitor,
		logger:          logger,
		ctx:             ctx,
	}
}

// HandleStreamWithAutoCleanup demonstrates proper stream handling with automatic cleanup
func (prm *P2PResourceManager) HandleStreamWithAutoCleanup(stream io.ReadWriteCloser, streamID string) error {
	// Create cleanup function for this stream
	streamCleanup := func() error {
		if err := stream.Close(); err != nil {
			prm.logger.Log("warn", fmt.Sprintf("Failed to close stream %s: %v", streamID, err), "p2p-resource-manager")
			return err
		}
		prm.logger.Log("debug", fmt.Sprintf("Stream %s cleaned up successfully", streamID), "p2p-resource-manager")
		return nil
	}

	// Track the stream for automatic cleanup
	prm.resourceTracker.TrackResourceWithCleanup(streamID, ResourceStream, streamCleanup)

	// Register cleanup with cleanup manager
	prm.cleanupManager.RegisterCleanup("stream-"+streamID, streamCleanup)

	// Create wrapped stream with automatic cleanup
	wrappedStream := NewStreamWrapper(stream, func() error {
		prm.resourceTracker.ReleaseResource(streamID)
		prm.cleanupManager.UnregisterCleanup("stream-" + streamID)
		return nil
	})

	// Use the stream with timeout and automatic cleanup
	return WithTimeout(10*time.Minute, func() error {
		return wrappedStream.Close()
	}, func(ctx context.Context) error {
		// Update last used time periodically during stream processing
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					prm.resourceTracker.UpdateLastUsed(streamID)
				}
			}
		}()

		// Simulate stream processing
		return prm.processStream(ctx, wrappedStream, streamID)
	})
}

// processStream simulates stream processing with proper buffer management
func (prm *P2PResourceManager) processStream(ctx context.Context, stream io.ReadWriter, streamID string) error {
	// Use pooled buffer for stream processing
	return SafeBufferOperation(P2PBufferPool, func(buf []byte) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// Read from stream with buffer
				n, err := stream.Read(buf)
				if err != nil {
					if err == io.EOF {
						return nil // End of stream
					}
					return fmt.Errorf("stream read error: %v", err)
				}

				// Process data (example)
				processedData := make([]byte, n)
				copy(processedData, buf[:n])

				// Echo back (example processing)
				if _, err := stream.Write(processedData); err != nil {
					return fmt.Errorf("stream write error: %v", err)
				}

				// Update resource usage
				prm.resourceTracker.UpdateLastUsed(streamID)
			}
		}
	})
}

// FileOperationWithTracking demonstrates tracked file operations
func (prm *P2PResourceManager) FileOperationWithTracking(filename string) error {
	// Open file with tracking
	file, err := prm.resourceTracker.NewTrackedFile(filename)
	if err != nil {
		return fmt.Errorf("failed to open tracked file: %v", err)
	}

	// File will be automatically cleaned up when Close() is called or on shutdown
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			prm.logger.Log("warn", fmt.Sprintf("Failed to close file %s: %v", filename, closeErr), "p2p-resource-manager")
		}
	}()

	// Use pooled buffer for file operations
	return SafeBufferOperation(FileBufferPool, func(buf []byte) error {
		// Read file content
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return fmt.Errorf("file read error: %v", err)
		}

		// Process file content
		prm.logger.Log("debug", fmt.Sprintf("Read %d bytes from %s", n, filename), "p2p-resource-manager")
		return nil
	})
}

// CreateManagedWorker creates a worker with automatic cleanup
func (prm *P2PResourceManager) CreateManagedWorker(workerID string, work func() error) {
	// Register worker cleanup
	prm.cleanupManager.RegisterCleanup("worker-"+workerID, func() error {
		prm.channelManager.CloseChannel("worker-" + workerID)
		prm.logger.Log("debug", fmt.Sprintf("Worker %s cleaned up", workerID), "p2p-resource-manager")
		return nil
	})

	// Start worker
	go func() {
		defer func() {
			// Cleanup worker on completion
			prm.cleanupManager.RunCleanup("worker-" + workerID)

			// Handle panics
			if r := recover(); r != nil {
				prm.logger.Log("error", fmt.Sprintf("Worker %s panic: %v", workerID, r), "p2p-resource-manager")
			}
		}()

		// Execute work
		if err := work(); err != nil {
			prm.logger.Log("error", fmt.Sprintf("Worker %s error: %v", workerID, err), "p2p-resource-manager")
		}

		// Signal completion
		prm.channelManager.CloseChannel("worker-" + workerID)
	}()
}

// PeriodicCleanup runs periodic cleanup tasks
func (prm *P2PResourceManager) PeriodicCleanup() {
	// Clean up old resources (older than 30 minutes)
	cleaned := prm.resourceTracker.CleanupOldResources(30 * time.Minute)
	if cleaned > 0 {
		prm.logger.Log("info", fmt.Sprintf("Cleaned up %d old resources", cleaned), "p2p-resource-manager")
	}

	// Get and log resource statistics
	stats := prm.resourceTracker.GetResourceStats()
	if len(stats) > 0 {
		prm.logger.Log("debug", fmt.Sprintf("Resource stats: %+v", stats), "p2p-resource-manager")
	}

	// Force garbage collection if needed
	memStats := ForceGC()
	prm.logger.Log("debug",
		fmt.Sprintf("GC stats - HeapInuse: %d MB, NumGC: %d",
			memStats.HeapInuse/1024/1024, memStats.NumGC),
		"p2p-resource-manager")
}

// Shutdown gracefully shuts down the resource manager
func (prm *P2PResourceManager) Shutdown() error {
	prm.logger.Log("info", "Starting P2P resource manager shutdown", "p2p-resource-manager")

	// Stop memory monitor
	prm.memoryMonitor.Stop()

	// Run final cleanup
	prm.PeriodicCleanup()

	// Shutdown all managers
	cleanupErrors := prm.cleanupManager.Shutdown()
	prm.channelManager.Shutdown()

	if shutdownErr := prm.resourceTracker.Shutdown(); shutdownErr != nil {
		cleanupErrors = append(cleanupErrors, shutdownErr)
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("shutdown errors: %v", cleanupErrors)
	}

	prm.logger.Log("info", "P2P resource manager shutdown completed", "p2p-resource-manager")
	return nil
}

// GetResourceStats returns current resource statistics
func (prm *P2PResourceManager) GetResourceStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Resource tracker stats
	resourceStats := prm.resourceTracker.GetResourceStats()
	stats["resources"] = resourceStats

	// Memory stats
	memStats := ForceGC()
	stats["memory"] = map[string]interface{}{
		"heap_inuse_mb":  memStats.HeapInuse / 1024 / 1024,
		"heap_sys_mb":    memStats.HeapSys / 1024 / 1024,
		"total_alloc_mb": memStats.TotalAlloc / 1024 / 1024,
		"num_gc":         memStats.NumGC,
		"goroutines":     fmt.Sprintf("%d", prm.ctx.Value("goroutines")), // This would need to be set elsewhere
	}

	return stats
}

// Usage example for integration with existing P2P code:
/*
// In your P2P manager initialization:
func (p2pm *P2PManager) InitResourceManager() {
    p2pm.resourceManager = NewP2PResourceManager(p2pm.ctx, p2pm.Lm)

    // Start periodic cleanup
    go func() {
        ticker := time.NewTicker(10 * time.Minute)
        defer ticker.Stop()

        for {
            select {
            case <-p2pm.ctx.Done():
                return
            case <-ticker.C:
                p2pm.resourceManager.PeriodicCleanup()
            }
        }
    }()
}

// In your stream handler:
func (p2pm *P2PManager) streamProposalResponse(s network.Stream) {
    streamID := s.ID()

    // Use resource manager for automatic cleanup
    if err := p2pm.resourceManager.HandleStreamWithAutoCleanup(s, streamID); err != nil {
        p2pm.Lm.Log("error", fmt.Sprintf("Stream handling error: %v", err), "p2p")
        s.Reset()
        return
    }
}

// In your shutdown function:
func (p2pm *P2PManager) Shutdown() error {
    return p2pm.resourceManager.Shutdown()
}
*/
