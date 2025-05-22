package utils

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Compress compresses a file or directory into a .tar.gz archive.
func Compress(sourcePath, outputFile string) error {
	dir := filepath.Dir(outputFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("error creating directories: %w", err)
	}

	outFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}
	defer outFile.Close()

	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Resolve symlink if sourcePath is a symlink
	realPath, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlink: %w", err)
	}

	info, err := os.Stat(realPath)
	if err != nil {
		return fmt.Errorf("failed to stat resolved path: %w", err)
	}

	baseDir := filepath.Dir(sourcePath) // use logical path for naming
	if info.IsDir() {
		sourcePath = realPath // walk the resolved real dir
		baseDir = filepath.Clean(sourcePath)
	}

	// Walk through all files in the directory
	err = filepath.Walk(sourcePath, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return addFileToTar(tarWriter, file, fi, baseDir)
	})

	return err
}

// addFileToTar adds a file or directory to the tar archive
func addFileToTar(tw *tar.Writer, filePath string, fi os.FileInfo, baseDir string) error {
	relPath, err := filepath.Rel(baseDir, filePath)
	if err != nil {
		return fmt.Errorf("failed to compute relative path: %w", err)
	}

	// Handle symlinks by resolving to the real file
	if fi.Mode()&os.ModeSymlink != 0 {
		// Resolve symlink
		resolvedPath, err := filepath.EvalSymlinks(filePath)
		if err != nil {
			return fmt.Errorf("failed to resolve symlink: %w", err)
		}

		// Stat the resolved target
		resolvedFi, err := os.Stat(resolvedPath)
		if err != nil {
			return fmt.Errorf("failed to stat resolved path: %w", err)
		}

		// Create header for the resolved file (or directory), but use symlink name
		header, err := tar.FileInfoHeader(resolvedFi, "")
		if err != nil {
			return fmt.Errorf("failed to create header: %w", err)
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}

		// If the symlink points to a directory, don't try to open it
		if resolvedFi.IsDir() {
			return nil
		}

		// Otherwise, open and copy file contents
		srcFile, err := os.Open(resolvedPath)
		if err != nil {
			return fmt.Errorf("failed to open resolved file: %w", err)
		}
		defer srcFile.Close()

		buf := make([]byte, 32*1024)
		_, err = io.CopyBuffer(tw, srcFile, buf)
		return err
	}

	// Handle directories (to preserve empty folders)
	if fi.IsDir() {
		header, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return fmt.Errorf("failed to create header for dir: %w", err)
		}
		header.Name = relPath + "/"
		if err := tw.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write dir header: %w", err)
		}
		return nil
	}

	// Skip non-regular files
	if !fi.Mode().IsRegular() {
		return nil
	}

	// Handle regular files
	header, err := tar.FileInfoHeader(fi, "")
	if err != nil {
		return fmt.Errorf("failed to create tar header: %w", err)
	}
	header.Name = relPath

	if err := tw.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	srcFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	buf := make([]byte, 32*1024)
	_, err = io.CopyBuffer(tw, srcFile, buf)
	return err
}

// Uncompress extracts a .tar.gz archive to a target directory
func Uncompress(archivePath, targetDir string) error {
	inFile, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer inFile.Close()

	gzipReader, err := gzip.NewReader(inFile)
	if err != nil {
		return fmt.Errorf("failed to open gzip stream: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar: %w", err)
		}

		targetPath := filepath.Join(targetDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			// Ensure the directory exists
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg, tar.TypeSymlink:

			// Ensure parent directories exist
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent directories: %w", err)
			}

			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer outFile.Close()

			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
		}
	}

	return nil
}
