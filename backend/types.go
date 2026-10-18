package main

import (
	"context"
	"encoding/json"
	"sync"
)

const (
	protocolVersion = "1"
	appVersion      = "0.1.0"
)

type message struct {
	ProtocolVersion string          `json:"protocolVersion"`
	RequestID       string          `json:"requestId"`
	Type            string          `json:"type"`
	Payload         json.RawMessage `json:"payload"`
}

type event struct {
	ProtocolVersion string `json:"protocolVersion"`
	RequestID       string `json:"requestId"`
	Type            string `json:"type"`
	Payload         any    `json:"payload"`
}

type scanRequest struct {
	InputDir         string `json:"inputDir"`
	OutputDir        string `json:"outputDir"`
	Recursive        bool   `json:"recursive"`
	OutputNameFormat string `json:"outputNameFormat"`
}

type runRequest struct {
	InputDir         string  `json:"-"`
	OutputDir        string  `json:"outputDir"`
	MaxParallel      int     `json:"maxParallel"`
	StrictHash       bool    `json:"strictHash"`
	DateFolders      bool    `json:"dateFolders"`
	DateFolderFormat string  `json:"dateFolderFormat"`
	CopyOriginals    bool    `json:"copyOriginals"`
	PreserveFolders  bool    `json:"preserveFolders"`
	WriteReport      bool    `json:"writeReport"`
	Groups           []group `json:"groups"`
}

type videoFile struct {
	Path           string          `json:"path"`
	Name           string          `json:"name"`
	Size           int64           `json:"size"`
	Modified       string          `json:"modified"`
	Captured       string          `json:"captured"`
	CapturedSource string          `json:"capturedSource"`
	Duration       float64         `json:"duration"`
	Width          int             `json:"width"`
	Height         int             `json:"height"`
	Chapter        int             `json:"chapter"`
	HasGPMF        bool            `json:"hasGpmf"`
	Classification string          `json:"classification"`
	Streams        []streamSummary `json:"streams"`
}

type streamSummary struct {
	Type          string `json:"type"`
	Codec         string `json:"codec"`
	CodecTag      string `json:"codecTag,omitempty"`
	Profile       string `json:"profile,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	PixelFormat   string `json:"pixelFormat,omitempty"`
	ColorTransfer string `json:"colorTransfer,omitempty"`
	FrameRate     string `json:"frameRate,omitempty"`
	SampleRate    string `json:"sampleRate,omitempty"`
	Channels      int    `json:"channels,omitempty"`
}

type group struct {
	ID          string      `json:"id"`
	Key         string      `json:"key"`
	Files       []videoFile `json:"files"`
	Status      string      `json:"status"`
	Confidence  string      `json:"confidence"`
	Basis       string      `json:"basis"`
	HasGPMF     bool        `json:"hasGpmf"`
	Reason      string      `json:"reason,omitempty"`
	OutputName  string      `json:"outputName"`
	RelativeDir string      `json:"relativeDir"`
}

type skippedFile struct {
	Path           string `json:"path"`
	Reason         string `json:"reason"`
	Classification string `json:"classification"`
}

type scanResult struct {
	Groups  []group       `json:"groups"`
	Skipped []skippedFile `json:"skipped"`
}

type jobResult struct {
	GroupID          string          `json:"groupId"`
	Status           string          `json:"status"`
	OutputPath       string          `json:"outputPath,omitempty"`
	Code             string          `json:"code,omitempty"`
	Message          string          `json:"message,omitempty"`
	SHA256           string          `json:"sha256,omitempty"`
	OutputSize       int64           `json:"outputSize,omitempty"`
	OutputModified   string          `json:"outputModified,omitempty"`
	Originals        []string        `json:"originals,omitempty"`
	Warnings         []string        `json:"warnings,omitempty"`
	Captured         string          `json:"captured,omitempty"`
	CapturedSource   string          `json:"capturedSource,omitempty"`
	OutputStreams    []streamSummary `json:"outputStreams,omitempty"`
	GPMFVerification string          `json:"gpmfVerification,omitempty"`
}

type runReport struct {
	Version    string         `json:"version"`
	StartedAt  string         `json:"startedAt"`
	FinishedAt string         `json:"finishedAt"`
	InputDir   string         `json:"inputDir"`
	OutputDir  string         `json:"outputDir"`
	FileCount  int            `json:"fileCount"`
	GroupCount int            `json:"groupCount"`
	Results    []reportResult `json:"results"`
}

type reportResult struct {
	Group  group     `json:"group"`
	Result jobResult `json:"result"`
}

type jobProgress struct {
	GroupID  string  `json:"groupId"`
	Progress float64 `json:"progress"`
}

type probeResult struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

type probeFormat struct {
	Duration string            `json:"duration"`
	Tags     map[string]string `json:"tags"`
}

type probeStream struct {
	Index          int               `json:"index"`
	CodecName      string            `json:"codec_name"`
	CodecType      string            `json:"codec_type"`
	CodecTagString string            `json:"codec_tag_string"`
	Profile        string            `json:"profile"`
	Width          int               `json:"width"`
	Height         int               `json:"height"`
	PixelFormat    string            `json:"pix_fmt"`
	ColorSpace     string            `json:"color_space"`
	ColorTransfer  string            `json:"color_transfer"`
	ColorPrimaries string            `json:"color_primaries"`
	FrameRate      string            `json:"r_frame_rate"`
	TimeBase       string            `json:"time_base"`
	SampleRate     string            `json:"sample_rate"`
	Channels       int               `json:"channels"`
	ChannelLayout  string            `json:"channel_layout"`
	ReadPackets    string            `json:"nb_read_packets"`
	Tags           map[string]string `json:"tags"`
}

type app struct {
	toolsMu   sync.RWMutex
	out       *json.Encoder
	emitMu    sync.Mutex
	runMu     sync.Mutex
	cancelRun context.CancelFunc
	nameMu    sync.Mutex
	reserved  map[string]bool
	planMu    sync.Mutex
	plan      *scanPlan
}

type scanPlan struct {
	InputDir  string
	OutputDir string
	Groups    map[string]group
}
