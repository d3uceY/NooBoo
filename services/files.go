package services

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// openInExplorer reveals a folder in the OS file manager.
func openInExplorer(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// countTree totals files and bytes below dir. Missing dir counts as zero.
func countTree(dir string) (files int, size int64) {
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // ponytail: unreadable file is skipped, not fatal
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				files++
				size += info.Size()
			}
		}
		return nil
	})
	return files, size
}

// copyTree copies dir into dst, reporting after each file.
func copyTree(src, dst string, onFile func(string)) (int, int64, error) {
	var files int
	var size int64
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := copyFile(p, target); err != nil {
			return err
		}
		if info, err := d.Info(); err == nil {
			size += info.Size()
		}
		files++
		if onFile != nil {
			onFile(p)
		}
		return nil
	})
	return files, size, err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// treeInfo totals a folder and keeps a sample of file names plus the newest mtime.
// A loose file is its own tree, so its name comes back as the name, not as ".".
func treeInfo(dir string, maxNames int) (files int, size int64, names []string, newest time.Time) {
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		files++
		size += info.Size()
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		if len(names) < maxNames {
			if p == dir {
				names = append(names, filepath.Base(p))
			} else if rel, err := filepath.Rel(dir, p); err == nil {
				names = append(names, filepath.ToSlash(rel))
			}
		}
		return nil
	})
	return files, size, names, newest
}

// folderSafe strips characters Windows refuses in a folder name.
func folderSafe(name string) string {
	name = strings.TrimSpace(name)
	replacer := strings.NewReplacer(
		`\`, "-", "/", "-", ":", "-", "*", "", "?", "",
		`"`, "", "<", "", ">", "", "|", "-",
	)
	name = replacer.Replace(name)
	name = strings.TrimRight(name, ". ")
	if name == "" {
		name = "Unnamed"
	}
	return name
}

// uniqueDir returns base, or base-2, base-3 and so on when the name is taken.
// Backups made in the same second would otherwise overwrite each other.
func uniqueDir(base string) string {
	if !fileExists(base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !fileExists(candidate) {
			return candidate
		}
	}
}
