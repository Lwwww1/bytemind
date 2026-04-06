//go:build windows

package secretstore

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	dpapiScheme               = "windows-dpapi"
	cryptProtectUIForbidden   = 0x1
	cryptUnprotectUIForbidden = 0x1
)

var (
	modCrypt32             = windows.NewLazySystemDLL("Crypt32.dll")
	modKernel32            = windows.NewLazySystemDLL("Kernel32.dll")
	procCryptProtectData   = modCrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = modCrypt32.NewProc("CryptUnprotectData")
	procLocalFree          = modKernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func platformProtect(plain []byte) (string, []byte, error) {
	if len(plain) == 0 {
		return dpapiScheme, []byte{}, nil
	}

	in := dataBlob{
		cbData: uint32(len(plain)),
		pbData: &plain[0],
	}
	var out dataBlob

	ok, _, callErr := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		uintptr(cryptProtectUIForbidden),
		uintptr(unsafe.Pointer(&out)),
	)
	if ok == 0 {
		return "", nil, formatDPAPIError("CryptProtectData", callErr)
	}
	defer localFree(uintptr(unsafe.Pointer(out.pbData)))

	return dpapiScheme, copyBlob(out), nil
}

func platformUnprotect(scheme string, cipher []byte) ([]byte, error) {
	if scheme == "" || scheme == plainScheme {
		out := make([]byte, len(cipher))
		copy(out, cipher)
		return out, nil
	}
	if scheme != dpapiScheme {
		return nil, fmt.Errorf("unsupported secret scheme %q", scheme)
	}
	if len(cipher) == 0 {
		return []byte{}, nil
	}

	in := dataBlob{
		cbData: uint32(len(cipher)),
		pbData: &cipher[0],
	}
	var out dataBlob
	var description *uint16

	ok, _, callErr := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&in)),
		uintptr(unsafe.Pointer(&description)),
		0,
		0,
		0,
		uintptr(cryptUnprotectUIForbidden),
		uintptr(unsafe.Pointer(&out)),
	)
	if ok == 0 {
		return nil, formatDPAPIError("CryptUnprotectData", callErr)
	}
	defer localFree(uintptr(unsafe.Pointer(out.pbData)))
	if description != nil {
		defer localFree(uintptr(unsafe.Pointer(description)))
	}

	return copyBlob(out), nil
}

func formatDPAPIError(name string, callErr error) error {
	if callErr == nil {
		return fmt.Errorf("%s failed", name)
	}
	if errno, ok := callErr.(windows.Errno); ok && errno == windows.ERROR_SUCCESS {
		return fmt.Errorf("%s failed", name)
	}
	return fmt.Errorf("%s failed: %w", name, callErr)
}

func localFree(ptr uintptr) {
	if ptr == 0 {
		return
	}
	_, _, _ = procLocalFree.Call(ptr)
}

func copyBlob(blob dataBlob) []byte {
	if blob.cbData == 0 || blob.pbData == nil {
		return []byte{}
	}
	src := unsafe.Slice(blob.pbData, blob.cbData)
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
