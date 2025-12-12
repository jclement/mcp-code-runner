package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Manager handles sandbox filesystem operations
type Manager struct {
	sandboxRoot     string // Root directory for filesystem operations (server's view)
	sandboxHostPath string // Root directory on Docker host for bind mounts (may be same as sandboxRoot)
}

// NewManager creates a new sandbox manager
func NewManager(sandboxRoot, sandboxHostPath string) *Manager {
	return &Manager{
		sandboxRoot:     sandboxRoot,
		sandboxHostPath: sandboxHostPath,
	}
}

// EnsureSandboxDir ensures the sandbox directory exists for a conversation
// Creates the directory and sets ownership to 1000:1000 for runner containers
// Returns the absolute path to the conversation's sandbox directory
func (m *Manager) EnsureSandboxDir(conversationID string) (string, error) {
	if conversationID == "" {
		return "", fmt.Errorf("conversationID cannot be empty")
	}

	// Create conversation directory path
	sandboxDir := filepath.Join(m.sandboxRoot, conversationID)

	// Create directory with 0777 permissions
	if err := os.MkdirAll(sandboxDir, 0o777); err != nil {
		return "", fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	// Change ownership to 1000:1000 (sandbox user in containers)
	if err := os.Chown(sandboxDir, 1000, 1000); err != nil {
		// Log warning but don't fail - this might not work on all systems (e.g., Docker Desktop for Mac)
		// The directory will still be usable, just with different ownership
		fmt.Printf("Warning: failed to chown %s to 1000:1000: %v\n", sandboxDir, err)
	}

	// Try to set permissions via chmod as well
	if err := os.Chmod(sandboxDir, 0o777); err != nil {
		fmt.Printf("Warning: failed to chmod %s to 0777: %v\n", sandboxDir, err)
	}

	return sandboxDir, nil
}

// ListFiles lists all files in a conversation's sandbox directory
func (m *Manager) ListFiles(conversationID string) ([]string, error) {
	sandboxDir := filepath.Join(m.sandboxRoot, conversationID)

	// Check if directory exists
	if _, err := os.Stat(sandboxDir); os.IsNotExist(err) {
		return []string{}, nil
	}

	// Read directory contents
	entries, err := os.ReadDir(sandboxDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read sandbox directory: %w", err)
	}

	// Collect filenames (not directories)
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// GetFilePath returns the absolute path to a file in a conversation's sandbox
func (m *Manager) GetFilePath(conversationID, filename string) string {
	return filepath.Join(m.sandboxRoot, conversationID, filename)
}

// GetSandboxDir returns the absolute path to a conversation's sandbox directory
// This path is used for filesystem operations by the server
func (m *Manager) GetSandboxDir(conversationID string) string {
	return filepath.Join(m.sandboxRoot, conversationID)
}

// GetSandboxHostPath returns the absolute path on the Docker host for bind mounting
// This is used when creating runner containers - they need the host's perspective
func (m *Manager) GetSandboxHostPath(conversationID string) string {
	return filepath.Join(m.sandboxHostPath, conversationID)
}

// DeleteSandbox removes a conversation's sandbox directory and all its contents
func (m *Manager) DeleteSandbox(conversationID string) error {
	sandboxDir := filepath.Join(m.sandboxRoot, conversationID)
	return os.RemoveAll(sandboxDir)
}

// GetSandboxRoot returns the root directory for all sandboxes
func (m *Manager) GetSandboxRoot() string {
	return m.sandboxRoot
}

// chownRecursive changes ownership of a directory and all its contents
func chownRecursive(path string, uid, gid int) error {
	return filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Chown(name, uid, gid)
	})
}

// setStickyBit sets the sticky bit on a directory for better multi-user handling
func setStickyBit(path string) error {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return err
	}
	return syscall.Chmod(path, uint32(stat.Mode)|syscall.S_ISVTX)
}
