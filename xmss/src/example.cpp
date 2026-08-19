#include <cstdio>
#include <cstring>
#include "../xmss.h"

int main() {
    // Seeds (in practice, derive from HD wallet / BIP39)
    uint8_t private_seed[PRIVATE_SEED_LEN];
    uint8_t public_seed[PUBLIC_SEED_LEN];
    for (int i = 0; i < PRIVATE_SEED_LEN; i++) private_seed[i] = i;
    for (int i = 0; i < PUBLIC_SEED_LEN; i++) public_seed[i] = i + 64;

    // Generate keypair
    uint8_t pk[PK_LEN];
    uint8_t sk[SK_LEN];
    if (xmss_keygen(private_seed, public_seed, pk, sk) != 0) {
        printf("keygen failed\n");
        return 1;
    }
    printf("Keypair generated.\n");

    // Message to sign (32-byte hash)
    uint8_t msg[MSG_LEN];
    memset(msg, 0x42, MSG_LEN);

    // Sign with msg_uid=0 (first signature)
    uint8_t sig[SIGNATURE_LEN];
    if (xmss_sign(0, sk, msg, sig) != 0) {
        printf("sign failed\n");
        return 1;
    }
    printf("Signed with msg_uid=0.\n");

    // Verify
    if (xmss_verify(pk, msg, sig) != 0) {
        printf("verify failed\n");
        return 1;
    }
    printf("Verification passed!\n");

    // Tamper and verify again
    sig[100] ^= 1;
    if (xmss_verify(pk, msg, sig) == 0) {
        printf("ERROR: tampered sig accepted!\n");
        return 1;
    }
    printf("Tampered signature correctly rejected.\n");

    return 0;
}
