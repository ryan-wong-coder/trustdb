//go:build sdf && cgo && (linux || darwin)

package sdfsigner

/*
#cgo CFLAGS: -I${SRCDIR}/../../sdk/sdfadapter
#cgo linux LDFLAGS: -ldl

#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include "trustdb_sdf_adapter_v1.h"

static int trustdb_sdf_api_complete(const trustdb_sdf_api_v1 *api) {
	return api != NULL &&
		api->struct_size == sizeof(trustdb_sdf_api_v1) &&
		api->abi_version == TRUSTDB_SDF_ADAPTER_ABI_V1 &&
		api->open_device != NULL &&
		api->close_device != NULL &&
		api->device_identity != NULL &&
		api->device_capabilities != NULL &&
		api->open_session != NULL &&
		api->close_session != NULL &&
		api->health != NULL &&
		api->sm2_public_key != NULL &&
		api->sm2_sign_digest != NULL &&
		api->generate_random != NULL &&
		api->generate_sm4_key_with_kek != NULL &&
		api->import_sm4_key_with_kek != NULL &&
		api->encrypt_sm4_cbc != NULL &&
		api->decrypt_sm4_cbc != NULL &&
		api->calculate_sm4_mac != NULL &&
		api->destroy_session_key != NULL;
}

static int trustdb_sdf_load(
		const char *path, void **library, trustdb_sdf_api_v1 *api) {
	void *loaded;
	trustdb_sdf_adapter_get_api_v1_fn get_api;
	trustdb_sdf_status_v1 status;
	if (path == NULL || library == NULL || api == NULL) {
		return 0;
	}
	loaded = dlopen(path, RTLD_NOW | RTLD_LOCAL);
	if (loaded == NULL) {
		return 0;
	}
	get_api = (trustdb_sdf_adapter_get_api_v1_fn)dlsym(
		loaded, "trustdb_sdf_adapter_get_api_v1");
	if (get_api == NULL) {
		dlclose(loaded);
		return 0;
	}
	memset(api, 0, sizeof(*api));
	api->struct_size = sizeof(*api);
	api->abi_version = TRUSTDB_SDF_ADAPTER_ABI_V1;
	status = get_api(TRUSTDB_SDF_ADAPTER_ABI_V1, api);
	if (status != TRUSTDB_SDF_OK || !trustdb_sdf_api_complete(api)) {
		memset(api, 0, sizeof(*api));
		dlclose(loaded);
		return 0;
	}
	*library = loaded;
	return 1;
}

static void trustdb_sdf_unload(void *library) {
	if (library != NULL) {
		dlclose(library);
	}
}

static trustdb_sdf_status_v1 trustdb_sdf_open_device(
		trustdb_sdf_api_v1 *api, const uint8_t *config, uint32_t config_len,
		trustdb_sdf_device_v1 *device) {
	return api->open_device(config, config_len, device);
}
static trustdb_sdf_status_v1 trustdb_sdf_close_device(
		trustdb_sdf_api_v1 *api, trustdb_sdf_device_v1 device) {
	return api->close_device(device);
}
static trustdb_sdf_status_v1 trustdb_sdf_device_identity(
		trustdb_sdf_api_v1 *api, trustdb_sdf_device_v1 device,
		trustdb_sdf_device_identity_v1 *identity) {
	return api->device_identity(device, identity);
}
static trustdb_sdf_status_v1 trustdb_sdf_device_capabilities(
		trustdb_sdf_api_v1 *api, trustdb_sdf_device_v1 device,
		uint64_t *capabilities) {
	return api->device_capabilities(device, capabilities);
}
static trustdb_sdf_status_v1 trustdb_sdf_open_session(
		trustdb_sdf_api_v1 *api, trustdb_sdf_device_v1 device,
		trustdb_sdf_session_v1 *session) {
	return api->open_session(device, session);
}
static trustdb_sdf_status_v1 trustdb_sdf_close_session(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session) {
	return api->close_session(session);
}
static trustdb_sdf_status_v1 trustdb_sdf_health(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session) {
	return api->health(session);
}
static trustdb_sdf_status_v1 trustdb_sdf_public_key(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session,
		uint32_t key_index, const uint8_t *credential,
		uint32_t credential_len, uint8_t *public_key) {
	return api->sm2_public_key(
		session, key_index, credential, credential_len, public_key);
}
static trustdb_sdf_status_v1 trustdb_sdf_sign_digest(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session,
		uint32_t key_index, const uint8_t *credential,
		uint32_t credential_len, const uint8_t *digest,
		uint8_t *signature) {
	return api->sm2_sign_digest(
		session, key_index, credential, credential_len, digest, signature);
}
static trustdb_sdf_status_v1 trustdb_sdf_random(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session,
		uint8_t *output, uint32_t output_len) {
	return api->generate_random(session, output, output_len);
}
static trustdb_sdf_status_v1 trustdb_sdf_generate_sm4(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session,
		uint32_t kek_index, const uint8_t *credential,
		uint32_t credential_len, uint8_t *wrapped,
		uint32_t wrapped_capacity, uint32_t *wrapped_len,
		trustdb_sdf_session_key_v1 *key) {
	return api->generate_sm4_key_with_kek(
		session, kek_index, credential, credential_len, wrapped,
		wrapped_capacity, wrapped_len, key);
}
static trustdb_sdf_status_v1 trustdb_sdf_import_sm4(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session,
		uint32_t kek_index, const uint8_t *credential,
		uint32_t credential_len, const uint8_t *wrapped,
		uint32_t wrapped_len, trustdb_sdf_session_key_v1 *key) {
	return api->import_sm4_key_with_kek(
		session, kek_index, credential, credential_len, wrapped,
		wrapped_len, key);
}
static trustdb_sdf_status_v1 trustdb_sdf_encrypt_sm4(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session,
		trustdb_sdf_session_key_v1 key, const uint8_t *iv,
		const uint8_t *input, uint32_t input_len, uint8_t *output) {
	return api->encrypt_sm4_cbc(session, key, iv, input, input_len, output);
}
static trustdb_sdf_status_v1 trustdb_sdf_decrypt_sm4(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session,
		trustdb_sdf_session_key_v1 key, const uint8_t *iv,
		const uint8_t *input, uint32_t input_len, uint8_t *output) {
	return api->decrypt_sm4_cbc(session, key, iv, input, input_len, output);
}
static trustdb_sdf_status_v1 trustdb_sdf_mac_sm4(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session,
		trustdb_sdf_session_key_v1 key, const uint8_t *iv,
		const uint8_t *input, uint32_t input_len, uint8_t *mac) {
	return api->calculate_sm4_mac(session, key, iv, input, input_len, mac);
}
static trustdb_sdf_status_v1 trustdb_sdf_destroy_key(
		trustdb_sdf_api_v1 *api, trustdb_sdf_session_v1 session,
		trustdb_sdf_session_key_v1 key) {
	return api->destroy_session_key(session, key);
}
*/
import "C"

