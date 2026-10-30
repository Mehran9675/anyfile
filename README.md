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

**Settings > Engines** installs TrID, Magika, 7-Zip and FFmpeg for you. AnyFile downloads each one from its publisher, checks it against the published checksum where there is one (Magika, and FFmpeg on Windows and Linux), makes sure it runs, and installs it for your account only, without administrator rights. **Update** fetches the current release and **Remove** deletes it. The inspector offers the same install when TrID or Magika is missing. Installed engines live in:

- Windows: `%LOCALAPPDATA%\AnyFile\tools`
- macOS: `~/Library/Application Support/AnyFile/tools`
- Linux: `$XDG_DATA_HOME/AnyFile/tools` (by default `~/.local/share/AnyFile/tools`)

AnyFile looks for each tool in this order: the path set in Settings, an environment variable, its own install, next to `anyfile.exe` or in its `tools/` folder, `PATH`, then common install locations.

| Tool | Variable | In-app install from | Notes |
| --- | --- | --- | --- |
| TrID | `ANYFILE_TRID` | [mark0.net](https://mark0.net/soft-trid-e.html) | `triddefs.trd` must sit next to `trid.exe`. No macOS or ARM Linux build. |
| Magika | `ANYFILE_MAGIKA` | [google/magika](https://github.com/google/magika) | Google's deep-learning content-type model. Asked when the built-in engines aren't sure, and about generic plain text. |
| 7-Zip | `ANYFILE_7Z` | [ip7z/7zip](https://github.com/ip7z/7zip) | `7z.exe` on Windows, `7zz` elsewhere |
| ffprobe / FFmpeg | `ANYFILE_FFPROBE` / `ANYFILE_FFMPEG` | [BtbN/FFmpeg-Builds](https://github.com/BtbN/FFmpeg-Builds); [martin-riedl.de](https://ffmpeg.martin-riedl.de) on macOS | both are needed for media streams |
| Oodle | `ANYFILE_OODLE` | none (proprietary) | `oo2core_*_win64.dll`. Games ship it, so AnyFile also looks in the opened file's folder and two levels up. |

The status bar shows which engines were found.

AnyFile runs as one window: files opened while it runs, from the command line or with **Inspect with AnyFile** in the file manager's right-click menu, open as tabs in it. The Windows installer and the macOS app add that menu entry; for other copies, **Settings > Files** adds or removes it for your account.

## Languages

The interface is available in English and 24 other European languages: Bulgarian, Croatian, Czech, Danish, Dutch, Estonian, Finnish, French, German, Greek, Hungarian, Italian, Latvian, Lithuanian, Norwegian Bokmål, Polish, Portuguese, Romanian, Russian, Slovak, Slovenian, Spanish, Swedish and Ukrainian. AnyFile follows the system language; **Settings > General > Language** picks another one, and the change applies right away. File type names and the engines' own reports stay in English.

Translations live in `internal/locales/<language>.json` and are keyed by the English text, so a missing entry falls back to English. Entries with a `{count}` take plural forms (`one`, `few`, `other`… as the language needs).

## Build and run

AnyFile is built with the [giode CLI](https://github.com/Mehran9675/giode-cli), configured by `giode.toml`:

```bash
go install github.com/mehran9675/giode-cli/cmd/giode@latest
```

```bash
giode run
```

`giode run` starts AnyFile on this computer; pass a file to inspect after `--`, as in `giode run desktop -- path/to/file`. To build packages into `dist/`:

```bash
giode build windows windows-installer
```

Use `macos` or `dmg` on a Mac and `linux`, `deb` or `rpm` on Linux (Gio needs cgo there). `giode doctor` lists what each target needs; the Windows installer needs [NSIS](https://nsis.sourceforge.io). Without the CLI, `go build -ldflags="-H windowsgui" ./cmd/anyfile` still works, minus the icon and version info.

You can also drop a file onto the window or use **Open file**. To test the engines without the UI:

```bash
go run ./cmd/anyprobe -depth 3 path/to/file
```

Useful flags: `-limit N` extracts at most N entries per container (`-1` only prints a summary), `-match Audio/` filters entries by path, `-out DIR` keeps the extracted files, and `-deep` runs the signature carver instead of the format engines.

```bash
go test ./...
```

The engine installers have a test that downloads every engine, for this computer and the others, to check that their sources still match:

```bash
ANYFILE_NET_TEST=1 go test -run 'Live|Other' -timeout 30m ./internal/install
```
