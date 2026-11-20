# Vendored Preact runtime

Browser-native ES modules committed so counterslop stays a single Go binary with
no JS build step. Code imports `preact`, `preact/hooks` and `htm` through the
import map in `web/index.html`.

| Package | Version |
| --- | --- |
| `preact` (+ `preact/hooks`) | 10.29.7 |
| `htm` | 3.1.1 |

Preact is MIT-licensed, HTM is Apache-2.0; license texts are alongside.
