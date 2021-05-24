# Changelog

This document outlines major changes between releases.

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
