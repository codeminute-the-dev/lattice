#ifndef XMSS_CORE_H
#define XMSS_CORE_H

#include "params.h"

/**
 * Given a set of parameters, this function returns the size of the secret key.
 * This is implementation specific, as varying choices in tree traversal will
 * result in varying requirements for state storage.
 */
unsigned long long xmss_core_sk_bytes(const xmss_params *params);

/*
 * Derives a XMSS key pair for a given parameter set.
 * private_seed must be 2*n long (SK_SEED || SK_PRF).
 * public_seed must be n long (PUB_SEED), assumed to be unpredictable.
 * Format sk: [(ceil(h/8) bit) index || SK_SEED || SK_PRF || root || PUB_SEED]
 * Format pk: [root || PUB_SEED] omitting algorithm OID.
 */
int xmss_core_seed_keypair(const xmss_params *params,
                           unsigned char *pk, unsigned char *sk,
                           unsigned char *private_seed,
                           unsigned char *public_seed);

/**
 * Signs a message. Returns an array containing the signature followed by the
 * message and an updated secret key.
 */
int xmss_core_sign(const xmss_params *params,
                   unsigned char *sk,
                   unsigned char *sm, unsigned long long *smlen,
                   const unsigned char *m, unsigned long long mlen);

/**
 * Verifies a given message signature pair under a given public key.
 * Note that this assumes a pk without an OID, i.e. [root || PUB_SEED]
 */
int xmss_core_sign_open(const xmss_params *params,
                        unsigned char *m, unsigned long long *mlen,
                        const unsigned char *sm, unsigned long long smlen,
                        const unsigned char *pk);

#endif
