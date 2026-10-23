# Fedora COPR "make srpm" source method. COPR runs, as root in a Fedora chroot with network,
#   make -f <clone>/.copr/Makefile srpm outdir=<dir> spec=<spec>
# and builds the x86_64 and aarch64 rpms from the source rpm left in outdir.
# The release is the v* tag checked out, or version=X.Y.Z; see packaging/rpm/README.md.
top := $(abspath $(dir $(lastword $(MAKEFILE_LIST)))..)
outdir ?= $(top)/dist/rpm

.PHONY: srpm
srpm:
	command -v curl >/dev/null && command -v git >/dev/null && command -v rpmbuild >/dev/null || dnf -y install curl git rpm-build
	cd '$(top)' && v='$(version)' && \
	{ [ -n "$$v" ] || v=$$(git -c safe.directory='*' describe --tags --exact-match --match 'v[0-9]*' HEAD | sed 's/^v//'); } && \
	{ [ -n "$$v" ] || { echo "HEAD is not a v* release tag: pass version=X.Y.Z" >&2; exit 2; }; } && \
	sh scripts/package-srpm.sh "$$v" '$(outdir)'
