#include <stdint.h>
#include <string.h>

#include "params.h"
#include "xmss_core.h"

int xmss_str_to_oid(uint32_t *oid, const char *s)
{
    if (!strcmp(s, "XMSS-SHA2_10_256")) {
        *oid = 0x00000001;
    }
    else if (!strcmp(s, "XMSS-SHA2_16_256")) {
        *oid = 0x00000002;
    }
    else if (!strcmp(s, "XMSS-SHA2_20_256")) {
        *oid = 0x00000003;
    }
    else if (!strcmp(s, "XMSS-SHA2_10_512")) {
        *oid = 0x00000004;
    }
    else if (!strcmp(s, "XMSS-SHA2_16_512")) {
        *oid = 0x00000005;
    }
    else if (!strcmp(s, "XMSS-SHA2_20_512")) {
        *oid = 0x00000006;
    }
    else if (!strcmp(s, "XMSS-SHAKE_10_256")) {
        *oid = 0x00000007;
    }
    else if (!strcmp(s, "XMSS-SHAKE_16_256")) {
        *oid = 0x00000008;
    }
    else if (!strcmp(s, "XMSS-SHAKE_20_256")) {
        *oid = 0x00000009;
    }
    else if (!strcmp(s, "XMSS-SHAKE_10_512")) {
        *oid = 0x0000000a;
    }
    else if (!strcmp(s, "XMSS-SHAKE_16_512")) {
        *oid = 0x0000000b;
    }
    else if (!strcmp(s, "XMSS-SHAKE_20_512")) {
        *oid = 0x0000000c;
    }
    else if (!strcmp(s, "XMSS-SHA2_10_192")) {
        *oid = 0x0000000d;
    }
    else if (!strcmp(s, "XMSS-SHA2_16_192")) {
        *oid = 0x0000000e;
    }
    else if (!strcmp(s, "XMSS-SHA2_20_192")) {
        *oid = 0x0000000f;
    }
    else if (!strcmp(s, "XMSS-SHAKE256_10_256")) {
        *oid = 0x00000010;
    }
    else if (!strcmp(s, "XMSS-SHAKE256_16_256")) {
        *oid = 0x00000011;
    }
    else if (!strcmp(s, "XMSS-SHAKE256_20_256")) {
        *oid = 0x00000012;
    }
    else if (!strcmp(s, "XMSS-SHAKE256_10_192")) {
        *oid = 0x00000013;
    }
    else if (!strcmp(s, "XMSS-SHAKE256_16_192")) {
        *oid = 0x00000014;
    }
    else if (!strcmp(s, "XMSS-SHAKE256_20_192")) {
        *oid = 0x00000015;
    }
    else {
        return -1;
    }
    return 0;
}

int xmss_parse_oid(xmss_params *params, const uint32_t oid)
{
    switch (oid) {
        case 0x00000001:
        case 0x00000002:
        case 0x00000003:
        case 0x00000004:
        case 0x00000005:
        case 0x00000006:

        case 0x0000000d:
        case 0x0000000e:
        case 0x0000000f:
            params->func = XMSS_SHA2;
            break;

        case 0x00000007:
        case 0x00000008:
        case 0x00000009:
            params->func = XMSS_SHAKE128;
            break;

        case 0x0000000a:
        case 0x0000000b:
        case 0x0000000c:

        case 0x00000010:
        case 0x00000011:
        case 0x00000012:
        case 0x00000013:
        case 0x00000014:
        case 0x00000015:
            params->func = XMSS_SHAKE256;
            break;

        default:
            return -1;
    }
    switch (oid) {
        case 0x0000000d:
        case 0x0000000e:
        case 0x0000000f:

        case 0x00000013:
        case 0x00000014:
        case 0x00000015:
            params->n = 24;
            params->padding_len = 4;
            break;

        case 0x00000001:
        case 0x00000002:
        case 0x00000003:

        case 0x00000007:
        case 0x00000008:
        case 0x00000009:

        case 0x00000010:
        case 0x00000011:
        case 0x00000012:
            params->n = 32;
            params->padding_len = 32;
            break;

        case 0x00000004:
        case 0x00000005:
        case 0x00000006:

        case 0x0000000a:
        case 0x0000000b:
        case 0x0000000c:
            params->n = 64;
            params->padding_len = 64;
            break;

        default:
            return -1;
    }
    switch (oid) {
        case 0x00000001:
        case 0x00000004:
        case 0x00000007:
        case 0x0000000a:
        case 0x0000000d:
        case 0x00000010:
        case 0x00000013:
            params->full_height = 10;
            break;

        case 0x00000002:
        case 0x00000005:
        case 0x00000008:
        case 0x0000000b:
        case 0x0000000e:
        case 0x00000011:
        case 0x00000014:
            params->full_height = 16;
            break;

        case 0x00000003:
        case 0x00000006:
        case 0x00000009:
        case 0x0000000c:
        case 0x0000000f:
        case 0x00000012:
        case 0x00000015:
            params->full_height = 20;

            break;
        default:
            return -1;
    }

    params->d = 1;
    params->wots_w = 16;

    return xmss_xmssmt_initialize_params(params);
}

/**
 * Given a params struct where the following properties have been initialized;
 *  - full_height; the height of the complete (hyper)tree
 *  - n; the number of bytes of hash function output
 *  - d; the number of layers (d > 1 implies XMSSMT)
 *  - func; one of {XMSS_SHA2, XMSS_SHAKE128, XMSS_SHAKE256}
 *  - wots_w; the Winternitz parameter
 * this function initializes the remainder of the params structure.
 */
int xmss_xmssmt_initialize_params(xmss_params *params)
{
    params->tree_height = params->full_height  / params->d;
    if (params->wots_w == 4) {
        params->wots_log_w = 2;
        params->wots_len1 = 8 * params->n / params->wots_log_w;
        /* len_2 = floor(log(len_1 * (w - 1)) / log(w)) + 1 */
        params->wots_len2 = 5;
    }
    else if (params->wots_w == 16) {
        params->wots_log_w = 4;
        params->wots_len1 = 8 * params->n / params->wots_log_w;
        /* len_2 = floor(log(len_1 * (w - 1)) / log(w)) + 1 */
        params->wots_len2 = 3;
    }
    else if (params->wots_w == 256) {
        params->wots_log_w = 8;
        params->wots_len1 = 8 * params->n / params->wots_log_w;
        /* len_2 = floor(log(len_1 * (w - 1)) / log(w)) + 1 */
        params->wots_len2 = 2;
    }
    else {
        return -1;
    }
    params->wots_len = params->wots_len1 + params->wots_len2;
    params->wots_sig_bytes = params->wots_len * params->n;

    if (params->d == 1) {  // Assume this is XMSS, not XMSS^MT
        /* In XMSS, always use fixed 4 bytes for index_bytes */
        params->index_bytes = 4;
    }
    else {
        /* In XMSS^MT, round index_bytes up to nearest byte. */
        params->index_bytes = (params->full_height + 7) / 8;
    }
    params->sig_bytes = (params->index_bytes + params->n
                         + params->d * params->wots_sig_bytes
                         + params->full_height * params->n);

    params->pk_bytes = 2 * params->n;
    params->sk_bytes = xmss_core_sk_bytes(params);

    return 0;
}