import (
	"bytes"
	"context"
	"math"
	"sync"
	"unsafe"
)

// nativeBackend loads only TrustDB's adapter ABI. Vendor headers and libraries
// are confined to the deployment-built adapter shared object.
type nativeBackend struct {
	mu      sync.RWMutex
	library unsafe.Pointer
	api     C.trustdb_sdf_api_v1
	device  C.trustdb_sdf_device_v1
	closed  bool
}

func OpenNativeBackend(adapterPath string, adapterConfig []byte) (Backend, error) {
	if !validAbsolutePath(adapterPath) ||
		len(adapterConfig) == 0 || len(adapterConfig) > MaxAdapterConfigBytes {
		return nil, newFault(faultInvalid)
	}
	cPath := C.CString(adapterPath)
	defer C.free(unsafe.Pointer(cPath))
	backend := &nativeBackend{}
	if C.trustdb_sdf_load(cPath, &backend.library, &backend.api) != 1 {
		return nil, newFault(faultUnavailable)
	}
	status := C.trustdb_sdf_open_device(
		&backend.api,
		bytePointer(adapterConfig),
		C.uint32_t(len(adapterConfig)),
		&backend.device,
	)
	if err := classifyNativeStatus(status); err != nil || backend.device == nil {
		if backend.device != nil {
			_ = classifyNativeStatus(C.trustdb_sdf_close_device(&backend.api, backend.device))
		}
		C.trustdb_sdf_unload(backend.library)
		backend.library = nil
		backend.device = nil
		if err != nil {
			return nil, err
		}
		return nil, newFault(faultInternal)
	}
	return backend, nil
}

