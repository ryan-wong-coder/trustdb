#ifndef TRUSTDB_SDF_ADAPTER_V1_H
#define TRUSTDB_SDF_ADAPTER_V1_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define TRUSTDB_SDF_ADAPTER_ABI_V1 UINT32_C(0x00010000)
#define TRUSTDB_SDF_SM2_PUBLIC_KEY_BYTES UINT32_C(65)
#define TRUSTDB_SDF_SM2_DIGEST_BYTES UINT32_C(32)
#define TRUSTDB_SDF_SM2_SIGNATURE_BYTES UINT32_C(64)
#define TRUSTDB_SDF_SM4_BLOCK_BYTES UINT32_C(16)

#define TRUSTDB_SDF_CAP_HEALTH (UINT64_C(1) << 0)
#define TRUSTDB_SDF_CAP_SM2_SIGN (UINT64_C(1) << 1)
#define TRUSTDB_SDF_CAP_SM2_PUBLIC_KEY (UINT64_C(1) << 2)
#define TRUSTDB_SDF_CAP_RANDOM (UINT64_C(1) << 3)
#define TRUSTDB_SDF_CAP_SM4_KEK_GENERATE (UINT64_C(1) << 4)
#define TRUSTDB_SDF_CAP_SM4_KEK_IMPORT (UINT64_C(1) << 5)
#define TRUSTDB_SDF_CAP_SM4_CBC (UINT64_C(1) << 6)
#define TRUSTDB_SDF_CAP_SM4_MAC (UINT64_C(1) << 7)

typedef uint32_t trustdb_sdf_status_v1;

enum {
  TRUSTDB_SDF_OK = 0,
  TRUSTDB_SDF_INVALID_ARGUMENT = 1,
  TRUSTDB_SDF_NOT_FOUND = 2,
  TRUSTDB_SDF_FAILED_PRECONDITION = 3,
  TRUSTDB_SDF_UNAUTHENTICATED = 4,
  TRUSTDB_SDF_PERMISSION_DENIED = 5,
  TRUSTDB_SDF_UNSUPPORTED = 6,
  TRUSTDB_SDF_BUSY = 7,
  TRUSTDB_SDF_UNAVAILABLE = 8,
  TRUSTDB_SDF_INTERNAL = 255
};

typedef void *trustdb_sdf_device_v1;
typedef void *trustdb_sdf_session_v1;
typedef uint64_t trustdb_sdf_session_key_v1;

typedef struct trustdb_sdf_device_identity_v1 {
  char adapter_id[64];
  char adapter_version[64];
  char device_id[256];
  char serial[128];
  char firmware[128];
} trustdb_sdf_device_identity_v1;

/*
 * Every pointer supplied by TrustDB is borrowed only for the duration of the
 * call. An adapter must copy configuration or input that it needs afterward.
 * It must map vendor status values to the fixed statuses above and must never
 * return vendor error strings, credentials, paths, indexes, or handles.
 */
typedef struct trustdb_sdf_api_v1 {
  uint32_t struct_size;
  uint32_t abi_version;

  trustdb_sdf_status_v1 (*open_device)(
      const uint8_t *config, uint32_t config_len,
      trustdb_sdf_device_v1 *device);
  trustdb_sdf_status_v1 (*close_device)(trustdb_sdf_device_v1 device);
  trustdb_sdf_status_v1 (*device_identity)(
      trustdb_sdf_device_v1 device,
      trustdb_sdf_device_identity_v1 *identity);
  trustdb_sdf_status_v1 (*device_capabilities)(
      trustdb_sdf_device_v1 device, uint64_t *capabilities);
  trustdb_sdf_status_v1 (*open_session)(
      trustdb_sdf_device_v1 device, trustdb_sdf_session_v1 *session);
  trustdb_sdf_status_v1 (*close_session)(trustdb_sdf_session_v1 session);
  trustdb_sdf_status_v1 (*health)(trustdb_sdf_session_v1 session);

  trustdb_sdf_status_v1 (*sm2_public_key)(
      trustdb_sdf_session_v1 session, uint32_t key_index,
      const uint8_t *credential, uint32_t credential_len,
      uint8_t public_key[TRUSTDB_SDF_SM2_PUBLIC_KEY_BYTES]);
  trustdb_sdf_status_v1 (*sm2_sign_digest)(
      trustdb_sdf_session_v1 session, uint32_t key_index,
      const uint8_t *credential, uint32_t credential_len,
      const uint8_t digest[TRUSTDB_SDF_SM2_DIGEST_BYTES],
      uint8_t signature[TRUSTDB_SDF_SM2_SIGNATURE_BYTES]);

  trustdb_sdf_status_v1 (*generate_random)(
      trustdb_sdf_session_v1 session, uint8_t *output,
      uint32_t output_len);
  trustdb_sdf_status_v1 (*generate_sm4_key_with_kek)(
      trustdb_sdf_session_v1 session, uint32_t kek_index,
      const uint8_t *credential, uint32_t credential_len,
      uint8_t *wrapped, uint32_t wrapped_capacity,
      uint32_t *wrapped_len, trustdb_sdf_session_key_v1 *key);
  trustdb_sdf_status_v1 (*import_sm4_key_with_kek)(
      trustdb_sdf_session_v1 session, uint32_t kek_index,
      const uint8_t *credential, uint32_t credential_len,
      const uint8_t *wrapped, uint32_t wrapped_len,
      trustdb_sdf_session_key_v1 *key);
  trustdb_sdf_status_v1 (*encrypt_sm4_cbc)(
      trustdb_sdf_session_v1 session, trustdb_sdf_session_key_v1 key,
      const uint8_t iv[TRUSTDB_SDF_SM4_BLOCK_BYTES],
      const uint8_t *input, uint32_t input_len, uint8_t *output);
  trustdb_sdf_status_v1 (*decrypt_sm4_cbc)(
      trustdb_sdf_session_v1 session, trustdb_sdf_session_key_v1 key,
      const uint8_t iv[TRUSTDB_SDF_SM4_BLOCK_BYTES],
      const uint8_t *input, uint32_t input_len, uint8_t *output);
  trustdb_sdf_status_v1 (*calculate_sm4_mac)(
      trustdb_sdf_session_v1 session, trustdb_sdf_session_key_v1 key,
      const uint8_t iv[TRUSTDB_SDF_SM4_BLOCK_BYTES],
      const uint8_t *input, uint32_t input_len,
      uint8_t mac[TRUSTDB_SDF_SM4_BLOCK_BYTES]);
  trustdb_sdf_status_v1 (*destroy_session_key)(
      trustdb_sdf_session_v1 session, trustdb_sdf_session_key_v1 key);
} trustdb_sdf_api_v1;

/*
 * Adapter libraries export exactly this symbol. The host initializes
 * api->struct_size and api->abi_version before calling it. The adapter must
 * fill every v1 function pointer or return TRUSTDB_SDF_UNSUPPORTED.
 */
typedef trustdb_sdf_status_v1 (*trustdb_sdf_adapter_get_api_v1_fn)(
    uint32_t host_abi, trustdb_sdf_api_v1 *api);

trustdb_sdf_status_v1 trustdb_sdf_adapter_get_api_v1(
    uint32_t host_abi, trustdb_sdf_api_v1 *api);

#ifdef __cplusplus
}
#endif

#endif
