# Parked workflows

These came from Pearl and cannot run here yet: they need self-hosted GPU
runners, signing certificates, container registry credentials, or release
infrastructure that this fork does not have. They are kept rather than deleted
so they can be revived once the corresponding infrastructure exists.

To re-enable one, move it back into `.github/workflows/` and supply the secrets
it references.
