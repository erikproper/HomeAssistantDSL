---
name: project-haos-encrypted-backup-decrypt-procedure
description: "How to decrypt a password-protected HAOS Supervisor backup (securetar format) to recover core.entity_registry or other .storage files, when ha CLI/supervisor API access isn't available."
metadata: 
  node_type: memory
  type: reference
  originSessionId: 02e0cbb6-56e4-4de4-b032-b8a76cfdca1f
  modified: 2026-09-14T19:46:02.323Z
---

Used live 2026-09-14 to recover Vienna's `core.entity_registry` after a coordinator bug wiped
several entities' manual renames (see [[project_retiremissing_passthrough_wipe_incident]]).

**When `ha` CLI/supervisor API access isn't available** (no supervisor token on the SSH session —
common, see [[project_ha_entity_registry_collision_lessons]] on HAOS's restricted default shell),
a HAOS backup tar (`/backup/*.tar` on the instance itself) can still be decrypted directly with the
official `securetar` Python package, given the backup's own encryption key (HA's "emergency kit"
code, format `XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX`).

**Procedure:**
1. Check `backup.json` inside the outer tar (`tar -xf backup.tar backup.json`) for `"protected"` —
   if true, the per-component members (`homeassistant.tar.gz`, etc.) are individually encrypted;
   `tar -tzf` on one directly fails with "invalid magic" as confirmation.
2. Extract just the needed member from the outer (unencrypted) tar: `tar -xf backup.tar
   homeassistant.tar.gz` — this file itself is still encrypted.
3. Locally (needs internet access, so not on the HAOS box itself): `python3 -m venv venv &&
   ./venv/bin/pip install securetar`.
4. Decrypt with the real API (a third-party "v3 decryptor" script found via search uses the same
   shape, confirmed working against a 2026.09 supervisor backup — Argon2id/XChaCha20 v3 cipher,
   handled transparently by the library):
   ```python
   from pathlib import Path
   from securetar import SecureTarFile
   with SecureTarFile(name=Path("homeassistant.tar.gz"), password="XXXX-XXXX-...") as tar:
       tar.extractall(path=Path("extracted"))
   ```
   `core.entity_registry` etc. land under `extracted/data/.storage/`.
5. **To extract just ONE inner file instead of the whole ~260MB archive**: the encrypted stream is
   NOT seekable (`tarfile.StreamError: seeking backwards is not allowed` if you try
   `getmember()`+`extractfile()` after the fact) — iterate instead: `for member in tar: if
   member.name == target: tar.extractfile(member).read(); break`.
6. Treat the decrypted registry as a point-in-time snapshot to diff against the live one (match on
   `(platform, unique_id)`, not `id` or `entity_id` alone) — restoring entity_id renames back onto
   the live registry needs the same stop-core/edit/start-core discipline as any other direct
   registry edit ([[project_ha_entity_registry_collision_lessons]]).
7. **Clean up afterward**: delete the decrypted registry, the encrypted tar copy, and the venv —
   they contain the full HA config including secrets. Never leave the encryption key or decrypted
   `.storage` content sitting in scratch files longer than the task needs.
