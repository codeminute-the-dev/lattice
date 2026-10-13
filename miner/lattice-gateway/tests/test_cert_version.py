import pytest

from lattice_gateway.blockchain_utils.zk_certificate import CertificateVersion


@pytest.mark.parametrize(
    "version,salted,lattice",
    [
        (CertificateVersion.ZK_DENSE, False, False),
        (CertificateVersion.ZK_MOE, False, False),
        (CertificateVersion.ZK_V3, True, False),
        (CertificateVersion.ZK_V4, True, True),
    ],
)
def test_seed_derivation_flags(version, salted, lattice):
    assert version.uses_salted_seeds is salted
    assert version.uses_lattice_salts is lattice


def test_v4_wire_number_matches_node():
    # Go wire.CertificateVersion and Rust plain_proof::CertificateVersion use 4.
    assert int(CertificateVersion.ZK_V4) == 4
    assert CertificateVersion(4) is CertificateVersion.ZK_V4
