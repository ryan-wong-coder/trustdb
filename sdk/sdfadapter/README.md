# TrustDB SDF adapter ABI v1

This directory defines the C boundary between the optional
`trustdb-signer-sdf` sidecar and a deployment-specific SDF shim. TrustDB does
not compile against a vendor SDF header, link a proprietary library, or assume
that two products use identical structures, algorithm identifiers, error
codes, symbol visibility, or authentication extensions.

A deployment builds its own shared adapter library against the purchased
device SDK and exports `trustdb_sdf_adapter_get_api_v1`. The returned function
table must implement the exact contract in
[`trustdb_sdf_adapter_v1.h`](trustdb_sdf_adapter_v1.h).

The adapter is responsible for:

- opening the selected vendor library/device from the bounded configuration
  bytes and returning a stable device identity;
- mapping every vendor status to the small normalized TrustDB status set;
- exporting an internal SM2 public key as a canonical 65-byte uncompressed
  `sm2p256v1` point;
- signing exactly one 32-byte SM3 digest with the indexed internal SM2 key and
  returning canonical-width `r || s`;
- generating/importing SM4 session keys under an indexed internal KEK without
  exporting plaintext key bytes;
- using only opaque, same-session key handles for SM4-CBC and MAC operations;
- destroying every session key and tolerating abrupt sidecar termination.

The sidecar computes the fixed SM2 `ZA || message` SM3 digest from the public
key and TrustDB's immutable user ID before calling the adapter. A vendor shim
must not hash the digest again or apply a different user ID.

Configuration, credential, input, output, and IV pointers are borrowed for one
call only. The adapter must not retain them. Raw vendor diagnostics, device
references, key indexes, credentials, and handles must never be returned or
logged.

The ABI is vendor-neutral plumbing, not a qualification claim. A product is
supported only after the gated smoke procedure in
`docs/integrations/SDF_SIGNER.md` has passed for the exact module, firmware,
operating system, CPU architecture, and algorithm profile.
