package wal

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const namespaceLockFileName = ".trustdb-wal.lock"

type namespaceLock struct {
	mu            sync.Mutex
	access        *sync.RWMutex
	path          string
	dir           *os.File
	file          *os.File
	borrowed      *namespaceLock
	writeBorrow   bool
	exclusive     bool
	activeSegment func() uint64
	closed        bool
}

var namespaceLocks = struct {
	sync.Mutex
	owners map[string]*namespaceLock
}{owners: make(map[string]*namespaceLock)}

func acquireDirectoryNamespaceLock(dir string, exclusive bool) (*namespaceLock, error) {
	return acquireNamespaceLock(dir, filepath.Join(dir, namespaceLockFileName), exclusive, false)
}

func acquireDirectoryMaintenanceLock(dir string) (*namespaceLock, error) {
	return acquireNamespaceLock(dir, filepath.Join(dir, namespaceLockFileName), true, true)
}

func acquireSingleFileNamespaceLock(path string, exclusive bool) (*namespaceLock, error) {
	dir := filepath.Dir(path)
	return acquireNamespaceLock(dir, path+".lock", exclusive, false)
}

func acquireSingleFileMaintenanceLock(path string) (*namespaceLock, error) {
	dir := filepath.Dir(path)
	return acquireNamespaceLock(dir, path+".lock", true, true)
}

func acquireNamespaceLock(root, path string, exclusive, maintenance bool) (*namespaceLock, error) {
	absolutePath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, fmtError("resolve namespace lock path", err)
	}
	namespaceLocks.Lock()
	defer namespaceLocks.Unlock()
	if owner := namespaceLocks.owners[absolutePath]; owner != nil {
		if exclusive {
			if !maintenance || !owner.exclusive {
				return nil, errors.New("wal: namespace is already open by an incompatible owner in this process")
			}
			owner.access.Lock()
			return &namespaceLock{borrowed: owner, writeBorrow: true}, nil
		}
		owner.access.RLock()
		return &namespaceLock{borrowed: owner}, nil
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmtError("create WAL namespace directory", err)
	}
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, fmtError("inspect WAL namespace directory", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, errors.New("wal: namespace root must be a real directory, not a symlink")
	}
	if !validateNamespaceRootPermissions(rootInfo) {
		return nil, errors.New("wal: namespace root must not be group- or other-writable")
	}
	dir, err := os.Open(root)
	if err != nil {
		return nil, fmtError("open WAL namespace directory", err)
	}
	dirInfo, err := dir.Stat()
	if err != nil {
		_ = dir.Close()
		return nil, fmtError("stat opened WAL namespace directory", err)
	}
	if !os.SameFile(rootInfo, dirInfo) {
		_ = dir.Close()
		return nil, errors.New("wal: namespace directory changed while opening")
	}
	file, err := openSecureNamespaceLockFile(path)
	if err != nil {
		_ = dir.Close()
		return nil, err
	}
	if err := lockNamespaceFile(file, exclusive); err != nil {
		_ = file.Close()
		_ = dir.Close()
		return nil, fmtError("namespace is already open by an incompatible process", err)
	}
	lock := &namespaceLock{
		access:    &sync.RWMutex{},
		path:      absolutePath,
		dir:       dir,
		file:      file,
		exclusive: exclusive,
	}
	namespaceLocks.owners[absolutePath] = lock
	return lock, nil
}

func openSecureNamespaceLockFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		file, createErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
		if createErr == nil {
			info, err = os.Lstat(path)
			if err != nil {
				_ = file.Close()
				return nil, fmtError("inspect created namespace lock", err)
			}
			fileInfo, statErr := file.Stat()
			if statErr != nil {
				_ = file.Close()
				return nil, fmtError("stat created namespace lock", statErr)
			}
			if err := validateNamespaceLockFile(info, fileInfo); err != nil {
				_ = file.Close()
				return nil, err
			}
			return file, nil
		}
		if !errors.Is(createErr, os.ErrExist) {
			return nil, fmtError("create namespace lock", createErr)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return nil, fmtError("inspect namespace lock", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("wal: namespace lock must be a regular file, not a symlink")
	}
	if !validateNamespaceLockPermissions(info) {
		return nil, errors.New("wal: namespace lock permissions must be 0600")
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmtError("open namespace lock", err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmtError("stat opened namespace lock", err)
	}
	if err := validateNamespaceLockFile(info, fileInfo); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func validateNamespaceLockFile(pathInfo, fileInfo os.FileInfo) error {
	if pathInfo.Mode()&os.ModeSymlink != 0 || !pathInfo.Mode().IsRegular() || !fileInfo.Mode().IsRegular() {
		return errors.New("wal: namespace lock must be a regular file, not a symlink")
	}
	if !validateNamespaceLockPermissions(pathInfo) || !validateNamespaceLockPermissions(fileInfo) {
		return errors.New("wal: namespace lock permissions must be 0600")
	}
	if !os.SameFile(pathInfo, fileInfo) {
		return errors.New("wal: namespace lock changed while opening")
	}
	return nil
}

func (l *namespaceLock) close() error {
	if l == nil {
		return nil
	}
	if l.borrowed != nil {
		l.mu.Lock()
		if !l.closed {
			l.closed = true
			if l.writeBorrow {
				l.borrowed.access.Unlock()
			} else {
				l.borrowed.access.RUnlock()
			}
		}
		l.mu.Unlock()
		return nil
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	l.mu.Unlock()
	namespaceLocks.Lock()
	l.access.Lock()
	if namespaceLocks.owners[l.path] == l {
		delete(namespaceLocks.owners, l.path)
	}
	namespaceLocks.Unlock()
	defer l.access.Unlock()
	unlockErr := unlockNamespaceFile(l.file)
	fileErr := l.file.Close()
	dirErr := l.dir.Close()
	return errors.Join(
		wrapError("wal: unlock namespace", unlockErr),
		wrapError("wal: close namespace lock", fileErr),
		wrapError("wal: close namespace directory", dirErr),
	)
}

func (l *namespaceLock) beginWrite() {
	if l != nil && l.borrowed == nil {
		l.access.Lock()
	}
}

func (l *namespaceLock) endWrite() {
	if l != nil && l.borrowed == nil {
		l.access.Unlock()
	}
}

func (l *namespaceLock) setActiveSegment(get func() uint64) {
	if l != nil && l.borrowed == nil {
		l.activeSegment = get
	}
}

func (l *namespaceLock) writerActiveSegment() (uint64, bool) {
	if l == nil || l.borrowed == nil || !l.writeBorrow || l.borrowed.activeSegment == nil {
		return 0, false
	}
	return l.borrowed.activeSegment(), true
}

func fmtError(operation string, err error) error {
	return wrapError("wal: "+operation, err)
}

func wrapError(message string, err error) error {
	if err == nil {
		return nil
	}
	return errors.New(message + ": " + err.Error())
}
