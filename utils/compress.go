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
	if err := os.MkdirAll(dir, 0777); err != nil {
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

	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	baseDir := filepath.Dir(sourcePath)
	if info.IsDir() {
		baseDir = filepath.Clean(sourcePath)
		sourcePath = filepath.Clean(sourcePath) // Ensure sourcePath is properly set
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
	var link string
	if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
		var err error
		if link, err = os.Readlink(filePath); err != nil {
			return err
		}
	}

	header, err := tar.FileInfoHeader(fi, link)
	if err != nil {
		return err
	}

	relPath, err := filepath.Rel(filepath.Dir(baseDir), filePath)
	if err != nil {
		return err
	}
	header.Name = relPath

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	if !fi.Mode().IsRegular() { // Skip non-regular files (directories, symlinks, etc.)
		return nil
	}

	srcFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Create a buffer for io.CopyBuffer()
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
			if err := os.MkdirAll(targetPath, 0777); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg, tar.TypeSymlink:

			// Ensure parent directories exist
			if err := os.MkdirAll(filepath.Dir(targetPath), 0777); err != nil {
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
