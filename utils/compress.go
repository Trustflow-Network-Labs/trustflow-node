package utils

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// compress compresses a file or directory into a .tar.gz archive.
func Compress(sourcePath, outputFile string) error {
	// Ensure the directory exists
	dir := filepath.Dir(outputFile)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		fmt.Println("Error creating directories:", err)
		return err
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

	// If it's a file, compress just that one
	if !info.IsDir() {
		return addFileToTar(tarWriter, sourcePath, info)
	}

	// Otherwise, compress the whole directory
	err = filepath.Walk(sourcePath, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil // Skip directories
		}
		return addFileToTar(tarWriter, file, fi)
	})

	return err
}

// addFileToTar adds a file to the tar archive
func addFileToTar(tw *tar.Writer, filePath string, fi os.FileInfo) error {
	srcFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	header, err := tar.FileInfoHeader(fi, filePath)
	if err != nil {
		return err
	}
	header.Name, _ = filepath.Rel(filepath.Dir(filePath), filePath)

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, srcFile)
	return err
}

// uncompress extracts a .tar.gz archive to a target directory
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
			break // End of archive
		}
		if err != nil {
			return fmt.Errorf("error reading tar: %w", err)
		}

		// Determine file/folder path
		targetPath := filepath.Join(targetDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory if needed
			if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		case tar.TypeReg:
			// Extract file
			outFile, err := os.Create(targetPath)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer outFile.Close()

			// Stream file contents
			if _, err := io.Copy(outFile, tarReader); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
		}
	}

	return nil
}
