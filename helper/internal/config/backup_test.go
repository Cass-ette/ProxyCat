package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupCreatesTimestampedCopy(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.yaml")
	backupDir := filepath.Join(temp, "backups")

	original := []byte("original config")
	if err := os.WriteFile(configPath, original, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	backupPath, err := Backup(configPath, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Verify backup file exists and has content
	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupContent) != string(original) {
		t.Fatalf("backup content mismatch")
	}
}

func TestBackupCreatesBackupDir(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.yaml")
	backupDir := filepath.Join(temp, "new-backups")

	if err := os.WriteFile(configPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Backup(configPath, backupDir)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}

	// Verify backup directory was created
	info, err := os.Stat(backupDir)
	if err != nil {
		t.Fatalf("backup dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("backup path is not a directory")
	}
}

func TestBackupNonexistentConfig(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "nonexistent.yaml")
	backupDir := filepath.Join(temp, "backups")

	_, err := Backup(configPath, backupDir)
	if err == nil {
		t.Fatal("expected error for nonexistent config")
	}
}

func TestCleanupOldBackups(t *testing.T) {
	temp := t.TempDir()
	backupDir := filepath.Join(temp, "backups")

	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("create backup dir: %v", err)
	}

	// Create 5 fake backup files
	for i := 0; i < 5; i++ {
		name := filepath.Join(backupDir, formatBackupName(i))
		if err := os.WriteFile(name, []byte("backup"), 0o644); err != nil {
			t.Fatalf("write backup: %v", err)
		}
	}

	// Cleanup to keep only 3
	if err := CleanupOldBackups(backupDir, 3); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	// Count remaining backups
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}

	count := 0
	for _, e := range entries {
		if !e.IsDir() && isBackupFile(e.Name()) {
			count++
		}
	}

	if count != 3 {
		t.Fatalf("backup count = %d, want 3", count)
	}
}

func formatBackupName(index int) string {
	// Create names that look like timestamped backups (e.g., "backup-a.yaml")
	// Must contain "-" and end with ".yaml" to match isBackupFile logic
	return "backup-" + string(rune('a'+index)) + ".yaml"
}
