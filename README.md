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

When ready to distribute, package everything into a single self-contained script:

```bash
bashfs package myscript.sh -o dist/myscript.sh
```

This finds the `eval $(bashfs gen <dir>)` line in your script and replaces it with embedded, gzip-compressed, base64-encoded file data and self-contained helper functions. The output script has no external file dependencies.

## Generated Functions

| Function | Description |
|---|---|
| `bashfs_cat <path>` | Print file contents to stdout |
| `bashfs_extract <path> <dest>` | Extract a file to the given destination |
| `bashfs_list` | List all embedded file paths |
| `bashfs_jq <path> [filter]` | Run jq on a file (filter defaults to `.`) |

## How It Works

1. Files are recursively collected from the specified directory
2. Each file is gzip-compressed and base64-encoded
3. Data is stored in a bash associative array (`declare -A`, requires bash 4+)
4. Helper functions decode and decompress on the fly using `base64 -d | gzip -d`

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
