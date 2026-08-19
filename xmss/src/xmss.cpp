#include <stdint.h>
#include <string.h>

#include "../xmss.h"

extern "C" {
#include "../external/params.h"
#include "../external/utils.h"
#include "../external/xmss_core.h"
}

// config: XMSS-SHAKE256_5_256 (not in RFC 8391)
// around (slightly less than) 256 security bits.
static constexpr xmss_params default_params = {
    .func = XMSS_SHAKE256,
    .n = 32,                   // hash input & output length
    .padding_len = 32,         // bytes reserved for domain separation
    .wots_w = 16,              // Winternitz parameter
    .wots_log_w = 4,           // log2(wots_w)
    .wots_len1 = 64,           // 8 * n / wots_log_w,
    .wots_len2 = 3,            // floor(log(len_1 * (w - 1)) / wots_log_w) + 1,
    .wots_len = 67,            // len1 + len2,
    .wots_sig_bytes = 67 * 32, // wots_len * n,
    .full_height = 5,          // allows 32 signatures
    .tree_height = 5,          // full_height / d,
    .d = 1,                    // no MT recursion
    .index_bytes = 4,          // msg_uid serialization
    .sig_bytes = 2340, // wots_sig_bytes + index_bytes + (height + 1) * n,
    .pk_bytes = 64,    // 2 * n
    .sk_bytes = 132,   // 4 * n + index_bytes
};

int xmss_keygen(uint8_t private_seed[PRIVATE_SEED_LEN],
                uint8_t public_seed[PUBLIC_SEED_LEN], uint8_t out_pk[PK_LEN],
                uint8_t out_sk[SK_LEN]) {
  static_assert(PRIVATE_SEED_LEN + PUBLIC_SEED_LEN == 3 * default_params.n,
                "seed lengths must match 3*n");
  static_assert(SK_LEN == default_params.sk_bytes - default_params.index_bytes,
                "SK_LEN must equal sk_bytes - index_bytes");
  static_assert(PK_LEN == default_params.pk_bytes,
                "PK_LEN must equal pk_bytes");
  static_assert(SIGNATURE_LEN == default_params.sig_bytes,
                "SIGNATURE_LEN must equal sig_bytes");

  uint8_t sk_with_index[default_params.sk_bytes];
  int ret = xmss_core_seed_keypair(&default_params, out_pk, sk_with_index,
                                   private_seed, public_seed);
  memcpy(out_sk, sk_with_index + default_params.index_bytes, SK_LEN);

  return ret;
}

int xmss_sign(unsigned int msg_uid, uint8_t sk[SK_LEN], uint8_t msg[MSG_LEN],
              uint8_t out_signature[SIGNATURE_LEN]) {
  if (msg_uid >= MAX_SIGNS) {
    return -1;
  }

  // build full sk with index
  uint8_t full_sk[default_params.sk_bytes];
  ull_to_bytes(full_sk, default_params.index_bytes, msg_uid);
  memcpy(full_sk + default_params.index_bytes, sk, SK_LEN);

  // xmss_core_sign outputs: signature || message
  uint8_t sm[SIGNATURE_LEN + MSG_LEN];
  unsigned long long smlen;

  int ret = xmss_core_sign(&default_params, full_sk, sm, &smlen, msg, MSG_LEN);

  memcpy(out_signature, sm, SIGNATURE_LEN);

  return ret;
}

int xmss_verify(uint8_t pk[PK_LEN], uint8_t msg[MSG_LEN],
                uint8_t signature[SIGNATURE_LEN]) {
  // xmss_core_sign_open expects sm = signature || message
  // msg_uid stored in first 4 bytes of signature
  uint8_t sm[SIGNATURE_LEN + MSG_LEN];
  memcpy(sm, signature, SIGNATURE_LEN);
  memcpy(sm + SIGNATURE_LEN, msg, MSG_LEN);

  uint8_t m_out[SIGNATURE_LEN + MSG_LEN];
  unsigned long long mlen;

  return xmss_core_sign_open(&default_params, m_out, &mlen, sm,
                             SIGNATURE_LEN + MSG_LEN, pk);
}
