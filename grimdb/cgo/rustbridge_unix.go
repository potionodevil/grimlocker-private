//go:build !windows

package rustbridge

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>

typedef uintptr_t (*rustbridge_fn)(uintptr_t, uintptr_t, uintptr_t, uintptr_t, uintptr_t,
                                   uintptr_t, uintptr_t, uintptr_t, uintptr_t, uintptr_t,
                                   uintptr_t, uintptr_t);

static uintptr_t bridge_call(rustbridge_fn fn,
                             uintptr_t a1, uintptr_t a2, uintptr_t a3, uintptr_t a4,
                             uintptr_t a5, uintptr_t a6, uintptr_t a7, uintptr_t a8,
                             uintptr_t a9, uintptr_t a10, uintptr_t a11, uintptr_t a12) {
	return fn(a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12);
}
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"unsafe"
)

var (
	so      unsafe.Pointer
	soAvail bool
	soOnce  sync.Once
	soErr   error
)

func init() {
	soOnce.Do(func() {
		exePath, _ := os.Executable()
		exeDir := ""
		if exePath != "" {
			exeDir = filepath.Dir(exePath)
		}
		paths := []string{
			"libgrimlocker_core.so",
		}
		if exeDir != "" {
			paths = append(paths, filepath.Join(exeDir, "libgrimlocker_core.so"))
		}
		for _, p := range paths {
			cPath := C.CString(p)
			handle := C.dlopen(cPath, C.RTLD_NOW|C.RTLD_LOCAL)
			C.free(unsafe.Pointer(cPath))
			if handle != nil {
				so = unsafe.Pointer(handle)
				soAvail = true
				log.Printf("[rustbridge] Loaded %s", p)
				return
			}
			errStr := C.dlerror()
			if errStr != nil {
				log.Printf("[rustbridge] dlopen(%s): %s", p, C.GoString(errStr))
			}
		}
		soErr = fmt.Errorf("libgrimlocker_core.so not found in PATH")
		log.Printf("[rustbridge] .so not found, using Go fallback: %v", soErr)
	})
}

func ensureDLL() error {
	if !soAvail {
		return fmt.Errorf("grimlocker_core .so not available: %w", soErr)
	}
	return nil
}

func callProc(procName string, args ...uintptr) (uintptr, error) {
	if err := ensureDLL(); err != nil {
		return 0, err
	}
	cName := C.CString(procName)
	defer C.free(unsafe.Pointer(cName))
	fn := C.dlsym(so, cName)
	if fn == nil {
		return 0, fmt.Errorf("dlsym %s: symbol not found", procName)
	}
	padded := make([]C.uintptr_t, 12)
	for i, a := range args {
		if i < 12 {
			padded[i] = C.uintptr_t(a)
		}
	}
	result := C.bridge_call(
		C.rustbridge_fn(fn),
		padded[0], padded[1], padded[2], padded[3],
		padded[4], padded[5], padded[6], padded[7],
		padded[8], padded[9], padded[10], padded[11],
	)
	return uintptr(result), nil
}

func uint8PtrToString(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}
	return C.GoString((*C.char)(unsafe.Pointer(ptr)))
}

func freeCString(ptr uintptr) {
	if ptr == 0 || !soAvail {
		return
	}
	cName := C.CString("free_cstring")
	defer C.free(unsafe.Pointer(cName))
	fn := C.dlsym(so, cName)
	if fn == nil {
		return
	}
	C.bridge_call(
		C.rustbridge_fn(fn),
		C.uintptr_t(ptr), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	)
}

func SecureZero(data []byte) {
	if len(data) == 0 {
		return
	}
	if !soAvail {
		for i := range data {
			data[i] = 0
		}
		return
	}
	cName := C.CString("secure_zero")
	defer C.free(unsafe.Pointer(cName))
	fn := C.dlsym(so, cName)
	if fn == nil {
		for i := range data {
			data[i] = 0
		}
		return
	}
	C.bridge_call(
		C.rustbridge_fn(fn),
		C.uintptr_t(uintptr(unsafe.Pointer(&data[0]))),
		C.uintptr_t(len(data)),
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	)
}

func InitCore() error {
	if !soAvail {
		return nil
	}
	r, err := callProc("grimcore_init")
	if err != nil {
		return err
	}
	msg := uint8PtrToString(r)
	freeCString(r)
	if msg != "OK" {
		return fmt.Errorf("grimcore_init: %s", msg)
	}
	return nil
}

func ShutdownCore() {
	if !soAvail {
		return
	}
	cName := C.CString("grimcore_shutdown")
	defer C.free(unsafe.Pointer(cName))
	fn := C.dlsym(so, cName)
	if fn == nil {
		return
	}
	C.bridge_call(
		C.rustbridge_fn(fn),
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	)
}

