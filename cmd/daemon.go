// daemon.go — nginx-style daemon management for nanobot server.
//
// Usage:
//
//	nanobot server start    — start as background daemon (spawns N workers)
//	nanobot server stop     — send SIGTERM to all workers
//	nanobot server restart  — stop + start
//	nanobot server reload   — send SIGHUP to all workers
//	nanobot server status   — check running workers
//	nanobot server          — run single foreground process
//
// Workers: set "gateway.workers" in config.json (default 1).
// Each worker runs on port basePort+i and registers independently to the backend.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dayuer/nanobot-go/internal/config"
	"github.com/spf13/cobra"
)

const pidFileName = "nanobot.pid"

func init() {
	serverCmd.AddCommand(startCmd)
	serverCmd.AddCommand(stopCmd)
	serverCmd.AddCommand(restartCmd)
	serverCmd.AddCommand(reloadCmd)
	serverCmd.AddCommand(serverStatusCmd)
}

// --- PID file helpers (multi-worker: one PID per line) ---

func pidFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nanobot", pidFileName)
}

func writePIDs(pids []int) error {
	dir := filepath.Dir(pidFilePath())
	os.MkdirAll(dir, 0755)
	lines := make([]string, len(pids))
	for i, p := range pids {
		lines[i] = strconv.Itoa(p)
	}
	return os.WriteFile(pidFilePath(), []byte(strings.Join(lines, "\n")), 0644)
}

// writePID writes a single PID (used in foreground mode)
func writePID(pid int) error {
	return writePIDs([]int{pid})
}

func readPIDs() ([]int, error) {
	data, err := os.ReadFile(pidFilePath())
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var pids []int
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		pid, err := strconv.Atoi(l)
		if err != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

func removePID() {
	os.Remove(pidFilePath())
}

// isRunning checks if a process with the given PID is alive.
func isRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// getRunningPIDs returns all currently alive worker PIDs.
func getRunningPIDs() []int {
	pids, err := readPIDs()
	if err != nil {
		return nil
	}
	var alive []int
	for _, pid := range pids {
		if isRunning(pid) {
			alive = append(alive, pid)
		}
	}
	if len(alive) == 0 {
		removePID()
	}
	return alive
}

// For backward compat with server.go foreground mode
func getRunningPID() (int, bool) {
	pids := getRunningPIDs()
	if len(pids) == 0 {
		return 0, false
	}
	return pids[0], true
}

// getWorkerCount reads config to determine number of workers.
func getWorkerCount() int {
	cfg, err := config.Load("")
	if err != nil {
		return 1
	}
	if cfg.Gateway.Workers > 0 {
		return cfg.Gateway.Workers
	}
	return 1
}

// --- Helper: spawn a single worker process ---

func spawnWorker(exe string, port int, workerID int) (*os.Process, string, error) {
	serverArgs := []string{"server", "--port", strconv.Itoa(port)}
	if serverAPIKey != "" {
		serverArgs = append(serverArgs, "--api-key", serverAPIKey)
	}
	if registryURL != "" {
		serverArgs = append(serverArgs, "--registry", registryURL)
	}
	if agentsFile != "" {
		serverArgs = append(serverArgs, "--agents", agentsFile)
	}

	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".nanobot")
	os.MkdirAll(logDir, 0755)

	var logFile string
	if workerID == 0 {
		logFile = filepath.Join(logDir, "nanobot.log")
	} else {
		logFile = filepath.Join(logDir, fmt.Sprintf("nanobot-worker%d.log", workerID))
	}

	outFile, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, "", fmt.Errorf("cannot open log file: %w", err)
	}

	proc := exec.Command(exe, serverArgs...)
	proc.Stdout = outFile
	proc.Stderr = outFile
	proc.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	proc.Env = os.Environ()

	if err := proc.Start(); err != nil {
		outFile.Close()
		return nil, "", fmt.Errorf("failed to start worker: %w", err)
	}
	outFile.Close()

	return proc.Process, logFile, nil
}

// --- Helper: stop all PIDs ---

