package utils

import (
	"io"
	"os"
)

func FileCopy(src, dst string, bufSize int) error {
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
