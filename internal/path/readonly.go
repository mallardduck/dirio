package path

import (
	"errors"
	"os"

	"github.com/go-git/go-billy/v5"
)

type ReadOnlyFS struct {
	billy.Filesystem
}

func (r ReadOnlyFS) Create(filename string) (billy.File, error) {
	return nil, errors.New("filesystem is read-only")
}

func (r ReadOnlyFS) OpenFile(name string, flag int, perm os.FileMode) (billy.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return nil, errors.New("filesystem is read-only")
	}
	return r.Filesystem.OpenFile(name, flag, perm)
}

func (r ReadOnlyFS) Remove(filename string) error {
	return errors.New("filesystem is read-only")
}

func (r ReadOnlyFS) Rename(oldpath, newpath string) error {
	return errors.New("filesystem is read-only")
}

func (r ReadOnlyFS) MkdirAll(filename string, perm os.FileMode) error {
	return errors.New("filesystem is read-only")
}
