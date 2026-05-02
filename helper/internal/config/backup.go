package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const backupTimeFormat = "20060102-150405"

// Backup creates a timestamped copy of configPath in backupDir.
// Returns the path to the created backup file.
func Backup(configPath string, backupDir string) (string, error) {
	// Read original config
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read config: %w", err)
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	// Create timestamped backup filename
	timestamp := time.Now().Format(backupTimeFormat)
	baseName := filepath.Base(configPath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)
	backupName := fmt.Sprintf("%s-%s%s", nameWithoutExt, timestamp, ext)
	backupPath := filepath.Join(backupDir, backupName)

	// Write backup
	if err := os.WriteFile(backupPath, content, 0o644); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}

	return backupPath, nil
}

// CleanupOldBackups removes oldest backups in backupDir, keeping only the specified count.
// Only files matching the timestamped backup pattern are affected.
func CleanupOldBackups(backupDir string, keep int) error {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read backup dir: %w", err)
	}

	// Collect backup files
	var backups []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && isBackupFile(e.Name()) {
			backups = append(backups, e)
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(backups, func(i, j int) bool {
		infoI, _ := backups[i].Info()
		infoJ, _ := backups[j].Info()
		return infoI.ModTime().After(infoJ.ModTime())
	})

	// Remove excess backups
	if len(backups) > keep {
		for _, b := range backups[keep:] {
			path := filepath.Join(backupDir, b.Name())
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("remove old backup %s: %w", b.Name(), err)
			}
		}
	}

	return nil
}

// backupFilePattern matches timestamped backup files like "config-20060102-150405.yaml"
// Pattern: any name, hyphen, 8 digits (YYYYMMDD), hyphen, 6 digits (HHMMSS), .yaml extension
var backupFilePattern = regexp.MustCompile(`^.+-\d{8}-\d{6}\.yaml$`)

func isBackupFile(name string) bool {
	return backupFilePattern.MatchString(name)
}
