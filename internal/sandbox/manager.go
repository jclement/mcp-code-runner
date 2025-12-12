package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Manager handles sandbox filesystem operations
type Manager struct {
	sandboxRoot     string // Root directory for filesystem operations (server's view)
	sandboxHostPath string // Root directory on Docker host for bind mounts (may be same as sandboxRoot)
	secret          string // Secret for hashing conversation IDs
}

// NewManager creates a new sandbox manager
func NewManager(sandboxRoot, sandboxHostPath, secret string) *Manager {
	return &Manager{
		sandboxRoot:     sandboxRoot,
		sandboxHostPath: sandboxHostPath,
		secret:          secret,
	}
}

// hashConversationID creates a filesystem-safe hash of conversationID + secret
func (m *Manager) hashConversationID(conversationID string) string {
	h := sha256.New()
	h.Write([]byte(conversationID))
	h.Write([]byte(m.secret))
	return hex.EncodeToString(h.Sum(nil))
}

// EnsureSandboxDir ensures the sandbox directory exists for a conversation
// Creates the directory and sets ownership to 1000:1000 for runner containers
// Returns the hashed directory name (not full path)
func (m *Manager) EnsureSandboxDir(conversationID string) (string, error) {
	if conversationID == "" {
		return "", fmt.Errorf("conversationID cannot be empty")
	}

	// Hash the conversation ID to create filesystem-safe directory name
	hashedDir := m.hashConversationID(conversationID)
	sandboxDir := filepath.Join(m.sandboxRoot, hashedDir)

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

	return hashedDir, nil
}

// ListFiles lists all files in a conversation's sandbox directory
func (m *Manager) ListFiles(conversationID string) ([]string, error) {
	hashedDir := m.hashConversationID(conversationID)
	sandboxDir := filepath.Join(m.sandboxRoot, hashedDir)

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
func (m *Manager) GetFilePath(hashedDir, filename string) string {
	return filepath.Join(m.sandboxRoot, hashedDir, filename)
}

// GetSandboxDir returns the absolute path to a conversation's sandbox directory
// This path is used for filesystem operations by the server
func (m *Manager) GetSandboxDir(conversationID string) string {
	hashedDir := m.hashConversationID(conversationID)
	return filepath.Join(m.sandboxRoot, hashedDir)
}

// GetSandboxHostPath returns the absolute path on the Docker host for bind mounting
// This is used when creating runner containers - they need the host's perspective
func (m *Manager) GetSandboxHostPath(conversationID string) string {
	hashedDir := m.hashConversationID(conversationID)
	return filepath.Join(m.sandboxHostPath, hashedDir)
}

// DeleteSandbox removes a conversation's sandbox directory and all its contents
func (m *Manager) DeleteSandbox(conversationID string) error {
	hashedDir := m.hashConversationID(conversationID)
	sandboxDir := filepath.Join(m.sandboxRoot, hashedDir)
	return os.RemoveAll(sandboxDir)
}

// GetSandboxRoot returns the root directory for all sandboxes
func (m *Manager) GetSandboxRoot() string {
	return m.sandboxRoot
}

// WriteFile writes content to a file in a conversation's sandbox
// Creates the sandbox directory if it doesn't exist
func (m *Manager) WriteFile(conversationID, filename string, content []byte) error {
	hashedDir := m.hashConversationID(conversationID)
	sandboxDir := filepath.Join(m.sandboxRoot, hashedDir)

	// Ensure sandbox directory exists
	if err := os.MkdirAll(sandboxDir, 0o777); err != nil {
		return fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	// Change ownership to 1000:1000
	if err := os.Chown(sandboxDir, 1000, 1000); err != nil {
		fmt.Printf("Warning: failed to chown %s to 1000:1000: %v\n", sandboxDir, err)
	}

	// Write file
	filePath := filepath.Join(sandboxDir, filename)
	if err := os.WriteFile(filePath, content, 0o666); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Change file ownership to 1000:1000
	if err := os.Chown(filePath, 1000, 1000); err != nil {
		fmt.Printf("Warning: failed to chown %s to 1000:1000: %v\n", filePath, err)
	}

	return nil
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
