package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"

	mp4 "github.com/abema/go-mp4"
	"github.com/remko/go-mkvparse"
)

// probeVideoNative reads duration, size, codec and rotation from the
// container in pure Go (MP4/MOV via abema/go-mp4, MKV/WebM via
// go-mkvparse), so the info panel and aspect ratios need no ffprobe. ok
// is false when the container is not one of these or lacks the fields;
// probeVideo then asks ffprobe.
func probeVideoNative(path string) (mediaInfo, bool) {
	var info mediaInfo
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mov", ".3gp":
		info = probeMP4(path)
	case ".mkv", ".webm":
		info = probeMKV(path)
	default:
		return info, false
	}
	if info.Width <= 0 || info.Height <= 0 || info.DurationSec <= 0 {
		return info, false
	}
	info.Duration = formatDuration(info.DurationSec)
	if st, err := os.Stat(path); err == nil && info.BitRate == "" {
		info.BitRate = formatBits(int64(float64(st.Size()) * 8 / info.DurationSec))
	}
	return info, true
}

func probeMP4(path string) mediaInfo {
	var info mediaInfo
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	boxes, err := mp4.ExtractBoxesWithPayload(f, nil, []mp4.BoxPath{
		{mp4.BoxTypeMoov(), mp4.BoxTypeMvhd()},
		{mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeTkhd()},
		{mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd()},
	})
	if err != nil {
		return info
	}
	for _, b := range boxes {
		switch p := b.Payload.(type) {
		case *mp4.Mvhd:
			if p.Timescale > 0 {
				d := p.DurationV0
				if p.GetVersion() == 1 {
					info.DurationSec = float64(p.DurationV1) / float64(p.Timescale)
				} else {
					info.DurationSec = float64(d) / float64(p.Timescale)
				}
			}
		case *mp4.Tkhd:
			// The first track with a picture size is the video track.
			if info.Width == 0 && p.Width > 0 && p.Height > 0 {
				info.Width, info.Height = int(p.GetWidthInt()), int(p.GetHeightInt())
				// Rotation from the display matrix (a, b of the first row).
				deg := int(math.Round(math.Atan2(float64(p.Matrix[1]), float64(p.Matrix[0])) * 180 / math.Pi))
				if deg < 0 {
					deg += 360
				}
				if deg == 90 || deg == 270 {
					info.Width, info.Height = info.Height, info.Width
				}
			}
		}
	}
	// Codec: the sample entry type under stsd (avc1, hvc1, hev1, av01, ...).
	if info.Codec == "" {
		if _, err := f.Seek(0, 0); err == nil {
			entries, _ := mp4.ExtractBoxes(f, nil, []mp4.BoxPath{
				{mp4.BoxTypeMoov(), mp4.BoxTypeTrak(), mp4.BoxTypeMdia(), mp4.BoxTypeMinf(), mp4.BoxTypeStbl(), mp4.BoxTypeStsd(), mp4.BoxTypeAny()},
			})
			for _, e := range entries {
				if name := mp4CodecName(e.Type.String()); name != "" {
					info.Codec = name
					break
				}
			}
		}
	}
	return info
}

func mp4CodecName(fourcc string) string {
	switch fourcc {
	case "avc1", "avc3":
		return "h264"
	case "hvc1", "hev1":
		return "hevc"
	case "av01":
		return "av1"
	case "vp09":
		return "vp9"
	case "mp4v":
		return "mpeg4"
	case "apcn", "apch", "apcs", "apco", "ap4h":
		return "prores"
	}
	return ""
}

// mkvMeta collects Info and Tracks for probeMKV.
type mkvMeta struct {
	mkvparse.DefaultHandler
	scale    int64
	duration float64
	width    int64
	height   int64
	codec    string
	inVideo  bool
}

func (h *mkvMeta) HandleMasterBegin(id mkvparse.ElementID, _ mkvparse.ElementInfo) (bool, error) {
	if id == mkvparse.VideoElement {
		h.inVideo = true
	}
	return true, nil
}

func (h *mkvMeta) HandleMasterEnd(id mkvparse.ElementID, _ mkvparse.ElementInfo) error {
	if id == mkvparse.VideoElement {
		h.inVideo = false
	}
	return nil
}

func (h *mkvMeta) HandleInteger(id mkvparse.ElementID, v int64, _ mkvparse.ElementInfo) error {
	switch id {
	case mkvparse.TimecodeScaleElement:
		h.scale = v
	case mkvparse.PixelWidthElement:
		if h.width == 0 {
			h.width = v
		}
	case mkvparse.PixelHeightElement:
		if h.height == 0 {
			h.height = v
		}
	}
	return nil
}

func (h *mkvMeta) HandleFloat(id mkvparse.ElementID, v float64, _ mkvparse.ElementInfo) error {
	if id == mkvparse.DurationElement {
		h.duration = v
	}
	return nil
}

func (h *mkvMeta) HandleString(id mkvparse.ElementID, v string, _ mkvparse.ElementInfo) error {
	if id == mkvparse.CodecIDElement && h.codec == "" && strings.HasPrefix(v, "V_") {
		h.codec = strings.ToLower(strings.TrimPrefix(v, "V_"))
	}
	return nil
}

func probeMKV(path string) mediaInfo {
	var info mediaInfo
	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()
	h := &mkvMeta{scale: 1000000}
	if err := mkvparse.ParseSections(f, h, mkvparse.InfoElement, mkvparse.TracksElement); err != nil {
		return info
	}
	info.DurationSec = h.duration * float64(h.scale) / 1e9
	info.Width, info.Height = int(h.width), int(h.height)
	info.Codec = strings.NewReplacer("mpeg4/iso/avc", "h264", "mpegh/iso/hevc", "hevc").Replace(h.codec)
	return info
}

// mkvAttach picks the cover among Matroska attachments: a file named
// cover.* wins, otherwise the first image attachment.
type mkvAttach struct {
	mkvparse.DefaultHandler
	name, mime string
	data       []byte
	cover      []byte
	first      []byte
}

func (h *mkvAttach) HandleMasterEnd(id mkvparse.ElementID, _ mkvparse.ElementInfo) error {
	if id == mkvparse.AttachedFileElement && strings.HasPrefix(h.mime, "image/") && len(h.data) > 0 {
		if h.cover == nil && strings.HasPrefix(strings.ToLower(h.name), "cover") {
			h.cover = h.data
		}
		if h.first == nil {
			h.first = h.data
		}
	}
	if id == mkvparse.AttachedFileElement {
		h.name, h.mime, h.data = "", "", nil
	}
	return nil
}

func (h *mkvAttach) HandleString(id mkvparse.ElementID, v string, _ mkvparse.ElementInfo) error {
	switch id {
	case mkvparse.FileNameElement:
		h.name = v
	case mkvparse.FileMimeTypeElement:
		h.mime = v
	}
	return nil
}

func (h *mkvAttach) HandleBinary(id mkvparse.ElementID, v []byte, _ mkvparse.ElementInfo) error {
	if id == mkvparse.FileDataElement {
		h.data = v
	}
	return nil
}

func mkvCover(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	h := &mkvAttach{}
	if mkvparse.ParseSections(f, h, mkvparse.AttachmentsElement) != nil {
		return nil
	}
	if h.cover != nil {
		return h.cover
	}
	return h.first
}
