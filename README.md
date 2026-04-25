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
bashfs_jq config.json .port     # run jq on a file
bashfs_extract template.txt /tmp/out.txt  # extract to a file
```

### Distribution: `bashfs package`

When ready to distribute, package everything into a single self-contained script. The packaged script is written to stdout — redirect it to a file:

```bash
bashfs package myscript.sh > dist/myscript.sh
chmod +x dist/myscript.sh
```

This finds the `eval $(bashfs gen <dir>)` line in your script and replaces it with embedded helper functions, then appends gzip-compressed file data after an `exit 0` guard. The output script has no external file dependencies.

#### `--encoding raw` (default) vs `--encoding base64`

The trailing payload can be appended in two encodings:

| Encoding | Size | Use when |
|---|---|---|
| `raw` (default) | smallest — raw gzip bytes, zero encoding overhead | shipping a binary file (download, release artifact, `scp`, registry) |
| `base64` | ~33% larger — concatenated per-file base64 chunks, pure printable ASCII | the script needs to survive a copy-paste through a text-only channel: chat clients, web forms, code review comments, JIRA, sticky notes, anywhere that strips or mangles non-printable bytes |

```bash
bashfs package myscript.sh --encoding base64 > dist/myscript.sh
```

Raw mode refuses to write to a terminal (it's binary, would trash your cursor). Always redirect, or pick `--encoding base64` if you want to inspect the output directly.

Both encodings produce a single self-contained script that runs identically — the `bashfs_*` helpers transparently decode whichever payload is at the end. `curl … | bash` piping works in both modes.

## Generated Functions

| Function | Description |
|---|---|
| `bashfs_cat <path>` | Print file contents to stdout |
| `bashfs_extract <path> <dest>` | Extract a file to the given destination |
| `bashfs_list` | List all embedded file paths |
| `bashfs_jq <path> [filter]` | Run jq on a file (filter defaults to `.`) |

## How It Works

1. Files are recursively collected from the specified directory
2. Each file is gzip-compressed
3. With `--encoding raw` (default), the compressed bytes are concatenated and appended as the trailing payload. With `--encoding base64`, each file's compressed bytes are individually base64-encoded and the per-file chunks are concatenated as the trailing payload — every chunk is self-contained valid base64, so the runtime can slice and decode one file's chunk without touching the rest.
4. An `exit 0` guard separates the script body from the trailing payload
5. File offsets and lengths are stored in a bash associative array (`declare -A`, requires bash 4+) — offsets index the trailing payload byte-stream regardless of encoding
6. Helper functions use `tail -c` + `head -c` to extract a file's chunk, then pipe through `base64 -d` (base64 mode only) and `gzip -d`

## Example

**myscript.sh** (development version):
```bash
#!/bin/bash
eval "$(bashfs gen ./assets)"
echo "Available files:"
bashfs_list
echo "Config:"
bashfs_jq config.json .database.host
```

**Package it:**
```bash
bashfs package myscript.sh -o dist/myscript.sh
```

**dist/myscript.sh** is now a single file with all assets embedded. Ship it anywhere.
