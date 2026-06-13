# bashfs

Embed a simple filesystem into your bash scripts, to allow single-file bash script distribution.

## Install

```bash
go install bashfs@latest
```

Or download a binary from [releases](https://github.com/wow-look-at-my/bashfs/releases).

## Usage

### Development: `bashfs gen`

During development, use `eval` to load helper functions that read from real files on disk:

```bash
#!/bin/bash
eval "$(bashfs gen ./myfiles)"

bashfs_list                      # list all files
bashfs_cat config.json           # print file contents to stdout
bashfs_cat config.json | jq .port  # use jq on a file
bashfs_extract template.txt /tmp/out.txt  # extract to a file
```

### Distribution: `bashfs package`

When ready to distribute, package everything into a single self-contained script. The packaged script is written to stdout - redirect it to a file:

```bash
bashfs package myscript.sh > dist/myscript.sh
chmod +x dist/myscript.sh
```

This finds the `eval "$(bashfs gen <dir>)"` line in your script (the unquoted `eval $(bashfs gen <dir>)` form is also accepted) and replaces it with embedded helper functions, then appends gzip-compressed file data after an `exit 0` guard. The output script has no external file dependencies.

#### `--encoding raw` (default) vs `--encoding base64`

The trailing payload can be appended in two encodings:

| Encoding | Size | Use when |
|---|---|---|
| `raw` (default) | smallest - raw gzip bytes, zero encoding overhead | shipping a binary file (download, release artifact, `scp`, registry) |
| `base64` | ~33% larger - concatenated per-file base64 chunks, pure printable ASCII | the script needs to survive a copy-paste through a text-only channel: chat clients, web forms, code review comments, JIRA, sticky notes, anywhere that strips or mangles non-printable bytes |

```bash
bashfs package myscript.sh --encoding base64 > dist/myscript.sh
```

Raw mode refuses to write to a terminal (it's binary, would trash your cursor). Always redirect, or pick `--encoding base64` if you want to inspect the output directly.

Both encodings produce a single self-contained script that runs identically - the `bashfs_*` helpers transparently decode whichever payload is at the end. `curl ... | bash` piping works in both modes.

#### Pre-packaging validation

Before compressing, `bashfs package` validates all shell scripts (`.sh`, `.bash`, or files with a `#!/bin/bash` shebang) in the embedded filesystem:

- **`bash -n` syntax check** on every shell file and the main script -- catches syntax errors before they're baked into the payload
- **shellcheck** (if installed) -- advisory warnings printed to stderr, does not block packaging
- **Source resolution** -- detects `source`/`.` commands within embedded shell files that reference missing files, and flags `source` commands in the main script that point into the bashfs directory (use `source <(bashfs_cat path)` instead)

To skip validation:

```bash
bashfs package myscript.sh --no-validate > dist/myscript.sh
```

#### Profiling mode

`bashfs package` can bake in an opt-in profiling mode that benchmarks the parts
of the packaged script that bashfs owns -- the startup integrity check (a
SHA-256 over the whole payload, paid on every run before any user code) and the
extraction of each embedded file -- using [hyperfine](https://github.com/sharkdp/hyperfine).

It is inert on a normal run. To profile, set `BASHFS_PROFILE_SCRIPT=1`:

```bash
BASHFS_PROFILE_SCRIPT=1 ./dist/myscript.sh
```

The script then benchmarks each operation with hyperfine, prints a comparison
table, and exits **before** running its own body -- so profiling never kicks off
your script's real work. It assumes `hyperfine` is installed on the machine
running the script.

Choose how profiling support is embedded with `--profiling-support`:

| Mode | Size impact | Runtime requirement | Use when |
|---|---|---|---|
| `web` (default) | tiny stub (~0.7 KB) | downloads the harness over HTTPS the first time you profile | smallest script; profiling machine has network access |
| `local` | embeds the full harness (~4 KB) | none | air-gapped / offline machines |
| `none` | nothing | n/a | you never want profiling support baked in |

```bash
# Embed the harness so profiling works with no network access:
bashfs package myscript.sh --profiling-support local > dist/myscript.sh
```

In `web` mode only a small stub is embedded; when profiling is triggered it
`curl`s the harness from the public repo and runs it, keeping the packaged
script tiny.

Optional runtime knobs:

| Variable | Effect |
|---|---|
| `BASHFS_PROFILE_SCRIPT=1` | enable profiling mode |
| `BASHFS_PROFILE_WARMUP=N` | hyperfine warmup runs (default 3) |
| `BASHFS_PROFILE_RUNS=N` | fix the number of timed runs |
| `BASHFS_PROFILE_JSON=path` | also export raw results as JSON to `path` |

## Generated Functions

| Function | Description |
|---|---|
| `bashfs_cat <path>` | Print file contents to stdout |
| `bashfs_extract <path> <dest>` | Extract a file to the given destination |
| `bashfs_list` | List all embedded file paths |

## How It Works

1. Files are recursively collected from the specified directory
2. Each file is gzip-compressed
3. With `--encoding raw` (default), the compressed bytes are concatenated and appended as the trailing payload. With `--encoding base64`, each file's compressed bytes are individually base64-encoded and the per-file chunks are concatenated as the trailing payload - every chunk is self-contained valid base64, so the runtime can slice and decode one file's chunk without touching the rest.
4. An `exit 0` guard separates the script body from the trailing payload
5. File offsets and lengths are stored in a bash associative array (`declare -A`, requires bash 4+) - offsets index the trailing payload byte-stream regardless of encoding
6. A SHA-256 checksum of the payload is embedded in the script and verified at load time, catching corruption or truncation before any user code runs
7. Helper functions use `tail -c` + `head -c` to extract a file's chunk, then pipe through `base64 -d` (base64 mode only) and `gzip -d`

## Example

**myscript.sh** (development version):
```bash
#!/bin/bash
eval "$(bashfs gen ./assets)"
echo "Available files:"
bashfs_list
echo "Config:"
bashfs_cat config.json | jq .database.host
```

**Package it:**
```bash
bashfs package myscript.sh -o dist/myscript.sh
```

**dist/myscript.sh** is now a single file with all assets embedded. Ship it anywhere.
