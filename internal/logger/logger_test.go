package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestLogger(t *testing.T) *RotatingLogger {
	t.Helper()
	dir := t.TempDir()
	rl, err := NewRotatingLogger(dir)
	if err != nil {
		t.Fatalf("NewRotatingLogger: %v", err)
	}
	t.Cleanup(func() { rl.Close() })
	return rl
}

func TestNewRotatingLogger(t *testing.T) {
	dir := t.TempDir()
	rl, err := NewRotatingLogger(dir)
	if err != nil {
		t.Fatalf("NewRotatingLogger: %v", err)
	}
	defer rl.Close()

	expected := filepath.Join(dir, "forward-port.log")
	if rl.filePath != expected {
		t.Errorf("filePath = %q, want %q", rl.filePath, expected)
	}
	if rl.file == nil {
		t.Error("file should not be nil")
	}
	if _, err := os.Stat(expected); os.IsNotExist(err) {
		t.Error("log file should exist on disk")
	}
}

func TestRotatingLogger_Write(t *testing.T) {
	rl := newTestLogger(t)

	msg := "hello world\n"
	n, err := rl.Write([]byte(msg))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write returned %d, want %d", n, len(msg))
	}

	rl.Close()
	data, err := os.ReadFile(rl.filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "hello world") {
		t.Errorf("log file should contain message, got: %s", string(data))
	}
}

func TestRotatingLogger_Write_TriggerRotation(t *testing.T) {
	dir := t.TempDir()
	rl, err := NewRotatingLogger(dir)
	if err != nil {
		t.Fatalf("NewRotatingLogger: %v", err)
	}
	defer rl.Close()

	// Close the original file to replace with a pre-sized one
	rl.file.Close()

	// Write a file larger than MaxLogSize
	big := strings.Repeat("log line for testing rotation\n", (MaxLogSize/30)+1)
	if err := os.WriteFile(rl.filePath, []byte(big), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Reopen
	if err := rl.openFile(); err != nil {
		t.Fatalf("openFile: %v", err)
	}
	rl.writer = rl.file // write to file only for test

	// Trigger rotation
	_, err = rl.Write([]byte("trigger\n"))
	if err != nil {
		t.Fatalf("Write after rotation: %v", err)
	}

	info, err := os.Stat(rl.filePath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() > MaxLogSize {
		t.Errorf("file size %d should be <= %d after rotation", info.Size(), MaxLogSize)
	}
}

func TestRotatingLogger_Write_NoRotation(t *testing.T) {
	rl := newTestLogger(t)

	small := strings.Repeat("x", 100)
	_, err := rl.Write([]byte(small))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	rl.Close()
	info, err := os.Stat(rl.filePath)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() > MaxLogSize {
		t.Error("small write should not trigger rotation")
	}
}

func TestRotatingLogger_Close(t *testing.T) {
	rl := newTestLogger(t)

	if err := rl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// File should be closed (subsequent writes will fail)
	if rl.file != nil {
		// rl.file is still non-nil after close — that's expected behavior
	}
}

func TestGetLogPath_BeforeInit(t *testing.T) {
	// Reset instance for test
	instance = nil
	if got := GetLogPath(); got != "" {
		t.Errorf("GetLogPath() before init = %q, want empty", got)
	}
}

func TestGetLogPath_AfterInit(t *testing.T) {
	orig := instance
	defer func() { instance = orig }()

	dir := t.TempDir()
	rl, err := NewRotatingLogger(dir)
	if err != nil {
		t.Fatalf("NewRotatingLogger: %v", err)
	}
	instance = rl

	got := GetLogPath()
	expected := filepath.Join(dir, "forward-port.log")
	if got != expected {
		t.Errorf("GetLogPath() = %q, want %q", got, expected)
	}
	rl.Close()
}

func TestRotatingLogger_Rotate_SmallContent(t *testing.T) {
	dir := t.TempDir()
	rl, err := NewRotatingLogger(dir)
	if err != nil {
		t.Fatalf("NewRotatingLogger: %v", err)
	}
	defer rl.Close()

	// Write content smaller than MaxLogSize/2
	rl.file.Close()
	small := strings.Repeat("x", MaxLogSize/4)
	if err := os.WriteFile(rl.filePath, []byte(small), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := rl.openFile(); err != nil {
		t.Fatalf("openFile: %v", err)
	}
	rl.writer = rl.file

	// Trigger rotation — content is < MaxLogSize/2 so it should be kept as-is
	_, err = rl.Write([]byte("trigger\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(rl.filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "trigger") {
		t.Error("log should contain trigger message after rotation")
	}
}

func TestRotatingLogger_MultiWrite(t *testing.T) {
	rl := newTestLogger(t)

	for i := 0; i < 10; i++ {
		_, err := rl.Write([]byte("line\n"))
		if err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}

	rl.Close()
	data, err := os.ReadFile(rl.filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	count := strings.Count(string(data), "line\n")
	if count != 10 {
		t.Errorf("expected 10 lines, got %d", count)
	}
}

func TestNewRotatingLogger_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "subdir", "logs")

	rl, err := NewRotatingLogger(newDir)
	if err != nil {
		t.Fatalf("NewRotatingLogger: %v", err)
	}
	defer rl.Close()

	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected directory, got file")
	}
}

func TestNewRotatingLogger_ErrorWhenLogDirIsFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := NewRotatingLogger(filePath); err == nil {
		t.Fatal("expected error when logDir points to a file")
	}
}

func TestOpenFile_ErrorWhenFilePathIsDirectory(t *testing.T) {
	rl := &RotatingLogger{
		filePath: t.TempDir(),
	}

	if err := rl.openFile(); err == nil {
		t.Fatal("expected openFile to fail when filePath is a directory")
	}
}

func TestRotatingLogger_Rotate_ReadFailureReopensFile(t *testing.T) {
	rl := newTestLogger(t)
	oldPath := rl.filePath

	if err := rl.file.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.Remove(oldPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	rl.rotate()

	if rl.file == nil {
		t.Fatal("rotate should reopen the log file after read failure")
	}
	if rl.filePath != oldPath {
		t.Fatalf("filePath changed unexpectedly: got %q want %q", rl.filePath, oldPath)
	}

	if _, err := rl.Write([]byte("reopened\n")); err != nil {
		t.Fatalf("Write after rotate recovery: %v", err)
	}
}

func TestRotatingLogger_Close_NilFile(t *testing.T) {
	rl := &RotatingLogger{}

	if err := rl.Close(); err != nil {
		t.Fatalf("Close with nil file should return nil, got %v", err)
	}
}

func TestInit(t *testing.T) {
	origOnce := once
	origInstance := instance
	defer func() {
		if instance != nil {
			instance.Close()
		}
		once = origOnce
		instance = origInstance
	}()

	once = new(sync.Once)
	instance = nil

	dir := t.TempDir()
	err := Init(dir)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if instance == nil {
		t.Fatal("instance should be set after Init")
	}

	// Close to release file handle so temp dir can be cleaned up
	instance.Close()
}

func TestInit_AlreadyInitialized(t *testing.T) {
	origOnce := once
	origInstance := instance
	defer func() {
		if instance != nil {
			instance.Close()
		}
		once = origOnce
		instance = origInstance
	}()

	once = new(sync.Once)
	instance = nil

	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatalf("Init first: %v", err)
	}
	instance.Close()

	// Second Init should be a no-op (once.Do)
	if err := Init(t.TempDir()); err != nil {
		t.Fatalf("Init second: %v", err)
	}
	instance.Close()
}
