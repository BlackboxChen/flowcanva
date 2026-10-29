package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var backupMutex sync.Mutex

// SQLite's online snapshot includes committed WAL data. Immutable media are copied
// once into the backup store; each snapshot records exactly which files it needs.
func (h *Handler) createBackup() (string, error) {
	backupMutex.Lock()
	defer backupMutex.Unlock()
	root := filepath.Join("backups", time.Now().Format("20060102-150405.000000000"))
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	dbPath, err := filepath.Abs(filepath.Join(root, "data.db"))
	if err != nil {
		return "", err
	}
	if _, err = h.db.Exec(`VACUUM INTO ?`, dbPath); err != nil {
		return "", err
	}
	manifest := map[string]string{}
	err = filepath.WalkDir("uploads", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("备份不跟随链接: %s", path)
		}
		relative, err := filepath.Rel("uploads", path)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		hash := sha256.New()
		if _, err = io.Copy(hash, src); err != nil {
			return err
		}
		digest := hex.EncodeToString(hash.Sum(nil))
		if _, err = src.Seek(0, 0); err != nil {
			return err
		}
		target := filepath.Join("backups", "media", digest)
		if _, err = os.Stat(target); os.IsNotExist(err) {
			if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			dst, err := os.OpenFile(target+".partial", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(dst, src)
			closeErr := dst.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if err = os.Rename(target+".partial", target); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		manifest[relative] = digest
		return nil
	})
	if err != nil {
		return "", err
	}
	body, _ := json.MarshalIndent(manifest, "", "  ")
	if err = os.WriteFile(filepath.Join(root, "media.json"), body, 0600); err != nil {
		return "", err
	}
	h.log.Info("collaboration backup completed", "path", root, "media_files", len(manifest))
	return root, nil
}
func (h *Handler) startBackups() {
	go func() {
		// Startup delay avoids competing with task recovery.
		time.Sleep(time.Minute)
		for {
			if _, err := h.createBackup(); err != nil {
				h.log.Error("collaboration backup failed", "error", err)
			}
			time.Sleep(24 * time.Hour)
		}
	}()
}
func (h *Handler) BackupNow(c *gin.Context) {
	path, err := h.createBackup()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"path": path})
}