func (b *nativeBackend) Discover(ctx context.Context, reference string) (Device, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validIdentifier(reference, 4096) {
		return nil, newFault(faultInvalid)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed || b.library == nil || b.device == nil {
		return nil, newFault(faultUnavailable)
	}
	return &nativeDevice{backend: b}, nil
}

func (b *nativeBackend) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	var closeErr error
	if b.device != nil {
		closeErr = classifyNativeStatus(C.trustdb_sdf_close_device(&b.api, b.device))
		b.device = nil
	}
	if b.library != nil {
		C.trustdb_sdf_unload(b.library)
		b.library = nil
	}
	b.api = C.trustdb_sdf_api_v1{}
	return closeErr
}

type nativeDevice struct {
	backend *nativeBackend
}

func (d *nativeDevice) Identity(ctx context.Context) (DeviceIdentity, error) {
	if err := ctx.Err(); err != nil {
		return DeviceIdentity{}, err
	}
	d.backend.mu.RLock()
	defer d.backend.mu.RUnlock()
	if d.backend.closed || d.backend.device == nil {
		return DeviceIdentity{}, newFault(faultUnavailable)
	}
	var native C.trustdb_sdf_device_identity_v1
	if err := classifyNativeStatus(C.trustdb_sdf_device_identity(
		&d.backend.api, d.backend.device, &native,
	)); err != nil {
		return DeviceIdentity{}, err
	}
	identity := DeviceIdentity{}
	var err error
	if identity.AdapterID, err = fixedCString(unsafe.Pointer(&native.adapter_id[0]), len(native.adapter_id)); err != nil {
		return DeviceIdentity{}, newFault(faultPrecondition)
	}
	if identity.AdapterVersion, err = fixedCString(unsafe.Pointer(&native.adapter_version[0]), len(native.adapter_version)); err != nil {
		return DeviceIdentity{}, newFault(faultPrecondition)
	}
	if identity.DeviceID, err = fixedCString(unsafe.Pointer(&native.device_id[0]), len(native.device_id)); err != nil {
		return DeviceIdentity{}, newFault(faultPrecondition)
	}
	if identity.Serial, err = fixedCString(unsafe.Pointer(&native.serial[0]), len(native.serial)); err != nil {
		return DeviceIdentity{}, newFault(faultPrecondition)
	}
	if identity.Firmware, err = fixedCString(unsafe.Pointer(&native.firmware[0]), len(native.firmware)); err != nil {
		return DeviceIdentity{}, newFault(faultPrecondition)
	}
	if err := ctx.Err(); err != nil {
		return DeviceIdentity{}, err
	}
	return identity, nil
}

func (d *nativeDevice) Capabilities(ctx context.Context) (Capability, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	d.backend.mu.RLock()
	defer d.backend.mu.RUnlock()
	if d.backend.closed || d.backend.device == nil {
		return 0, newFault(faultUnavailable)
	}
	var capabilities C.uint64_t
	if err := classifyNativeStatus(C.trustdb_sdf_device_capabilities(
		&d.backend.api, d.backend.device, &capabilities,
	)); err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return Capability(capabilities), nil
}

