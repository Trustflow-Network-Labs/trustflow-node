package utils

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Copy files in chunks
func BufferFileCopy(src, dst string, bufSize int) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	buf := make([]byte, bufSize)
	_, err = io.CopyBuffer(destination, source, buf)
	return err
}

// CopyPath copies a file or directory from src to dst
func CopyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source: %w", err)
	}

	if info.IsDir() {
		return copyDir(src, dst)
	}
	return copyFile(src, dst)
}

func copyFile(srcFile, dstFile string) error {
	src, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(dstFile), 0755); err != nil {
		return fmt.Errorf("mkdir dst dir: %w", err)
	}

	dst, err := os.Create(dstFile)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy contents: %w", err)
	}

	// Copy file mode
	info, err := os.Stat(srcFile)
	if err == nil {
		_ = os.Chmod(dstFile, info.Mode())
	}
	return nil
}

func copyDir(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dstDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}
		return copyFile(path, dstPath)
	})
}
