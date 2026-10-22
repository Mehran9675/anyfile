# AnyFile

A desktop app that tells you what any file really is, lets you explore everything inside it, and hands each item to the viewer your system already uses for that type. AnyFile never renders content itself.

Built in Go with [giode](https://github.com/Mehran9675/giode).

## What it does

- **Identify.** Runs built-in signatures and structural probes first. It asks TrID only when those aren't confident, and ignores TrID guesses that rest on a single pattern. When TrID is missing or fails, AnyFile falls back to its own heuristics: text and encoding analysis, entropy, and a census of embedded objects.
- **Explore.** Expand any item that has contents, and expand nested containers too: a zip inside an email, the xlsx inside that zip, the image inside that xlsx.
- **View.** "Open in viewer" passes the item to the OS default app for its *detected* type. A spreadsheet named `report.dat` opens in your spreadsheet app, because the copy it opens is named `.xlsx`. "Open with…" shows the system chooser.
- **Extract.** "Save file…" saves a single member of a container. "Extract N items…" extracts a whole container and keeps its folder structure. Paths are sanitised, so an archive can't write outside the folder you chose. The opened file itself can only be viewed, since it is already a file.

## Engines

| Engine | Package | Handles |
| --- | --- | --- |
| ZIP / TAR / compressors / ar | `explore/engines/archive` | zip and every zip-based format (docx/xlsx/pptx, odt, jar, apk, epub…), tar in gz/bz2/xz/zst, gz/bz2/xz/zst streams, ar/deb |
| OLE compound files | `explore/engines/ole` + `cfb` | doc/xls/ppt, msi, msg, embedded OLE objects and Ole10Native packaged files |
| Executables | `explore/engines/binary` | PE resources (icons rebuilt as .ico, bitmaps, manifests, string tables), sections, overlay, Authenticode; ELF/Mach-O sections and universal slices |
| PDF | `explore/engines/pdf` | attachments, images (JPEG/JP2 bit-exact, raw RGB/Gray/CMYK converted to PNG), fonts, JavaScript, XMP |
| Email | `explore/engines/mail` | eml, mht/mhtml, mbox: bodies and attachments |
| Disc images | `explore/engines/disc` | ISO 9660 + Joliet |
| Encoded blobs | `explore/engines/blobs` | data: URIs and base64 payloads inside text files |
| Call of Duty XSUB | `explore/engines/xsub` + `oggopus` + `oodle` | `KAPI` stream packages: bare Opus audio rewrapped as standard `.opus` (mono to 7.1 and beyond), Oodle-compressed assets decompressed with the game's own `oo2core` DLL |
| 7-Zip *(optional)* | `explore/engines/sevenzip` | 7z, rar, cab, wim, vhd(x), dmg, NSIS and other installers, UDF… |
| FFmpeg *(optional)* | `explore/engines/media` | video/audio/subtitle streams, cover art, Matroska attachments, all copied without re-encoding |
| Signature carver | `explore/engines/carver` + `carve` | last resort, plus "Deep scan" on demand: finds embedded files anywhere and works out where each one ends |

## External tools (all optional)

AnyFile looks for each tool in this order: an environment variable, next to `anyfile.exe` or in its `tools/` folder, `PATH`, then common install locations.

| Tool | Variable | Notes |
| --- | --- | --- |
| TrID | `ANYFILE_TRID` | `triddefs.trd` must sit next to `trid.exe` |
| 7-Zip | `ANYFILE_7Z` | `7z.exe` |
| ffprobe / FFmpeg | `ANYFILE_FFPROBE` / `ANYFILE_FFMPEG` | both are needed for media streams |
| Oodle | `ANYFILE_OODLE` | `oo2core_*_win64.dll`. Games ship it, so AnyFile also looks in the opened file's folder and two levels up. Never bundled (proprietary). |

The status bar shows which engines were found.

## Layout

```
cmd/anyfile        the app
cmd/anyprobe       CLI: detect + recursively explore files (engine testing)
internal/
  detect           identification pipeline (signatures, structure, TrID, heuristics)
  carve            embedded-object scanner with per-format extent logic
  cfb              Compound File Binary reader
  oggopus          wraps bare Opus packets in Ogg (multistream layouts inferred)
  oodle            loads the oo2core library a game ships and decompresses with it
  explore          core: Entry/Listing model, engine registry, safe extraction
    engines/*      one package per engine family; engines/all enables them all
  session          the inspected tree and its background jobs (no UI code)
  platform         OS hand-off: open, open-with, reveal, dialogs, drag & drop
  tools            locating external programs
  ui               giode interface (tree, inspector, buttons, theme)
```

To add a format, create a package under `internal/explore/engines/` that calls `explore.Register` from `init`, then import it in `engines/all`.

## Download

Every [release](https://github.com/Mehran9675/anyfile/releases) has builds for:

| Platform | x86-64 | ARM64 |
| --- | --- | --- |
| Windows | `AnyFile-<version>-windows-amd64.zip` | `AnyFile-<version>-windows-arm64.zip` |
| macOS (11+) | `AnyFile-<version>-macos-amd64.zip` | `AnyFile-<version>-macos-arm64.zip` |
| Linux | `AnyFile-<version>-linux-amd64.tar.gz` | `AnyFile-<version>-linux-arm64.tar.gz` |

`SHA256SUMS.txt` lists the checksums. Platform notes:

- **macOS:** the app is not notarized. The first time, right-click AnyFile.app and choose Open.
- **Linux:** the file dialogs use `zenity` (or `kdialog`); install one if your desktop lacks it.
- **Dropping files onto the window** works on Windows only for now; elsewhere use **Open file** or pass a path: `anyfile path/to/file`.

## Releasing

Publishing a GitHub release runs `.github/workflows/release.yml`, which tests the engines, then builds and attaches all six packages. Windows builds don't need cgo. Linux builds run on x86-64 and ARM64 runners, because Gio needs cgo there. macOS builds both architectures on an Apple Silicon runner. To rebuild an existing release, run the workflow by hand with its tag.

## Build and run

```bash
go build -ldflags="-H windowsgui" -o bin/anyfile.exe ./cmd/anyfile
```

```bash
bin/anyfile.exe path/to/file
```

You can also drop a file onto the window or use **Open file**. To test the engines without the UI:

```bash
go run ./cmd/anyprobe -depth 3 path/to/file
```

Useful flags: `-limit N` extracts at most N entries per container (`-1` only prints a summary), `-match Audio/` filters entries by path, `-out DIR` keeps the extracted files, and `-deep` runs the signature carver instead of the format engines.

```bash
go test ./...
```
