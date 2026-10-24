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
| 7-Zip *(optional)* | `explore/engines/sevenzip` | 7z, rar, cab, wim, vhd(x), dmg, NSIS and other installers, UDF… |
| FFmpeg *(optional)* | `explore/engines/media` | video/audio/subtitle streams, cover art, Matroska attachments, all copied without re-encoding |
| Signature carver | `explore/engines/carver` + `carve` | last resort, plus "Deep scan" on demand: finds embedded files anywhere and works out where each one ends |

## External tools (all optional)

AnyFile looks for each tool in this order: an environment variable, next to `anyfile.exe` or in its `tools/` folder, `PATH`, then common install locations.

| Tool | Variable | Notes |
| --- | --- | --- |
| TrID | `ANYFILE_TRID` | `triddefs.trd` must sit next to `trid.exe` |
| Magika | `ANYFILE_MAGIKA` | Google's deep-learning content-type model: `pip install magika` or `cargo install magika-cli`. Asked when the built-in engines aren't sure, and about generic plain text. |
| 7-Zip | `ANYFILE_7Z` | `7z.exe` |
| ffprobe / FFmpeg | `ANYFILE_FFPROBE` / `ANYFILE_FFMPEG` | both are needed for media streams |
| Oodle | `ANYFILE_OODLE` | `oo2core_*_win64.dll`. Games ship it, so AnyFile also looks in the opened file's folder and two levels up. Never bundled (proprietary). |

The status bar shows which engines were found.

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
