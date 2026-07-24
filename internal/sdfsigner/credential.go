package sdfsigner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

// FileCredentialSource reads an owner-controlled regular file for each
// private-key or KEK operation. Neither its path nor content enters errors.
type FileCredentialSource struct {
	path string
}

func NewFileCredentialSource(path string) (*FileCredentialSource, error) {
	if path == "" {
		return nil, fmt.Errorf("%w: SDF credential file is required", ErrInvalidConfiguration)
	}
	return &FileCredentialSource{path: path}, nil
}

func (s *FileCredentialSource) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.path == "" {
		return nil, newFault(faultAuthentication)
	}
	before, err := os.Lstat(s.path)
	if err != nil || !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 ||
		!credentialFilePermissionsSafe(before) {
		return nil, newFault(faultAuthentication)
	}
	file, err := os.Open(s.path)
	if err != nil {
		return nil, newFault(faultAuthentication)
	}
	defer file.Close()
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) {
		return nil, newFault(faultAuthentication)
	}
	credential, err := io.ReadAll(io.LimitReader(file, MaxCredentialBytes+1))
	if err != nil || len(credential) == 0 || len(credential) > MaxCredentialBytes {
		clear(credential)
		return nil, newFault(faultAuthentication)
	}
	credential = bytes.TrimSuffix(credential, []byte("\n"))
	credential = bytes.TrimSuffix(credential, []byte("\r"))
	if len(credential) == 0 || bytes.IndexByte(credential, 0) >= 0 {
		clear(credential)
		return nil, newFault(faultAuthentication)
	}
	if err := ctx.Err(); err != nil {
		clear(credential)
		return nil, err
	}
	out := append([]byte(nil), credential...)
	clear(credential)
	return out, nil
}

type staticCredentialSource []byte

func (s staticCredentialSource) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(s) == 0 {
		return nil, errors.New("test credential is empty")
	}
	return append([]byte(nil), s...), nil
}