func (d *nativeDevice) OpenSession(ctx context.Context) (Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.backend.mu.RLock()
	defer d.backend.mu.RUnlock()
	if d.backend.closed || d.backend.device == nil {
		return nil, newFault(faultUnavailable)
	}
	var session C.trustdb_sdf_session_v1
	if err := classifyNativeStatus(C.trustdb_sdf_open_session(
		&d.backend.api, d.backend.device, &session,
	)); err != nil {
		return nil, err
	}
	if session == nil {
		return nil, newFault(faultInternal)
	}
	return &nativeSession{backend: d.backend, session: session}, nil
}

type nativeSession struct {
	backend *nativeBackend
	session C.trustdb_sdf_session_v1

	closeOnce sync.Once
}

func (s *nativeSession) Health(ctx context.Context) error {
	return s.call(ctx, func() C.trustdb_sdf_status_v1 {
		return C.trustdb_sdf_health(&s.backend.api, s.session)
	})
}

func (s *nativeSession) PublicKey(ctx context.Context, keyIndex uint32, credential []byte) ([]byte, error) {
	if keyIndex == 0 || len(credential) == 0 || len(credential) > MaxCredentialBytes {
		return nil, newFault(faultInvalid)
	}
	publicKey := make([]byte, C.TRUSTDB_SDF_SM2_PUBLIC_KEY_BYTES)
	err := s.call(ctx, func() C.trustdb_sdf_status_v1 {
		return C.trustdb_sdf_public_key(
			&s.backend.api, s.session, C.uint32_t(keyIndex),
			bytePointer(credential), C.uint32_t(len(credential)),
			bytePointer(publicKey),
		)
	})
	if err != nil {
		clear(publicKey)
		return nil, err
	}
	return publicKey, nil
}

func (s *nativeSession) SignSM2Digest(ctx context.Context, keyIndex uint32, credential, digest []byte) ([]byte, error) {
	if keyIndex == 0 || len(credential) == 0 || len(credential) > MaxCredentialBytes ||
		len(digest) != C.TRUSTDB_SDF_SM2_DIGEST_BYTES {
		return nil, newFault(faultInvalid)
	}
	signature := make([]byte, C.TRUSTDB_SDF_SM2_SIGNATURE_BYTES)
	err := s.call(ctx, func() C.trustdb_sdf_status_v1 {
		return C.trustdb_sdf_sign_digest(
			&s.backend.api, s.session, C.uint32_t(keyIndex),
			bytePointer(credential), C.uint32_t(len(credential)),
			bytePointer(digest), bytePointer(signature),
		)
	})
	if err != nil {
		clear(signature)
		return nil, err
	}
	return signature, nil
}

func (s *nativeSession) Random(ctx context.Context, length uint32) ([]byte, error) {
	if length == 0 || length > MaxRandomBytes {
		return nil, newFault(faultInvalid)
	}
	output := make([]byte, length)
	err := s.call(ctx, func() C.trustdb_sdf_status_v1 {
		return C.trustdb_sdf_random(
			&s.backend.api, s.session, bytePointer(output), C.uint32_t(length),
		)
	})
	if err != nil {
		clear(output)
		return nil, err
	}
	return output, nil
}

