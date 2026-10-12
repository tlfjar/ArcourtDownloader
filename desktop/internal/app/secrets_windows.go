//go:build windows

package app

import (
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credentialTypeGeneric         = 1
	credentialPersistLocalMachine = 2 // Current user's credential set, retained on this computer.
	credentialTargetPrefix        = "ArcourtDownloader/AI/"
)

var (
	credentialDLL    = windows.NewLazySystemDLL("advapi32.dll")
	credentialRead   = credentialDLL.NewProc("CredReadW")
	credentialWrite  = credentialDLL.NewProc("CredWriteW")
	credentialDelete = credentialDLL.NewProc("CredDeleteW")
	credentialFree   = credentialDLL.NewProc("CredFree")
)

// Mirrors CREDENTIALW in wincred.h. The last-written FILETIME is two DWORDs.
type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type credentialManager struct{ prefix string }

func DefaultSecretStore() SecretStore { return credentialManager{prefix: credentialTargetPrefix} }

func (s credentialManager) target(provider string) (*uint16, error) {
	if namingRecipient(provider) == "" {
		return nil, errors.New("unsupported naming provider")
	}
	return windows.UTF16PtrFromString(s.prefix + provider)
}

func (s credentialManager) Status(provider string) (bool, error) {
	target, err := s.target(provider)
	if err != nil {
		return false, err
	}
	var value *windowsCredential
	ok, _, callErr := credentialRead.Call(uintptr(unsafe.Pointer(target)), credentialTypeGeneric, 0, uintptr(unsafe.Pointer(&value)))
	runtime.KeepAlive(target)
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return false, nil
		}
		return false, errCredentialUnavailable
	}
	defer credentialFree.Call(uintptr(unsafe.Pointer(value)))
	return value != nil && value.CredentialBlobSize > 0 && value.CredentialBlobSize <= 2048 && value.CredentialBlob != nil, nil
}

func (s credentialManager) Read(provider string) (string, error) {
	target, err := s.target(provider)
	if err != nil {
		return "", err
	}
	var value *windowsCredential
	ok, _, callErr := credentialRead.Call(uintptr(unsafe.Pointer(target)), credentialTypeGeneric, 0, uintptr(unsafe.Pointer(&value)))
	runtime.KeepAlive(target)
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_NOT_FOUND) {
			return "", ErrCredentialMissing
		}
		return "", errCredentialUnavailable
	}
	defer credentialFree.Call(uintptr(unsafe.Pointer(value)))
	if value == nil || value.CredentialBlobSize == 0 || value.CredentialBlobSize > 2048 || value.CredentialBlob == nil {
		return "", errCredentialUnavailable
	}
	bytes := unsafe.Slice(value.CredentialBlob, int(value.CredentialBlobSize))
	key := string(bytes)
	validated, err := validateCredential(provider, key)
	if err != nil {
		return "", errCredentialUnavailable
	}
	return validated, nil
}

func (s credentialManager) Write(provider, key string) error {
	var err error
	key, err = validateCredential(provider, key)
	if err != nil {
		return err
	}
	target, err := s.target(provider)
	if err != nil {
		return err
	}
	username, err := windows.UTF16PtrFromString("ArcourtDownloader")
	if err != nil {
		return err
	}
	blob := []byte(key)
	defer clear(blob)
	value := windowsCredential{Type: credentialTypeGeneric, TargetName: target, CredentialBlobSize: uint32(len(blob)), CredentialBlob: &blob[0], Persist: credentialPersistLocalMachine, UserName: username}
	ok, _, _ := credentialWrite.Call(uintptr(unsafe.Pointer(&value)), 0)
	runtime.KeepAlive(value)
	runtime.KeepAlive(blob)
	if ok == 0 {
		return errCredentialUnavailable
	}
	return nil
}

func (s credentialManager) Delete(provider string) error {
	target, err := s.target(provider)
	if err != nil {
		return err
	}
	ok, _, callErr := credentialDelete.Call(uintptr(unsafe.Pointer(target)), credentialTypeGeneric, 0)
	runtime.KeepAlive(target)
	if ok == 0 && !errors.Is(callErr, windows.ERROR_NOT_FOUND) {
		return errCredentialUnavailable
	}
	return nil
}