func stopAllWorkers(pids []int, timeout time.Duration) {
	// Send SIGTERM to all
	for _, pid := range pids {
		if proc, err := os.FindProcess(pid); err == nil {
			proc.Signal(syscall.SIGTERM)
		}
	}

	// Wait for all to exit
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		allDead := true
		for _, pid := range pids {
			if isRunning(pid) {
				allDead = false
				break
			}
		}
		if allDead {
			removePID()
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Force kill remaining
	for _, pid := range pids {
		if isRunning(pid) {
			if proc, err := os.FindProcess(pid); err == nil {
				proc.Signal(syscall.SIGKILL)
			}
		}
	}
	time.Sleep(500 * time.Millisecond)
	removePID()
}

// --- Subcommands ---

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start nanobot server as background daemon(s)",
	Long: `Start nanobot server as background daemon(s).
Set "gateway.workers" in config.json to spawn multiple workers.
Each worker runs on a consecutive port (basePort, basePort+1, ...) and
registers independently to the backend pool, forming a service cluster.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if pids := getRunningPIDs(); len(pids) > 0 {
			return fmt.Errorf("nanobot server is already running (%d workers, PIDs: %v)", len(pids), pids)
		}

		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot find executable: %w", err)
		}

		workers := getWorkerCount()
		basePort := serverPort
		if basePort == 18790 {
			cfg, err := config.Load("")
			if err == nil && cfg.Gateway.Port != 0 {
				basePort = cfg.Gateway.Port
			}
		}

		fmt.Printf("🚀 Starting nanobot cluster (%d workers)...\n", workers)

		var allPIDs []int
		for i := 0; i < workers; i++ {
			port := basePort + i
			proc, logFile, err := spawnWorker(exe, port, i)
			if err != nil {
				// Stop any already-started workers
				if len(allPIDs) > 0 {
					fmt.Printf("⚠️ Worker %d failed, stopping %d started workers...\n", i, len(allPIDs))
					stopAllWorkers(allPIDs, 5*time.Second)
				}
				return fmt.Errorf("worker %d (port %d): %w", i, port, err)
			}

			pid := proc.Pid
			allPIDs = append(allPIDs, pid)
			proc.Release()

			fmt.Printf("   ✅ Worker %d → port %d (PID %d, log: %s)\n", i, port, pid, filepath.Base(logFile))
		}

		writePIDs(allPIDs)

		home, _ := os.UserHomeDir()
		fmt.Printf("\n✅ Cluster started: %d workers on ports %d-%d\n", workers, basePort, basePort+workers-1)
		fmt.Printf("   PID file: %s\n", pidFilePath())
		fmt.Printf("   Logs: %s/nanobot*.log\n", filepath.Join(home, ".nanobot"))
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop all running nanobot server workers",
	RunE: func(cmd *cobra.Command, args []string) error {
		pids := getRunningPIDs()
		if len(pids) == 0 {
			fmt.Println("ℹ️ nanobot server is not running")
			return nil
		}

		fmt.Printf("🛑 Stopping %d worker(s) (PIDs: %v)...\n", len(pids), pids)
		stopAllWorkers(pids, 10*time.Second)
		fmt.Println("✅ All workers stopped")
		return nil
	},
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart all nanobot server workers",
	RunE: func(cmd *cobra.Command, args []string) error {
		pids := getRunningPIDs()
		if len(pids) > 0 {
			fmt.Printf("🔄 Restarting %d worker(s)...\n", len(pids))
			stopAllWorkers(pids, 10*time.Second)
			fmt.Println("   Old workers stopped")
		}
		return startCmd.RunE(cmd, args)
	},
}

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Send SIGHUP to all workers (reload config)",
	RunE: func(cmd *cobra.Command, args []string) error {
		pids := getRunningPIDs()
		if len(pids) == 0 {
			return fmt.Errorf("nanobot server is not running")
		}

		for _, pid := range pids {
			if proc, err := os.FindProcess(pid); err == nil {
				proc.Signal(syscall.SIGHUP)
			}
		}
		fmt.Printf("✅ Reload signal sent to %d worker(s) (PIDs: %v)\n", len(pids), pids)
		return nil
	},
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check nanobot server workers status",
	Run: func(cmd *cobra.Command, args []string) {
		pids := getRunningPIDs()
		if len(pids) == 0 {
			fmt.Println("⚫ nanobot server is not running")
			return
		}

		fmt.Printf("✅ nanobot server: %d worker(s) running\n", len(pids))
		for i, pid := range pids {
			fmt.Printf("   Worker %d: PID %d ✅\n", i, pid)
		}
		fmt.Printf("   PID file: %s\n", pidFilePath())

		// Show log tail from main worker
		home, _ := os.UserHomeDir()
		logFile := filepath.Join(home, ".nanobot", "nanobot.log")
		if data, err := os.ReadFile(logFile); err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			start := len(lines) - 5
			if start < 0 {
				start = 0
			}
			fmt.Println("   Last log lines (worker 0):")
			for _, l := range lines[start:] {
				fmt.Printf("     %s\n", l)
			}
		}
	},
}