func (s *nativeSession) GenerateSM4KeyWithKEK(ctx context.Context, kekIndex uint32, credential []byte) ([]byte, SessionKeyHandle, error) {
	if kekIndex == 0 || len(credential) == 0 || len(credential) > MaxCredentialBytes {
		return nil, SessionKeyHandle{}, newFault(faultInvalid)
	}
	wrapped := make([]byte, MaxWrappedKeyBytes)
	var wrappedLength C.uint32_t
	var handle C.trustdb_sdf_session_key_v1
	err := s.call(ctx, func() C.trustdb_sdf_status_v1 {
		return C.trustdb_sdf_generate_sm4(
			&s.backend.api, s.session, C.uint32_t(kekIndex),
			bytePointer(credential), C.uint32_t(len(credential)),
			bytePointer(wrapped), C.uint32_t(len(wrapped)), &wrappedLength, &handle,
		)
	})
	if err != nil {
		clear(wrapped)
		if handle != 0 {
			_ = s.destroyHandle(context.Background(), handle)
		}
		return nil, SessionKeyHandle{}, err
	}
	if wrappedLength == 0 || wrappedLength > C.uint32_t(len(wrapped)) || handle == 0 {
		clear(wrapped)
		if handle != 0 {
			_ = s.destroyHandle(context.Background(), handle)
		}
		return nil, SessionKeyHandle{}, newFault(faultPrecondition)
	}
	return append([]byte(nil), wrapped[:wrappedLength]...), newSessionKeyHandle(uint64(handle)), nil
}

func (s *nativeSession) ImportSM4KeyWithKEK(ctx context.Context, kekIndex uint32, credential, wrapped []byte) (SessionKeyHandle, error) {
	if kekIndex == 0 || len(credential) == 0 || len(credential) > MaxCredentialBytes ||
		len(wrapped) == 0 || len(wrapped) > MaxWrappedKeyBytes {
		return SessionKeyHandle{}, newFault(faultInvalid)
	}
	var handle C.trustdb_sdf_session_key_v1
	err := s.call(ctx, func() C.trustdb_sdf_status_v1 {
		return C.trustdb_sdf_import_sm4(
			&s.backend.api, s.session, C.uint32_t(kekIndex),
			bytePointer(credential), C.uint32_t(len(credential)),
			bytePointer(wrapped), C.uint32_t(len(wrapped)), &handle,
		)
	})
	if err != nil {
		if handle != 0 {
			_ = s.destroyHandle(context.Background(), handle)
		}
		return SessionKeyHandle{}, err
	}
	if handle == 0 {
		return SessionKeyHandle{}, newFault(faultPrecondition)
	}
	return newSessionKeyHandle(uint64(handle)), nil
}

func (s *nativeSession) EncryptSM4CBC(ctx context.Context, handle SessionKeyHandle, iv, plaintext []byte) ([]byte, error) {
	return s.cryptSM4(ctx, true, handle, iv, plaintext)
}

func (s *nativeSession) DecryptSM4CBC(ctx context.Context, handle SessionKeyHandle, iv, ciphertext []byte) ([]byte, error) {
	return s.cryptSM4(ctx, false, handle, iv, ciphertext)
}

func (s *nativeSession) cryptSM4(ctx context.Context, encrypt bool, handle SessionKeyHandle, iv, input []byte) ([]byte, error) {
	if handle.value == 0 || len(iv) != SM4BlockBytes || len(input) == 0 ||
		len(input) > MaxSM4OperationBytes || len(input)%SM4BlockBytes != 0 {
		return nil, newFault(faultInvalid)
	}
	output := make([]byte, len(input))
	err := s.call(ctx, func() C.trustdb_sdf_status_v1 {
		if encrypt {
			return C.trustdb_sdf_encrypt_sm4(
				&s.backend.api, s.session, C.trustdb_sdf_session_key_v1(handle.value),
				bytePointer(iv), bytePointer(input), C.uint32_t(len(input)), bytePointer(output),
			)
		}
		return C.trustdb_sdf_decrypt_sm4(
			&s.backend.api, s.session, C.trustdb_sdf_session_key_v1(handle.value),
			bytePointer(iv), bytePointer(input), C.uint32_t(len(input)), bytePointer(output),
		)
	})
	if err != nil {
		clear(output)
		return nil, err
	}
	return output, nil
}

