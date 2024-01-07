package writer

import (
	"io"
	"os"
	"path"
)

func New(f string) io.Writer {
	return fileWriter{filePath: f}
}

type fileWriter struct {
	filePath string
}

func (f fileWriter) Write(p []byte) (n int, err error) {
	dir, _ := path.Split(f.filePath)

	if dir != "" {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return 0, err
		}
	}

	fs, err := os.Create(f.filePath)
	if err != nil {
		return 0, err
	}
	defer fs.Close()

	return fs.Write(p)
}
