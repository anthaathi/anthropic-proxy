package config

import (
	"anthropic-proxy/logger"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Reloader watches for config file changes and triggers reload callbacks
type Reloader struct {
	configPath string
	watcher    *fsnotify.Watcher
	onReload   func() error
	debounce   time.Duration
	stopCh     chan struct{}
}

// NewReloader creates a new config file reloader
func NewReloader(configPath string, onReload func() error) (*Reloader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Reloader{
		configPath: configPath,
		watcher:    watcher,
		onReload:   onReload,
		debounce:   500 * time.Millisecond, // Debounce rapid saves
		stopCh:     make(chan struct{}),
	}, nil
}

// Start begins watching for config file changes
func (r *Reloader) Start() error {
	logger.Info("Starting config file watcher", "path", r.configPath)

	// Add the config file to the watcher
	if err := r.watcher.Add(r.configPath); err != nil {
		return err
	}

	go r.watchLoop()
	return nil
}

// Stop stops the config file watcher
func (r *Reloader) Stop() {
	logger.Info("Stopping config file watcher")
	close(r.stopCh)
	r.watcher.Close()
}

// watchLoop watches for file system events and triggers reloads
func (r *Reloader) watchLoop() {
	var timer *time.Timer

	for {
		select {
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}

			logger.Info("Config file event detected", "op", event.Op.String(), "name", event.Name)

			// Handle write, create, rename, and remove events
			// Some editors use atomic saves (write to temp file, then rename)
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
				// If file was renamed or removed, re-add watch
				// This handles editors that use atomic saves
				if event.Op&(fsnotify.Rename|fsnotify.Remove) != 0 {
					logger.Info("File renamed or removed, re-adding watch")
					// Wait a bit for the new file to be created
					time.Sleep(100 * time.Millisecond)
					r.watcher.Remove(r.configPath) // Remove old watch if it exists
					if err := r.watcher.Add(r.configPath); err != nil {
						logger.Error("Failed to re-add watch", "error", err.Error())
					} else {
						logger.Info("Successfully re-added watch", "path", r.configPath)
					}
				}

				// Debounce rapid saves
				if timer != nil {
					timer.Stop()
				}

				timer = time.AfterFunc(r.debounce, func() {
					logger.Info("Config file changed, triggering reload")
					if err := r.onReload(); err != nil {
						logger.Error("Config reload failed", "error", err.Error())
					}
				})
			}

		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			logger.Error("Config file watcher error", "error", err.Error())

		case <-r.stopCh:
			// Stop the timer if it's running
			if timer != nil {
				timer.Stop()
			}
			return
		}
	}
}

// WatchConfig returns whether watching is enabled
func (r *Reloader) WatchConfig() bool {
	return r != nil
}