func (s *nativeSession) CalculateSM4MAC(ctx context.Context, handle SessionKeyHandle, iv, input []byte) ([]byte, error) {
	if handle.value == 0 || len(iv) != SM4BlockBytes || len(input) == 0 ||
		len(input) > MaxSM4OperationBytes || len(input)%SM4BlockBytes != 0 {
		return nil, newFault(faultInvalid)
	}
	mac := make([]byte, SM4BlockBytes)
	err := s.call(ctx, func() C.trustdb_sdf_status_v1 {
		return C.trustdb_sdf_mac_sm4(
			&s.backend.api, s.session, C.trustdb_sdf_session_key_v1(handle.value),
			bytePointer(iv), bytePointer(input), C.uint32_t(len(input)), bytePointer(mac),
		)
	})
	if err != nil {
		clear(mac)
		return nil, err
	}
	return mac, nil
}

func (s *nativeSession) DestroySessionKey(ctx context.Context, handle SessionKeyHandle) error {
	if handle.value == 0 {
		return newFault(faultInvalid)
	}
	return s.call(ctx, func() C.trustdb_sdf_status_v1 {
		return C.trustdb_sdf_destroy_key(
			&s.backend.api, s.session, C.trustdb_sdf_session_key_v1(handle.value),
		)
	})
}

func (s *nativeSession) Close() error {
	if s == nil || s.backend == nil {
		return nil
	}
	var closeErr error
	s.closeOnce.Do(func() {
		s.backend.mu.RLock()
		defer s.backend.mu.RUnlock()
		if s.backend.closed || s.session == nil {
			return
		}
		closeErr = classifyNativeStatus(C.trustdb_sdf_close_session(&s.backend.api, s.session))
		s.session = nil
	})
	return closeErr
}

func (s *nativeSession) call(ctx context.Context, operation func() C.trustdb_sdf_status_v1) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.backend.mu.RLock()
	defer s.backend.mu.RUnlock()
	if s.backend.closed || s.session == nil {
		return newFault(faultUnavailable)
	}
	err := classifyNativeStatus(operation())
	if err != nil {
		return err
	}
	return ctx.Err()
}

func (s *nativeSession) destroyHandle(ctx context.Context, handle C.trustdb_sdf_session_key_v1) error {
	if handle == 0 {
		return nil
	}
	return s.call(ctx, func() C.trustdb_sdf_status_v1 {
		return C.trustdb_sdf_destroy_key(&s.backend.api, s.session, handle)
	})
}

func classifyNativeStatus(status C.trustdb_sdf_status_v1) error {
	switch status {
	case C.TRUSTDB_SDF_OK:
		return nil
	case C.TRUSTDB_SDF_INVALID_ARGUMENT:
		return newFault(faultInvalid)
	case C.TRUSTDB_SDF_NOT_FOUND:
		return newFault(faultNotFound)
	case C.TRUSTDB_SDF_FAILED_PRECONDITION:
		return newFault(faultPrecondition)
	case C.TRUSTDB_SDF_UNAUTHENTICATED:
		return newFault(faultAuthentication)
	case C.TRUSTDB_SDF_PERMISSION_DENIED:
		return newFault(faultPermission)
	case C.TRUSTDB_SDF_UNSUPPORTED:
		return newFault(faultUnsupported)
	case C.TRUSTDB_SDF_BUSY:
		return newFault(faultBusy)
	case C.TRUSTDB_SDF_UNAVAILABLE:
		return newFault(faultUnavailable)
	default:
		return newFault(faultInternal)
	}
}

func bytePointer(value []byte) *C.uint8_t {
	if len(value) == 0 {
		return nil
	}
	return (*C.uint8_t)(unsafe.Pointer(&value[0]))
}

func fixedCString(pointer unsafe.Pointer, length int) (string, error) {
	if pointer == nil || length <= 1 || length > math.MaxInt32 {
		return "", newFault(faultPrecondition)
	}
	value := C.GoBytes(pointer, C.int(length))
	terminator := bytes.IndexByte(value, 0)
	if terminator <= 0 {
		return "", newFault(faultPrecondition)
	}
	for _, trailing := range value[terminator+1:] {
		if trailing != 0 {
			return "", newFault(faultPrecondition)
		}
	}
	return string(value[:terminator]), nil
}
