# AUR packaging

Tide ships to the AUR as **`tide-bin`**, which repackages the release tarball
published by `.github/workflows/release.yml`.

## Why a `-bin` package

The release workflow already builds and publishes static, CGO-free binaries for
linux/amd64 and linux/arm64. Repackaging them means the AUR package is the exact
artifact that was tested and released, and it installs in seconds without a Go
toolchain. A from-source package would also be viable here — Tide embeds no
build-time credentials — so one can be added later if there is demand.

## Files

| File | Purpose |
| --- | --- |
| `PKGBUILD.in` | The template. Edit this, never a rendered `PKGBUILD`. |
| `render-pkgbuild.sh` | Substitutes version, `pkgrel`, and checksums into the template. |

Placeholders: `@PKGVER@`, `@PKGREL@`, `@SHA256_X86_64@`, `@SHA256_AARCH64@`,
`@SHA256_LICENSE@`. Rendering fails if any placeholder survives, so a typo can
never publish a PKGBUILD that cannot build.

## Publishing

Use `./deploy.sh` → **AUR → Publish tide-bin**. It:

1. Confirms the GitHub release has both Linux tarballs and `checksums.txt`.
2. Reads the digests from that published `checksums.txt`, never from a local
   build, so the PKGBUILD always describes the bytes users actually download.
3. Hashes the `LICENSE` blob at the tag, which is what the PKGBUILD's
   `raw.githubusercontent.com` source URL serves.
4. Clones the AUR repo and picks `pkgrel`: `1` for a new `pkgver`, otherwise
   one more than what is published.
5. Renders the PKGBUILD, generates `.SRCINFO` with `makepkg --printsrcinfo`,
   shows the diff, and pushes only after you confirm.

Run it with `./deploy.sh --dry-run` first to see the exact PKGBUILD without
pushing anything.

The AUR serves an empty git repository for any package name, so a successful
clone does not mean the package exists — only a committed `PKGBUILD` does.

## First-time setup

Register an AUR account and add your SSH public key under **My Account → SSH
Public Key**, then:

```
Host aur.archlinux.org
  User aur
  IdentityFile ~/.ssh/id_ed25519
```

`./deploy.sh` shows `aur ✓ <username>` in its status bar once the key is
accepted, and **AUR → AUR setup help** prints these steps with your key.

## Rendering by hand

```bash
bash packaging/aur/render-pkgbuild.sh \
  --version v1.2.0 --pkgrel 1 \
  --sha256-x86_64 <hash> --sha256-aarch64 <hash> --sha256-license <hash> \
  --output /tmp/aur/PKGBUILD
cd /tmp/aur && makepkg --printsrcinfo > .SRCINFO && makepkg -f
```
