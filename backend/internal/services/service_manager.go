package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ServiceManager manages Python microservices (STT, TTS) and Node.js voice bridge
type ServiceManager struct {
	logger        *zap.Logger
	sttCmd        *exec.Cmd
	ttsCmd        *exec.Cmd
	bridgeCmd     *exec.Cmd
	sttCancel     context.CancelFunc
	ttsCancel     context.CancelFunc
	bridgeCancel  context.CancelFunc
	wg            sync.WaitGroup
	mu            sync.Mutex
	basePath      string
	sttServiceURL string
	ttsServiceURL string
}

// NewServiceManager creates a new service manager
func NewServiceManager(logger *zap.Logger, basePath, sttServiceURL, ttsServiceURL string) *ServiceManager {
	return &ServiceManager{
		logger:        logger,
		basePath:      basePath,
		sttServiceURL: sttServiceURL,
		ttsServiceURL: ttsServiceURL,
	}
}

// StartSTTService starts the STT service
func (sm *ServiceManager) StartSTTService() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.sttCmd != nil {
		return fmt.Errorf("STT service already running")
	}

	// Get the Python executable from the venv
	pythonPath := filepath.Join(sm.basePath, "services", "stt_service", ".venv", "Scripts", "python.exe")
	if _, err := os.Stat(pythonPath); os.IsNotExist(err) {
		// Try Unix-style path
		pythonPath = filepath.Join(sm.basePath, "services", "stt_service", ".venv", "bin", "python")
		if _, err := os.Stat(pythonPath); os.IsNotExist(err) {
			return fmt.Errorf("STT service Python not found at %s", pythonPath)
		}
	}

	mainPath := filepath.Join(sm.basePath, "services", "stt_service", "main.py")

	ctx, cancel := context.WithCancel(context.Background())
	sm.sttCancel = cancel

	cmd := exec.CommandContext(ctx, pythonPath, mainPath)
	cmd.Dir = filepath.Join(sm.basePath, "services", "stt_service")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the command (non-blocking)
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start STT service: %w", err)
	}

	sm.sttCmd = cmd
	sm.wg.Add(1)

	go func() {
		defer sm.wg.Done()
		if err := cmd.Wait(); err != nil {
			// Only log if it's not a normal exit (context cancellation)
			if ctx.Err() == nil {
				sm.logger.Error("STT service exited", zap.Error(err))
			}
		}
		sm.mu.Lock()
		sm.sttCmd = nil
		sm.mu.Unlock()
	}()

	// Wait a bit to see if it starts successfully
	time.Sleep(2 * time.Second)
	if cmd.Process == nil {
		return fmt.Errorf("STT service process is nil")
	}
	// On Windows, we can't easily check if process is running without platform-specific code
	// Instead, we'll just verify the process was created and let it run
	// If it fails, the goroutine will log the error

	sm.logger.Info("STT service started",
		zap.String("python", pythonPath),
		zap.String("service_url", sm.sttServiceURL))

	return nil
}

// StartTTSService starts the TTS service
func (sm *ServiceManager) StartTTSService() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.ttsCmd != nil {
		return fmt.Errorf("TTS service already running")
	}

	// Get the Python executable from the venv
	pythonPath := filepath.Join(sm.basePath, "services", "tts_service", ".venv", "Scripts", "python.exe")
	if _, err := os.Stat(pythonPath); os.IsNotExist(err) {
		// Try Unix-style path
		pythonPath = filepath.Join(sm.basePath, "services", "tts_service", ".venv", "bin", "python")
		if _, err := os.Stat(pythonPath); os.IsNotExist(err) {
			return fmt.Errorf("TTS service Python not found at %s", pythonPath)
		}
	}

	mainPath := filepath.Join(sm.basePath, "services", "tts_service", "main.py")

	ctx, cancel := context.WithCancel(context.Background())
	sm.ttsCancel = cancel

	cmd := exec.CommandContext(ctx, pythonPath, mainPath)
	cmd.Dir = filepath.Join(sm.basePath, "services", "tts_service")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the command
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start TTS service: %w", err)
	}

	sm.ttsCmd = cmd
	sm.wg.Add(1)

	go func() {
		defer sm.wg.Done()
		if err := cmd.Wait(); err != nil {
			// Only log if it's not a normal exit (context cancellation)
			if ctx.Err() == nil {
				sm.logger.Error("TTS service exited", zap.Error(err))
			}
		}
		sm.mu.Lock()
		sm.ttsCmd = nil
		sm.mu.Unlock()
	}()

	// Wait a bit to see if it starts successfully
	time.Sleep(2 * time.Second)
	if cmd.Process == nil {
		return fmt.Errorf("TTS service process is nil")
	}
	// On Windows, we can't easily check if process is running without platform-specific code
	// Instead, we'll just verify the process was created and let it run
	// If it fails, the goroutine will log the error

	sm.logger.Info("TTS service started",
		zap.String("python", pythonPath),
		zap.String("service_url", sm.ttsServiceURL))

	return nil
}

