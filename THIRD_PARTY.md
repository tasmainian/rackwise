# Third-party material

Everything reused in Rackwise is listed here with its licence. Package manager dependencies are listed in each service's own manifest (`engine/go.mod`, `coach/pyproject.toml`, `web/package.json`). None is GPL or AGPL. The engine's runtime dependencies are pgx and its helpers (MIT) and `golang.org/x` packages (BSD-3-Clause). The coach directly uses `anthropic` (MIT), `mcp` (MIT), `httpx` (BSD-3-Clause), `model2vec` (MIT), `numpy` (BSD-3-Clause), `pg8000` (BSD-3-Clause) and `pgvector` (MIT). Through `model2vec` it also installs `huggingface_hub`, `tokenizers` and `safetensors` (Apache-2.0), `joblib` (BSD-3-Clause) and `tqdm`. `tqdm` is MPL-2.0 and MIT. MPL-2.0 is a weak, file-level copyleft that only applies if `tqdm`'s own files are changed, and they aren't. `psycopg` was avoided because it's LGPL.

| Material | Licence | Used for | Status |
|---|---|---|---|
| [ENABLE word list](https://sources.debian.org/src/scowl/6-2/r/enable/README) | Public domain, according to its README | The game's dictionary, `engine/data/enable1.txt` | Copied from the [dolph/dictionary](https://github.com/dolph/dictionary) mirror. SHA-256 `3f16130220645692ed49c7134e24a18504c2ca55b3c012f7290e3e77c63b1a89`. That repo's other lists, including an official dictionary, are not used. |
| [SCOWL](http://wordlist.aspell.net/) 2020.12.07 | Permissive, see `engine/data/SCOWL-Copyright.txt` | Size levels 10, 20 and 35, intersected with ENABLE by `cmd/gencommon`, make `engine/data/common.txt`, the easy bot's word list | Source archive SHA-256 `5587667caa20c4891390c2d42dbb4d5c4c3f41bee77af1457ece3ba23fb859cc` |
| [ably-labs/word-game](https://github.com/ably-labs/word-game) | Apache-2.0 | Board, tile and blank-letter dialog, rewritten in TypeScript in `web/src/components/` | Each adapted file says so and lists what changed. Its licence and copyright notice are in `third_party/ably-word-game/`. |
| [minishlab/potion-base-8M](https://huggingface.co/minishlab/potion-base-8M) | MIT | The default embedding model for strategy notes. It's downloaded into the coach image at build time and isn't stored in the repo. | Set with `EMBEDDING_MODEL`. |

`engine/data/additions.txt` is written by hand for this project and isn't copied from any list. The strategy notes in `notes/` were also written for this project and aren't copied from any guide.

## External services

| Service | Terms | Used for | Status |
|---|---|---|---|
| [word.tips](https://word.tips/terms/) | Its terms grant no rights to its content and don't mention automated access. `robots.txt` disallows the path used. | The `lookup_words` practice tool and position reviews, with one request per lookup and nothing stored | Not part of the Rackwise game. `coach/tests/fixtures/` holds two small recorded responses. See `docs/decisions/0007-word-additions.md`. |

## Reviewed and not used

| Project | Licence | Why not |
|---|---|---|
| [Macondo](https://github.com/domino14/macondo) | GPL-3.0 | Copyleft and large. See `docs/decisions/0001-own-engine.md`. |
| [woogles-io/liwords](https://github.com/woogles-io/liwords) | AGPL-3.0 | Copyleft, including for hosted versions, and very large |
| [edlerd/scrabble](https://github.com/edlerd/scrabble) | Apache-2.0 | No React, and its bundled word list looks like an official dictionary |
| [first20hours/google-10000-english](https://github.com/first20hours/google-10000-english) | Educational, personal and research use only | Too restrictive for a published project |
