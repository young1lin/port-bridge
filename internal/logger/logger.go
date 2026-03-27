package logger

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

const (
	MaxLogSize = 1 * 1024 * 1024 // 1MB
)

var (
	instance *RotatingLogger
	once     = new(sync.Once)
)

// RotatingLogger writes to both console and file, rotating when size exceeds limit
type RotatingLogger struct {
	mu       sync.Mutex
	file     *os.File
	filePath string
	writer   io.Writer
}

// Init initializes the global logger
func Init(logDir string) error {
	var err error
	once.Do(func() {
		instance, err = NewRotatingLogger(logDir)
		if err == nil {
			// Set standard log output to our multi-writer
			log.SetOutput(instance)
		}
	})
	return err
}

// NewRotatingLogger creates a new rotating logger
func NewRotatingLogger(logDir string) (*RotatingLogger, error) {
	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	logPath := filepath.Join(logDir, "forward-port.log")

	rl := &RotatingLogger{
		filePath: logPath,
	}

	if err := rl.openFile(); err != nil {
		return nil, err
	}

	// Create multi-writer for both file and stdout
	rl.writer = io.MultiWriter(os.Stdout, rl.file)

	return rl, nil
}

// openFile opens or creates the log file with secure permissions (0600)
func (rl *RotatingLogger) openFile() error {
	f, err := os.OpenFile(rl.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	rl.file = f
	return nil
}

// Write implements io.Writer
func (rl *RotatingLogger) Write(p []byte) (n int, err error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Check file size and rotate if needed
	if info, err := rl.file.Stat(); err == nil {
		if info.Size() > MaxLogSize {
			rl.rotate()
		}
	}

	return rl.writer.Write(p)
}

// rotate truncates the log file to keep only recent entries
func (rl *RotatingLogger) rotate() {
	// Close current file
	rl.file.Close()

	// Read existing content
	content, err := os.ReadFile(rl.filePath)
	if err != nil {
		// If can't read, just truncate
		rl.openFile()
		rl.writer = io.MultiWriter(os.Stdout, rl.file)
		return
	}

	// Keep only the last half of the log
	if len(content) > MaxLogSize/2 {
		// Find a good cut point (start of a line)
		cutPoint := len(content) - MaxLogSize/2
		for i := cutPoint; i < len(content); i++ {
			if content[i] == '\n' {
				cutPoint = i + 1
				break
			}
		}
		content = content[cutPoint:]
	}

	// Write trimmed content back
	rl.file, _ = os.Create(rl.filePath)
	rl.file.Write(content)
	rl.writer = io.MultiWriter(os.Stdout, rl.file)
}

// Close closes the log file
func (rl *RotatingLogger) Close() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.file != nil {
		return rl.file.Close()
	}
	return nil
}

// GetLogPath returns the log file path
func GetLogPath() string {
	if instance != nil {
		return instance.filePath
	}
	return ""
}