// StartVoiceBridge starts the Node.js voice bridge service
func (sm *ServiceManager) StartVoiceBridge() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.bridgeCmd != nil {
		return fmt.Errorf("Voice bridge service already running")
	}

	// Find node executable
	nodePath, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("node executable not found: %w", err)
	}

	bridgeDir := filepath.Join(sm.basePath, "services", "voice_bridge")
	indexPath := filepath.Join(bridgeDir, "index.js")
	
	// Check if node_modules exists, if not, try to install dependencies
	nodeModulesPath := filepath.Join(bridgeDir, "node_modules")
	if _, err := os.Stat(nodeModulesPath); os.IsNotExist(err) {
		sm.logger.Warn("node_modules not found, attempting to install dependencies...")
		npmPath, err := exec.LookPath("npm")
		if err != nil {
			sm.logger.Warn("npm not found, skipping dependency installation",
				zap.String("note", "Please run 'npm install' in services/voice_bridge manually"))
		} else {
			// Use --ignore-scripts to skip native compilation (we use pure JS libsodium-wrappers)
			// This avoids Visual Studio build tool requirements
			installCmd := exec.Command(npmPath, "install", "--ignore-scripts")
			installCmd.Dir = bridgeDir
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
			if err := installCmd.Run(); err != nil {
				sm.logger.Warn("Failed to install dependencies automatically",
					zap.Error(err),
					zap.String("note", "Please run 'npm install --ignore-scripts' in services/voice_bridge manually"))
			} else {
				sm.logger.Info("Dependencies installed successfully (skipped native compilation)")
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	sm.bridgeCancel = cancel

	cmd := exec.CommandContext(ctx, nodePath, indexPath)
	cmd.Dir = filepath.Join(sm.basePath, "services", "voice_bridge")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Start the command (non-blocking)
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start voice bridge service: %w", err)
	}

	sm.bridgeCmd = cmd
	sm.wg.Add(1)

	go func() {
		defer sm.wg.Done()
		if err := cmd.Wait(); err != nil {
			// Only log if it's not a normal exit (context cancellation)
			if ctx.Err() == nil {
				sm.logger.Error("Voice bridge service exited", zap.Error(err))
			}
		}
		sm.mu.Lock()
		sm.bridgeCmd = nil
		sm.mu.Unlock()
	}()

	// Wait a bit to see if it starts successfully
	time.Sleep(2 * time.Second)
	if cmd.Process == nil {
		return fmt.Errorf("Voice bridge service process is nil")
	}

	sm.logger.Info("Voice bridge service started",
		zap.String("node", nodePath),
		zap.String("script", indexPath))

	return nil
}

// StartAll starts STT, TTS, and voice bridge services
func (sm *ServiceManager) StartAll() error {
	// Start voice bridge first since other services depend on it
	if err := sm.StartVoiceBridge(); err != nil {
		sm.logger.Warn("Failed to start voice bridge service", zap.Error(err))
		// Continue - service might be started manually
	}

	if err := sm.StartSTTService(); err != nil {
		sm.logger.Warn("Failed to start STT service", zap.Error(err))
		// Continue - services might be started manually
	}

	if err := sm.StartTTSService(); err != nil {
		sm.logger.Warn("Failed to start TTS service", zap.Error(err))
		// Continue - services might be started manually
	}

	return nil
}

// StopAll stops all services
func (sm *ServiceManager) StopAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.sttCancel != nil {
		sm.sttCancel()
		sm.sttCancel = nil
	}

	if sm.ttsCancel != nil {
		sm.ttsCancel()
		sm.ttsCancel = nil
	}

	if sm.bridgeCancel != nil {
		sm.bridgeCancel()
		sm.bridgeCancel = nil
	}

	// Wait for processes to exit
	done := make(chan struct{})
	go func() {
		sm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		sm.logger.Info("All services stopped")
	case <-time.After(5 * time.Second):
		sm.logger.Warn("Services did not stop gracefully, forcing termination")
		if sm.sttCmd != nil && sm.sttCmd.Process != nil {
			sm.sttCmd.Process.Kill()
		}
		if sm.ttsCmd != nil && sm.ttsCmd.Process != nil {
			sm.ttsCmd.Process.Kill()
		}
		if sm.bridgeCmd != nil && sm.bridgeCmd.Process != nil {
			sm.bridgeCmd.Process.Kill()
		}
	}
}

// IsSTTRunning checks if STT service is running
func (sm *ServiceManager) IsSTTRunning() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.sttCmd != nil && sm.sttCmd.Process != nil
}

// IsTTSRunning checks if TTS service is running
func (sm *ServiceManager) IsTTSRunning() bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.ttsCmd != nil && sm.ttsCmd.Process != nil
}

