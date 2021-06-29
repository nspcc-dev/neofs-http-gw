# Changelog

This document outlines major changes between releases.

## 0.16.0 (29 Jun 2021)

We update HTTP gateway with NEP-6 wallets support, YAML configuration files
and small fixes.

New features:
 * YAML configuration file (#71)

Behavior changes:
 * gateway key needs to be stored in a proper NEP-6 wallet now, `-k` option is
   no longer available, see `-w` and `-a` (#68)

Bugs fixed:
 * downloads were not streamed leading to excessive memory usage (#67)
 * Last-Modified header incorrectly used local time (#75)

## 0.15.2 (22 Jun 2021)

New features:
 * Content-Type returned for object GET requests can now be taken from
   attributes (overriding autodetection, #65)

Behavior changes:
 * grpc keepalive options can no longer be changed (#60)

Improvements:
 * code refactoring (more reuse between different gateways, moved some code to
   sdk-go, #47, #46, #51, #62, #63)
 * documentation updates and fixes (#53, #49, #55, #59)
 * updated api-go dependency (#57)

Bugs fixed:
 * `-k` option wasn't accepted for key although it was documented (#50)

## 0.15.1 (24 May 2021)

This important release makes HTTP gateway compatible with NeoFS node version
0.20.0.

Behavior changes:
 * neofs-api-go was updated to 1.26.1, which contains some incompatible
   changes in underlying components (#39, #44)
 * `neofs-http-gw` is consistently used now for repository, binary and image
   names (#43)

Improvements:
 * minor code cleanups based on stricter set of linters (#41)
 * updated README (#42)

## 0.15.0 (30 Apr 2021)

This is the first public release incorporating latest NeoFS protocol support
and fixing some bugs.

New features:
 * upload support (#14, #13, #29)
 * ephemeral keys (#26)
 * TLS server support (#28)

Behavior changes:
 * node weights can now be specified as simple numbers instead of percentages
   and gateway will calculate the proportion automatically (#27)
 * attributes are converted now to `X-Attribute-*` headers when retrieving
   object from gate instead of `X-*` (#29)

Improvements:
 * better Makefile (#16, #24, #33, #34)
 * updated documentation (#16, #29, #35, #36)
 * updated neofs-api-go to v1.25.0 (#17, #20)
 * updated fasthttp to v1.23.0+ (#17, #29)
 * refactoring, eliminating some dependencies (#20, #29)

Bugs fixed:
 * gateway attempted to work with no NeoFS peers configured (#29)
 * some invalid headers could be sent for attributes using non-ASCII or
   non-printable characters (#29)

## Older versions

Please refer to [Github
releases](https://github.com/nspcc-dev/neofs-http-gw/releases/) for older
releases.
