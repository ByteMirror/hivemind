package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ByteMirror/hivemind/brain"
	"github.com/ByteMirror/hivemind/config"
	"github.com/ByteMirror/hivemind/log"
	"github.com/ByteMirror/hivemind/session"
)

// RunDaemon runs the daemon process which iterates over all sessions and runs AutoYes mode on them.
// It's expected that the main process kills the daemon when the main process starts.
func RunDaemon(cfg *config.Config) error {
	log.InfoLog.Printf("starting daemon")
	state := config.LoadState()
	storage, err := session.NewStorage(state)
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	instances, err := storage.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}
	for _, instance := range instances {
		// Assume AutoYes is true if the daemon is running.
		instance.AutoYes = true
	}

	// Start brain IPC server for multi-agent coordination
	var brainServer *brain.Server
	configDir, err := config.GetConfigDir()
	if err == nil {
		socketPath := filepath.Join(configDir, "hivemind.sock")
		brainServer = brain.NewServer(socketPath)
		if err := brainServer.Start(); err != nil {
			log.WarningLog.Printf("failed to start brain server: %v", err)
			brainServer = nil
		} else {
			log.InfoLog.Printf("brain server started on %s", socketPath)
		}
	}

	pollInterval := time.Duration(cfg.DaemonPollInterval) * time.Millisecond

	// If we get an error for a session, it's likely that we'll keep getting the error. Log every 60 seconds.
	everyN := log.NewEvery(60 * time.Second)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	stopCh := make(chan struct{})
	go func() {
		defer wg.Done()
		ticker := time.NewTimer(pollInterval)
		for {
			for _, instance := range instances {
				// We only store started instances, but check anyway.
				if instance.Started() && !instance.Paused() {
					if _, hasPrompt := instance.HasUpdated(); hasPrompt {
						instance.TapEnter()
						if err := instance.UpdateDiffStats(); err != nil {
							if everyN.ShouldLog() {
								log.WarningLog.Printf("could not update diff stats for %s: %v", instance.Title, err)
							}
						}
					}
				}
			}

			// Automation schedule check.
			if brainServer != nil {
				checkAutomations(brainServer)
			}

			// Handle stop before ticker.
			select {
			case <-stopCh:
				return
			default:
			}

			<-ticker.C
			ticker.Reset(pollInterval)
		}
	}()

	// Notify on SIGINT (Ctrl+C) and SIGTERM. Save instances before
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan
	log.InfoLog.Printf("received signal %s", sig.String())

	// Stop the goroutine so we don't race.
	close(stopCh)
	wg.Wait()

	if brainServer != nil {
		brainServer.Stop()
	}

	if err := storage.SaveInstances(instances); err != nil {
		log.ErrorLog.Printf("failed to save instances when terminating daemon: %v", err)
	}
	return nil
}

// LaunchDaemon launches the daemon process.
func LaunchDaemon() error {
	// Find the claude squad binary.
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	cmd := exec.Command(execPath, "--daemon")

	// Detach the process from the parent
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Set process group to prevent signals from propagating
	cmd.SysProcAttr = getSysProcAttr()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start child process: %w", err)
	}

	log.InfoLog.Printf("started daemon child process with PID: %d", cmd.Process.Pid)

	// Save PID to a file for later management
	pidDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	pidFile := filepath.Join(pidDir, "daemon.pid")
	if err := os.WriteFile(pidFile, []byte(fmt.Sprintf("%d", cmd.Process.Pid)), 0600); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	// Don't wait for the child to exit, it's detached
	return nil
}

// StopDaemon attempts to stop a running daemon process if it exists. Returns no error if the daemon is not found
// (assumes the daemon does not exist).
func StopDaemon() error {
	pidDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	pidFile := filepath.Join(pidDir, "daemon.pid")
	data, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read PID file: %w", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID file format: %w", err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find daemon process: %w", err)
	}

	if err := proc.Kill(); err != nil {
		return fmt.Errorf("failed to stop daemon process: %w", err)
	}

	// Clean up PID file
	if err := os.Remove(pidFile); err != nil {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}

	log.InfoLog.Printf("daemon process (PID: %d) stopped successfully", pid)
	return nil
}

// checkAutomations loads automations, fires any that are due, and persists updated timestamps.
func checkAutomations(brainServer *brain.Server) {
	now := time.Now()

	automations, err := config.LoadAutomations()
	if err != nil {
		log.WarningLog.Printf("daemon: failed to load automations: %v", err)
		return
	}

	changed := false
	for _, auto := range automations {
		if !auto.Enabled {
			continue
		}
		if !now.After(auto.NextRun) {
			continue
		}
		// This automation is due — trigger it.
		if err := triggerAutomation(brainServer, auto); err != nil {
			log.WarningLog.Printf("daemon: failed to trigger automation %q: %v", auto.Name, err)
		} else {
			log.InfoLog.Printf("daemon: triggered automation %q", auto.Name)
		}

		// Update LastRun and recompute NextRun regardless of trigger success.
		auto.LastRun = now
		next, err := config.NextRunTime(auto.Schedule, now, now)
		if err != nil {
			log.WarningLog.Printf("daemon: failed to compute next run for automation %q: %v", auto.Name, err)
			next = now.Add(time.Hour) // fallback: retry in 1h
		}
		auto.NextRun = next
		changed = true
	}

	if changed {
		if err := config.SaveAutomations(automations); err != nil {
			log.WarningLog.Printf("daemon: failed to save automations: %v", err)
		}
	}
}

// triggerAutomation sends a CreateInstance action to the brain server with the
// automation's ID set so the resulting instance lands in the Review Queue.
func triggerAutomation(brainServer *brain.Server, auto *config.Automation) error {
	skipPerms := true
	params := brain.CreateInstanceParams{
		Title:           fmt.Sprintf("%s-%d", auto.Name, time.Now().Unix()),
		Prompt:          auto.Instructions,
		SkipPermissions: &skipPerms,
		AutomationID:    auto.ID,
	}
	return brainServer.CreateInstanceDirect(params)
}
