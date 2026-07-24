#include "trustdb_sdf_adapter_v1.h"

#include <stdlib.h>
#include <string.h>

typedef struct fake_device {
  int health_busy;
  int bad_identity;
} fake_device;

typedef struct fake_session {
  fake_device *device;
  uint64_t active_keys;
} fake_session;

static trustdb_sdf_status_v1 fake_open_device(
    const uint8_t *config, uint32_t config_len,
    trustdb_sdf_device_v1 *device) {
  fake_device *value;
  if (config == NULL || config_len == 0 || device == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  value = (fake_device *)calloc(1, sizeof(*value));
  if (value == NULL) {
    return TRUSTDB_SDF_INTERNAL;
  }
  if (config_len == 11 && memcmp(config, "health=busy", 11) == 0) {
    value->health_busy = 1;
  }
  if (config_len == 12 && memcmp(config, "bad-identity", 12) == 0) {
    value->bad_identity = 1;
  }
  *device = value;
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_close_device(
    trustdb_sdf_device_v1 device) {
  if (device == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  free(device);
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_device_identity(
    trustdb_sdf_device_v1 device,
    trustdb_sdf_device_identity_v1 *identity) {
  fake_device *value = (fake_device *)device;
  if (value == NULL || identity == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  if (value->bad_identity) {
    memset(identity, 'x', sizeof(*identity));
    return TRUSTDB_SDF_OK;
  }
  memset(identity, 0, sizeof(*identity));
  memcpy(identity->adapter_id, "trustdb.fake-sdf", 16);
  memcpy(identity->adapter_version, "1.0.0", 5);
  memcpy(identity->device_id, "sdf-production", 14);
  memcpy(identity->serial, "fake-serial-1", 13);
  memcpy(identity->firmware, "fake-firmware-1", 15);
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_device_capabilities(
    trustdb_sdf_device_v1 device, uint64_t *capabilities) {
  if (device == NULL || capabilities == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  *capabilities =
      TRUSTDB_SDF_CAP_HEALTH |
      TRUSTDB_SDF_CAP_SM2_SIGN |
      TRUSTDB_SDF_CAP_SM2_PUBLIC_KEY |
      TRUSTDB_SDF_CAP_RANDOM |
      TRUSTDB_SDF_CAP_SM4_KEK_GENERATE |
      TRUSTDB_SDF_CAP_SM4_KEK_IMPORT |
      TRUSTDB_SDF_CAP_SM4_CBC |
      TRUSTDB_SDF_CAP_SM4_MAC;
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_open_session(
    trustdb_sdf_device_v1 device, trustdb_sdf_session_v1 *session) {
  fake_session *value;
  if (device == NULL || session == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  value = (fake_session *)calloc(1, sizeof(*value));
  if (value == NULL) {
    return TRUSTDB_SDF_INTERNAL;
  }
  value->device = (fake_device *)device;
  *session = value;
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_close_session(
    trustdb_sdf_session_v1 session) {
  fake_session *value = (fake_session *)session;
  if (value == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  if (value->active_keys != 0) {
    free(value);
    return TRUSTDB_SDF_FAILED_PRECONDITION;
  }
  free(value);
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_health(
    trustdb_sdf_session_v1 session) {
  fake_session *value = (fake_session *)session;
  if (value == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  if (value->device->health_busy) {
    return TRUSTDB_SDF_BUSY;
  }
  return TRUSTDB_SDF_OK;
}

static int fake_credential(
    const uint8_t *credential, uint32_t credential_len) {
  return credential != NULL && credential_len == 6 &&
      memcmp(credential, "846295", 6) == 0;
}

static trustdb_sdf_status_v1 fake_public_key(
    trustdb_sdf_session_v1 session, uint32_t key_index,
    const uint8_t *credential, uint32_t credential_len,
    uint8_t public_key[TRUSTDB_SDF_SM2_PUBLIC_KEY_BYTES]) {
  if (session == NULL || public_key == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  if (!fake_credential(credential, credential_len)) {
    return TRUSTDB_SDF_UNAUTHENTICATED;
  }
  if (key_index != 7) {
    return TRUSTDB_SDF_NOT_FOUND;
  }
  memset(public_key, 0, TRUSTDB_SDF_SM2_PUBLIC_KEY_BYTES);
  public_key[0] = 4;
  public_key[32] = 1;
  public_key[64] = 2;
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_sign_digest(
    trustdb_sdf_session_v1 session, uint32_t key_index,
    const uint8_t *credential, uint32_t credential_len,
    const uint8_t digest[TRUSTDB_SDF_SM2_DIGEST_BYTES],
    uint8_t signature[TRUSTDB_SDF_SM2_SIGNATURE_BYTES]) {
  uint32_t index;
  if (session == NULL || digest == NULL || signature == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  if (!fake_credential(credential, credential_len)) {
    return TRUSTDB_SDF_UNAUTHENTICATED;
  }
  if (key_index != 7) {
    return TRUSTDB_SDF_NOT_FOUND;
  }
  for (index = 0; index < TRUSTDB_SDF_SM2_SIGNATURE_BYTES; index++) {
    signature[index] = digest[index % TRUSTDB_SDF_SM2_DIGEST_BYTES];
  }
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_random(
    trustdb_sdf_session_v1 session, uint8_t *output,
    uint32_t output_len) {
  uint32_t index;
  if (session == NULL || output == NULL || output_len == 0) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  for (index = 0; index < output_len; index++) {
    output[index] = (uint8_t)(index ^ 0xa5);
  }
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_generate_sm4(
    trustdb_sdf_session_v1 session, uint32_t kek_index,
    const uint8_t *credential, uint32_t credential_len,
    uint8_t *wrapped, uint32_t wrapped_capacity,
    uint32_t *wrapped_len, trustdb_sdf_session_key_v1 *key) {
  fake_session *value = (fake_session *)session;
  uint32_t index;
  if (value == NULL || wrapped == NULL || wrapped_len == NULL || key == NULL ||
      wrapped_capacity < 17) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  if (!fake_credential(credential, credential_len)) {
    return TRUSTDB_SDF_UNAUTHENTICATED;
  }
  if (kek_index != 11) {
    return TRUSTDB_SDF_NOT_FOUND;
  }
  wrapped[0] = 0x53;
  for (index = 1; index < 17; index++) {
    wrapped[index] = (uint8_t)(index ^ 0x3c);
  }
  *wrapped_len = 17;
  *key = 1;
  value->active_keys |= UINT64_C(1) << 1;
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_import_sm4(
    trustdb_sdf_session_v1 session, uint32_t kek_index,
    const uint8_t *credential, uint32_t credential_len,
    const uint8_t *wrapped, uint32_t wrapped_len,
    trustdb_sdf_session_key_v1 *key) {
  fake_session *value = (fake_session *)session;
  if (value == NULL || wrapped == NULL || key == NULL) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  if (!fake_credential(credential, credential_len)) {
    return TRUSTDB_SDF_UNAUTHENTICATED;
  }
  if (kek_index != 11 || wrapped_len != 17 || wrapped[0] != 0x53) {
    return TRUSTDB_SDF_NOT_FOUND;
  }
  *key = 2;
  value->active_keys |= UINT64_C(1) << 2;
  return TRUSTDB_SDF_OK;
}

static int fake_key_active(fake_session *session, uint64_t key) {
  return key > 0 && key < 63 &&
      (session->active_keys & (UINT64_C(1) << key)) != 0;
}

static trustdb_sdf_status_v1 fake_crypt(
    trustdb_sdf_session_v1 session, trustdb_sdf_session_key_v1 key,
    const uint8_t iv[TRUSTDB_SDF_SM4_BLOCK_BYTES],
    const uint8_t *input, uint32_t input_len, uint8_t *output) {
  fake_session *value = (fake_session *)session;
  uint32_t index;
  if (value == NULL || iv == NULL || input == NULL || output == NULL ||
      input_len == 0 || input_len % TRUSTDB_SDF_SM4_BLOCK_BYTES != 0) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  if (!fake_key_active(value, key)) {
    return TRUSTDB_SDF_FAILED_PRECONDITION;
  }
  for (index = 0; index < input_len; index++) {
    output[index] = input[index] ^ iv[index % TRUSTDB_SDF_SM4_BLOCK_BYTES] ^ 0x5a;
  }
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_mac(
    trustdb_sdf_session_v1 session, trustdb_sdf_session_key_v1 key,
    const uint8_t iv[TRUSTDB_SDF_SM4_BLOCK_BYTES],
    const uint8_t *input, uint32_t input_len,
    uint8_t mac[TRUSTDB_SDF_SM4_BLOCK_BYTES]) {
  fake_session *value = (fake_session *)session;
  uint32_t index;
  if (value == NULL || iv == NULL || input == NULL || mac == NULL ||
      input_len == 0 || input_len % TRUSTDB_SDF_SM4_BLOCK_BYTES != 0) {
    return TRUSTDB_SDF_INVALID_ARGUMENT;
  }
  if (!fake_key_active(value, key)) {
    return TRUSTDB_SDF_FAILED_PRECONDITION;
  }
  memcpy(mac, iv, TRUSTDB_SDF_SM4_BLOCK_BYTES);
  for (index = 0; index < input_len; index++) {
    mac[index % TRUSTDB_SDF_SM4_BLOCK_BYTES] ^= input[index];
  }
  return TRUSTDB_SDF_OK;
}

static trustdb_sdf_status_v1 fake_destroy(
    trustdb_sdf_session_v1 session, trustdb_sdf_session_key_v1 key) {
  fake_session *value = (fake_session *)session;
  if (value == NULL || !fake_key_active(value, key)) {
    return TRUSTDB_SDF_FAILED_PRECONDITION;
  }
  value->active_keys &= ~(UINT64_C(1) << key);
  return TRUSTDB_SDF_OK;
}

trustdb_sdf_status_v1 trustdb_sdf_adapter_get_api_v1(
    uint32_t host_abi, trustdb_sdf_api_v1 *api) {
  if (host_abi != TRUSTDB_SDF_ADAPTER_ABI_V1 || api == NULL ||
      api->struct_size != sizeof(*api) ||
      api->abi_version != TRUSTDB_SDF_ADAPTER_ABI_V1) {
    return TRUSTDB_SDF_UNSUPPORTED;
  }
  api->open_device = fake_open_device;
  api->close_device = fake_close_device;
  api->device_identity = fake_device_identity;
  api->device_capabilities = fake_device_capabilities;
  api->open_session = fake_open_session;
  api->close_session = fake_close_session;
  api->health = fake_health;
  api->sm2_public_key = fake_public_key;
  api->sm2_sign_digest = fake_sign_digest;
  api->generate_random = fake_random;
  api->generate_sm4_key_with_kek = fake_generate_sm4;
  api->import_sm4_key_with_kek = fake_import_sm4;
  api->encrypt_sm4_cbc = fake_crypt;
  api->decrypt_sm4_cbc = fake_crypt;
  api->calculate_sm4_mac = fake_mac;
  api->destroy_session_key = fake_destroy;
  return TRUSTDB_SDF_OK;
}