func MVKStore(mvk []byte) (string, error) {
	if len(mvk) != 32 {
		return "", fmt.Errorf("MVK must be 32 bytes")
	}
	if !soAvail {
		return fmt.Sprintf("mvk:%x", mvk[:8]), nil
	}
	r, err := callProc("grimcore_mvk_store",
		uintptr(unsafe.Pointer(&mvk[0])),
		uintptr(len(mvk)),
	)
	if err != nil {
		return "", err
	}
	handle := uint8PtrToString(r)
	freeCString(r)
	if len(handle) > 5 && handle[:5] == "ERROR" {
		return "", fmt.Errorf("mvk_store: %s", handle)
	}
	return handle, nil
}

func MVKRevoke(handle string) {
	if !soAvail {
		return
	}
	h := C.CString(handle)
	defer C.free(unsafe.Pointer(h))
	callProc("grimcore_mvk_revoke", uintptr(unsafe.Pointer(h)))
}

func SessionCreate() (string, [32]byte, error) {
	var keyOut [32]byte
	if !soAvail {
		return "", keyOut, fmt.Errorf("session create requires Rust enclave")
	}
	r, err := callProc("grimcore_session_create",
		uintptr(unsafe.Pointer(&keyOut[0])),
		uintptr(32),
	)
	if err != nil {
		return "", keyOut, err
	}
	handle := uint8PtrToString(r)
	freeCString(r)
	if len(handle) > 5 && handle[:5] == "ERROR" {
		return "", keyOut, fmt.Errorf("session_create: %s", handle)
	}
	return handle, keyOut, nil
}

func SessionDestroy(handle string) {
	if !soAvail {
		return
	}
	h := C.CString(handle)
	defer C.free(unsafe.Pointer(h))
	callProc("grimcore_session_destroy", uintptr(unsafe.Pointer(h)))
}

func SecureWipeFile(path string) error {
	if !soAvail {
		return fmt.Errorf("secure wipe requires Rust enclave")
	}
	h := C.CString(path)
	defer C.free(unsafe.Pointer(h))
	r, err := callProc("grimcore_secure_wipe", uintptr(unsafe.Pointer(h)))
	if err != nil {
		return err
	}
	msg := uint8PtrToString(r)
	freeCString(r)
	if msg != "OK" {
		return fmt.Errorf("secure_wipe: %s", msg)
	}
	return nil
}

func EncryptHandle(handle string, plaintext, aad []byte) ([]byte, error) {
	if !soAvail {
		return nil, fmt.Errorf("encrypt requires Rust enclave")
	}
	cName := C.CString("grimcore_encrypt_handle")
	defer C.free(unsafe.Pointer(cName))
	fn := C.dlsym(so, cName)
	if fn == nil {
		return nil, fmt.Errorf("find grimcore_encrypt_handle: symbol not found")
	}

	h := C.CString(handle)
	defer C.free(unsafe.Pointer(h))
	outBuf := make([]byte, len(plaintext)+64)
	var outLen C.uint32_t = C.uint32_t(len(outBuf))

	var aadPtr C.uintptr_t
	var aadLen C.uintptr_t
	if len(aad) > 0 {
		aadPtr = C.uintptr_t(uintptr(unsafe.Pointer(&aad[0])))
		aadLen = C.uintptr_t(len(aad))
	}

	r := C.bridge_call(
		C.rustbridge_fn(fn),
		C.uintptr_t(uintptr(unsafe.Pointer(h))),
		C.uintptr_t(uintptr(unsafe.Pointer(&plaintext[0]))),
		C.uintptr_t(len(plaintext)),
		aadPtr, aadLen,
		C.uintptr_t(uintptr(unsafe.Pointer(&outBuf[0]))),
		C.uintptr_t(uintptr(unsafe.Pointer(&outLen))),
		0, 0, 0, 0, 0,
	)
	msg := uint8PtrToString(uintptr(r))
	freeCString(uintptr(r))
	if msg != "OK" {
		return nil, fmt.Errorf("encrypt_handle: %s", msg)
	}
	return outBuf[:outLen], nil
}

func DecryptHandle(handle string, ciphertext, aad []byte) ([]byte, error) {
	if !soAvail {
		return nil, fmt.Errorf("decrypt requires Rust enclave")
	}
	cName := C.CString("grimcore_decrypt_handle")
	defer C.free(unsafe.Pointer(cName))
	fn := C.dlsym(so, cName)
	if fn == nil {
		return nil, fmt.Errorf("find grimcore_decrypt_handle: symbol not found")
	}

	h := C.CString(handle)
	defer C.free(unsafe.Pointer(h))
	outBuf := make([]byte, len(ciphertext))
	var outLen C.uint32_t = C.uint32_t(len(outBuf))

	var aadPtr C.uintptr_t
	var aadLen C.uintptr_t
	if len(aad) > 0 {
		aadPtr = C.uintptr_t(uintptr(unsafe.Pointer(&aad[0])))
		aadLen = C.uintptr_t(len(aad))
	}

	r := C.bridge_call(
		C.rustbridge_fn(fn),
		C.uintptr_t(uintptr(unsafe.Pointer(h))),
		C.uintptr_t(uintptr(unsafe.Pointer(&ciphertext[0]))),
		C.uintptr_t(len(ciphertext)),
		aadPtr, aadLen,
		C.uintptr_t(uintptr(unsafe.Pointer(&outBuf[0]))),
		C.uintptr_t(uintptr(unsafe.Pointer(&outLen))),
		0, 0, 0, 0, 0,
	)
	msg := uint8PtrToString(uintptr(r))
	freeCString(uintptr(r))
	if msg != "OK" {
		return nil, fmt.Errorf("decrypt_handle: %s", msg)
	}
	return outBuf[:outLen], nil
}

