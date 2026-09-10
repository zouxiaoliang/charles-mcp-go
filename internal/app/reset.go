package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Reset actions are explicit; a read or ordinary shutdown never restores files.
func (a *App) Reset(ctx context.Context, p Args) (any, error) {
	for _, s := range a.Live.Status() {
		if s.Active {
			return nil, fmt.Errorf("stop live sessions before changing Charles environment")
		}
	}
	var err error
	switch p.Action {
	case "clear_session":
		_, err = a.Charles.Get(ctx, "/session/clear")
	case "quit_charles":
		_, err = a.Charles.Get(ctx, "/quit")
		if err != nil && a.Charles.EnsureStopped(ctx) == nil {
			err = nil
		}
	case "start_recording":
		err = a.Charles.Record(ctx, true)
	case "stop_recording":
		err = a.Charles.Record(ctx, false)
	case "backup_config", "restore_config":
		if p.ConfigPath == "" || p.BackupPath == "" {
			return nil, fmt.Errorf("config_path and backup_path are required")
		}
		if p.Action == "backup_config" {
			if err = copyFile(p.ConfigPath, filepath.Join(p.BackupPath, "charles.config")); err == nil && p.ProfilesPath != "" {
				err = copyTree(p.ProfilesPath, filepath.Join(p.BackupPath, "profiles"))
			}
		} else {
			if _, statErr := os.Stat(filepath.Join(p.BackupPath, "charles.config")); statErr != nil {
				return nil, statErr
			}
			if statusErr := a.Charles.EnsureStopped(ctx); statusErr != nil {
				return nil, statusErr
			}
			if p.ProfilesPath != "" {
				info, err := os.Stat(filepath.Join(p.BackupPath, "profiles"))
				if err != nil {
					return nil, err
				}
				if !info.IsDir() {
					return nil, fmt.Errorf("backup profiles path is not a directory")
				}
			}
			if err = copyFile(filepath.Join(p.BackupPath, "charles.config"), p.ConfigPath); err == nil && p.ProfilesPath != "" {
				err = copyTree(filepath.Join(p.BackupPath, "profiles"), p.ProfilesPath)
			}
		}
	default:
		return nil, fmt.Errorf("action required: clear_session, quit_charles, start_recording, stop_recording, backup_config or restore_config")
	}
	return map[string]any{"action": p.Action, "success": err == nil}, err
}
func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(dst), ".charles-config-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(name, dst)
}
func copyTree(src, dst string) error {
	source, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(dst)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(source, target)
	if err != nil {
		return err
	}
	if rel == "." || filepath.IsLocal(rel) {
		return fmt.Errorf("backup target must be outside source directory")
	}
	reverse, err := filepath.Rel(target, source)
	if err != nil {
		return err
	}
	if filepath.IsLocal(reverse) {
		return fmt.Errorf("source must be outside target directory")
	}
	if target == filepath.Dir(target) {
		return fmt.Errorf("cannot replace filesystem root")
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(target), ".charles-profiles-*")
	if err != nil {
		return err
	}
	keepStage := false
	defer func() {
		if !keepStage {
			os.RemoveAll(stage)
		}
	}()
	newDir := filepath.Join(stage, "new")
	err = filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("profile symlinks are not supported: %s", path)
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		out := filepath.Join(newDir, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0700)
		}
		return copyFile(path, out)
	})
	if err != nil {
		return err
	}
	oldDir := filepath.Join(stage, "old")
	exists := false
	if _, err = os.Lstat(target); err == nil {
		exists = true
		if err = os.Rename(target, oldDir); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = os.Rename(newDir, target); err != nil {
		if exists {
			if rollback := os.Rename(oldDir, target); rollback != nil {
				keepStage = true
				return fmt.Errorf("replace profiles: %v; rollback: %w (backup at %s)", err, rollback, oldDir)
			}
		}
		return err
	}
	return nil
}
