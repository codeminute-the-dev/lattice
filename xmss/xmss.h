#ifndef XMSS_LIB_H
#define XMSS_LIB_H

// In bytes.
#define PRIVATE_SEED_LEN 64
#define PUBLIC_SEED_LEN 32
#define PK_LEN 64
#define SK_LEN 128
#define MSG_LEN 32
#define SIGNATURE_LEN 2340
#define MAX_SIGNS 32

typedef unsigned char uint8_t;

#ifdef __cplusplus
extern "C" {
#endif

// private_seed must remain secret and public_seed must not reveal private
// information. deterministic: need not store sk if seeds will be available in
// the future. Return value: 0 means success.
int xmss_keygen(uint8_t private_seed[PRIVATE_SEED_LEN],
                uint8_t public_seed[PUBLIC_SEED_LEN], uint8_t out_pk[PK_LEN],
                uint8_t out_sk[SK_LEN]);

// May sign only once with each msg_uid: 0 <= msg_uid < MAX_SIGNS.
// Publishing two signatures with the same msg_uid enables attackers to sign
// unintended messages in the name of the private_seed owner.
// Return value: 0 means success.
int xmss_sign(unsigned int msg_uid, uint8_t sk[SK_LEN], uint8_t msg[MSG_LEN],
              uint8_t out_signature[SIGNATURE_LEN]);

// Return value: 0 if signature authorizes the message. Other value otherwise.
int xmss_verify(uint8_t pk[PK_LEN], uint8_t msg[MSG_LEN],
                uint8_t signature[SIGNATURE_LEN]);

#ifdef __cplusplus
}
#endif

#endif // XMSS_LIB_H