func SKEEncrypt(handle string, plaintext []byte) ([]byte, error) {
	return EncryptHandle(handle, plaintext, nil)
}

func SKEDecrypt(handle string, ciphertext []byte) ([]byte, error) {
	return DecryptHandle(handle, ciphertext, nil)
}

func DeriveWorkspaceKey(masterKey []byte, workspaceID string) ([32]byte, error) {
	var result [32]byte
	if !soAvail {
		return result, fmt.Errorf("workspace key derivation requires Rust enclave")
	}
	wsID := C.CString(workspaceID)
	defer C.free(unsafe.Pointer(wsID))
	r, err := callProc("grimcore_derive_workspace_key",
		uintptr(unsafe.Pointer(&masterKey[0])),
		uintptr(len(masterKey)),
		uintptr(unsafe.Pointer(wsID)),
		uintptr(unsafe.Pointer(&result[0])),
		uintptr(len(result)),
	)
	if err != nil {
		return result, err
	}
	msg := uint8PtrToString(r)
	freeCString(r)
	if msg != "OK" {
		return result, fmt.Errorf("derive_workspace_key: %s", msg)
	}
	return result, nil
}

func DeriveCoordinate(entropyData []byte, offsets []int64) ([]byte, error) {
	if !soAvail {
		return nil, fmt.Errorf("coordinate derivation requires Rust enclave")
	}
	offsetsJSON := encodeOffsetsJSON(offsets)
	var keyOut [32]byte

	jsonPtr := C.CString(offsetsJSON)
	defer C.free(unsafe.Pointer(jsonPtr))
	r, err := callProc("grimcore_derive_coordinate",
		uintptr(unsafe.Pointer(&entropyData[0])),
		uintptr(len(entropyData)),
		uintptr(unsafe.Pointer(jsonPtr)),
		uintptr(unsafe.Pointer(&keyOut[0])),
		uintptr(32),
	)
	if err != nil {
		return nil, err
	}
	msg := uint8PtrToString(r)
	freeCString(r)
	if msg != "OK" {
		return nil, fmt.Errorf("derive_coordinate: %s", msg)
	}
	return keyOut[:], nil
}

func GenerateEntropyFile(path string, lineCount int) error {
	if !soAvail {
		return fmt.Errorf("entropy generation requires Rust enclave")
	}
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	r, err := callProc("generate_entropy_file", uintptr(unsafe.Pointer(p)), uintptr(lineCount))
	if err != nil {
		return err
	}
	msg := uint8PtrToString(r)
	freeCString(r)
	if msg != "OK" {
		return fmt.Errorf("generate_entropy_file: %s", msg)
	}
	return nil
}

func DeriveArgon2id(password, salt []byte, time, memory uint32, threads uint8, keyLen uint32) ([]byte, error) {
	if !soAvail {
		return nil, fmt.Errorf("argon2id derivation requires Rust enclave")
	}
	cName := C.CString("grimcore_derive_argon2id")
	defer C.free(unsafe.Pointer(cName))
	fn := C.dlsym(so, cName)
	if fn == nil {
		return nil, fmt.Errorf("find grimcore_derive_argon2id: symbol not found")
	}
	outBuf := make([]byte, keyLen)
	var outLen C.uint32_t = C.uint32_t(len(outBuf))

	r := C.bridge_call(
		C.rustbridge_fn(fn),
		C.uintptr_t(uintptr(unsafe.Pointer(&password[0]))),
		C.uintptr_t(len(password)),
		C.uintptr_t(uintptr(unsafe.Pointer(&salt[0]))),
		C.uintptr_t(len(salt)),
		C.uintptr_t(time),
		C.uintptr_t(memory),
		C.uintptr_t(threads),
		C.uintptr_t(keyLen),
		C.uintptr_t(uintptr(unsafe.Pointer(&outBuf[0]))),
		C.uintptr_t(uintptr(unsafe.Pointer(&outLen))),
		0, 0,
	)
	msg := uint8PtrToString(uintptr(r))
	freeCString(uintptr(r))
	if msg != "OK" {
		return nil, fmt.Errorf("derive_argon2id: %s", msg)
	}
	return outBuf[:outLen], nil
}
