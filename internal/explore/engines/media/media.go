// Package media exposes the streams of audio/video files (video, audio,
// subtitle tracks, cover art, Matroska attachments) through FFmpeg,
// copying them out without re-encoding.
package media

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xmhrnx/anyfile/internal/detect"
	"github.com/xmhrnx/anyfile/internal/explore"
	"github.com/xmhrnx/anyfile/internal/tools"
)

func init() {
	explore.Register(explore.Engine{Name: "FFmpeg", Priority: 110, Match: matchMedia, List: listMedia})
}

// --- FFmpeg media streams --------------------------------------------------------

func matchMedia(d *detect.Result, h []byte) bool {
	if !tools.FFprobe().Found() || !tools.FFmpeg().Found() {
		return false
	}
	switch d.Ext {
	case "opus", "wav", "aac", "ac3", "eac3", "dts", "amr", "au", "mid":
		return false // single-stream formats: nothing to split out
	}
	return d.Category == detect.CatAudio || d.Category == detect.CatVideo
}

type probeOut struct {
	Streams []struct {
		Index       int               `json:"index"`
		CodecType   string            `json:"codec_type"`
		CodecName   string            `json:"codec_name"`
		CodecLong   string            `json:"codec_long_name"`
		Width       int               `json:"width"`
		Height      int               `json:"height"`
		SampleRate  string            `json:"sample_rate"`
		Channels    int               `json:"channels"`
		Duration    string            `json:"duration"`
		Tags        map[string]string `json:"tags"`
		Disposition map[string]int    `json:"disposition"`
	} `json:"streams"`
	Format struct {
		FormatLong string `json:"format_long_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
}

// container/extension for a stream copied out without re-encoding.
func streamTarget(kind, codec string) (format, ext string) {
	switch kind {
	case "video":
		switch codec {
		case "mjpeg":
			return "image2pipe", "jpg"
		case "png":
			return "image2pipe", "png"
		case "vp8", "vp9", "av1":
			return "webm", "webm"
		}
		return "matroska", "mkv"
	case "audio":
		switch codec {
		case "aac":
			return "adts", "aac"
		case "mp3":
			return "mp3", "mp3"
		case "flac":
			return "flac", "flac"
		case "opus":
			return "opus", "opus"
		case "vorbis":
			return "ogg", "ogg"
		case "ac3":
			return "ac3", "ac3"
		case "eac3":
			return "eac3", "eac3"
		case "dts":
			return "dts", "dts"
		case "alac":
			return "matroska", "mka"
		}
		if strings.HasPrefix(codec, "pcm_") {
			return "wav", "wav"
		}
		return "matroska", "mka"
	case "subtitle":
		switch codec {
		case "subrip", "mov_text":
			return "srt", "srt"
		case "ass", "ssa":
			return "ass", "ass"
		case "webvtt":
			return "webvtt", "vtt"
		}
		return "matroska", "mks"
	}
	return "", ""
}

func listMedia(p string, d *detect.Result) (*explore.Listing, error) {
	cmd := tools.Command(tools.FFprobe().Path, "-v", "error", "-show_streams", "-show_format", "-of", "json", p)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe: %v", err)
	}
	var po probeOut
	if err := json.Unmarshal(out, &po); err != nil {
		return nil, err
	}
	l := &explore.Listing{Engine: "FFmpeg", Format: po.Format.FormatLong}
	ffmpeg := tools.FFmpeg().Path
	for _, s := range po.Streams {
		s := s
		var desc []string
		desc = append(desc, s.CodecName)
		if s.Width > 0 {
			desc = append(desc, fmt.Sprintf("%dx%d", s.Width, s.Height))
		}
		if s.SampleRate != "" {
			desc = append(desc, s.SampleRate+" Hz")
		}
		if s.Channels > 0 {
			desc = append(desc, fmt.Sprintf("%d ch", s.Channels))
		}
		if lang := s.Tags["language"]; lang != "" && lang != "und" {
			desc = append(desc, lang)
		}
		title := s.Tags["title"]
		switch s.CodecType {
		case "attachment":
			fname := s.Tags["filename"]
			if fname == "" {
				fname = fmt.Sprintf("attachment_%d.bin", s.Index)
			}
			idx := s.Index
			l.Entries = append(l.Entries, &explore.Entry{
				Path: "Attachments/" + fname, Size: -1, Note: s.Tags["mimetype"],
				Open: func() (io.ReadCloser, error) { return dumpAttachment(ffmpeg, p, idx, fname) },
			})
			continue
		case "data":
			continue
		}
		kind := s.CodecType
		if s.Disposition["attached_pic"] == 1 {
			kind = "video"
			title = "cover art"
		}
		format, ext := streamTarget(kind, s.CodecName)
		if format == "" {
			continue
		}
		label := strings.ToUpper(s.CodecType[:1]) + s.CodecType[1:]
		if title == "cover art" {
			label = "Cover art"
		}
		name := fmt.Sprintf("%s #%d", label, s.Index)
		if title != "" && title != "cover art" {
			name += " - " + explore.SafeName(title)
		}
		args := []string{"-v", "error", "-i", p, "-map", fmt.Sprintf("0:%d", s.Index), "-c", "copy"}
		if s.CodecName == "mov_text" {
			args[len(args)-1] = "srt"
		}
		if format == "image2pipe" {
			args = append(args, "-frames:v", "1")
		}
		args = append(args, "-f", format, "pipe:1")
		l.Entries = append(l.Entries, &explore.Entry{
			Path: "Streams/" + name + "." + ext, Size: -1,
			Note: strings.Join(desc, " · "),
			Open: func() (io.ReadCloser, error) { return pipeCommand(ffmpeg, args...) },
		})
	}
	if len(l.Entries) <= 1 {
		return nil, errors.New("a single stream: nothing to split out")
	}
	return l, nil
}

func pipeCommand(exe string, args ...string) (io.ReadCloser, error) {
	cmd := tools.Command(exe, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return explore.NewReadCloser(out, func() error {
		out.Close()
		if err := cmd.Wait(); err != nil {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return errors.New(msg)
			}
			return err
		}
		return nil
	}), nil
}

// dumpAttachment extracts a Matroska attachment through a temp file,
// since ffmpeg can only write attachments to disk.
func dumpAttachment(ffmpeg, p string, idx int, name string) (io.ReadCloser, error) {
	dir, err := os.MkdirTemp("", "anyfile-att-")
	if err != nil {
		return nil, err
	}
	dst := filepath.Join(dir, explore.SafeName(name))
	cmd := tools.Command(ffmpeg, "-v", "error", fmt.Sprintf("-dump_attachment:%d", idx), dst, "-i", p)
	cmd.Run() // ffmpeg exits non-zero ("no output file") even on success
	f, err := os.Open(dst)
	if err != nil {
		os.RemoveAll(dir)
		return nil, errors.New("attachment could not be extracted")
	}
	return explore.NewReadCloser(f, func() error { f.Close(); return os.RemoveAll(dir) }), nil
}
