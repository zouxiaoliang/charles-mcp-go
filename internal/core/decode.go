package core

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"golang.org/x/text/encoding/htmlindex"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Decoder working memory is independent of the session export size.
const maxDecoderMemoryBytes = 128 << 20

func preview(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func bodyBytes(m Message) ([]byte, []string, error) {
	b := m.Body.Data
	warnings := []string{}
	if len(b) > MaxBodyBytes {
		return nil, warnings, fmt.Errorf("body exceeds limit")
	}
	enc := strings.Split(strings.ToLower(m.Headers.Get("Content-Encoding")), ",")
	for i := len(enc) - 1; i >= 0; i-- {
		var r io.Reader
		var closeFn func()
		var err error
		switch strings.TrimSpace(enc[i]) {
		case "", "identity":
			continue
		case "gzip":
			var g *gzip.Reader
			g, err = gzip.NewReader(bytes.NewReader(b))
			if err == nil {
				r = g
				closeFn = func() { g.Close() }
			}
		case "deflate":
			var z io.ReadCloser
			z, err = zlib.NewReader(bytes.NewReader(b))
			if err == nil {
				r = z
				closeFn = func() { z.Close() }
			}
		case "br":
			r = brotli.NewReader(bytes.NewReader(b))
		case "zstd":
			var z *zstd.Decoder
			z, err = zstd.NewReader(bytes.NewReader(b), zstd.WithDecoderMaxMemory(maxDecoderMemoryBytes))
			if err == nil {
				r = z
				closeFn = z.Close
			}
		default:
			warnings = append(warnings, "unsupported content encoding: "+enc[i])
			continue
		}
		if err == nil {
			var decoded []byte
			decoded, err = readBounded(r, MaxBodyBytes)
			if err == nil {
				b = decoded
			}
		}
		if closeFn != nil {
			closeFn()
		}
		if err != nil {
			var sizeErr *SizeLimitError
			if errors.As(err, &sizeErr) {
				return nil, warnings, err
			}
			warnings = append(warnings, "could not decompress "+enc[i]+"; Charles may have already decoded the body")
			break
		}
	}
	return b, warnings, nil
}
func bodyText(m Message, b []byte) string {
	charset := m.Body.Charset
	if charset == "" {
		_, p, _ := mime.ParseMediaType(m.Headers.Get("Content-Type"))
		charset = p["charset"]
	}
	if charset != "" && !strings.EqualFold(charset, "utf-8") {
		if enc, err := htmlindex.Get(charset); err == nil {
			if decoded, err := enc.NewDecoder().Bytes(b); err == nil {
				return string(decoded)
			}
		}
	}
	return strings.ToValidUTF8(string(b), "�")
}

type DecodeResult struct {
	Format     string   `json:"format"`
	Value      any      `json:"value,omitempty"`
	Preview    string   `json:"preview"`
	Bytes      int      `json:"byte_length"`
	Truncated  bool     `json:"truncated"`
	Warnings   []string `json:"warnings"`
	ArtifactID string   `json:"artifact_id,omitempty"`
}

func Decode(m Message, descriptor, message string, maxChars int) (DecodeResult, error) {
	out := DecodeResult{Warnings: []string{}}
	if m.Body.Preservation == "missing" {
		return out, fmt.Errorf("body not captured")
	}
	b, w, err := bodyBytes(m)
	out.Warnings = w
	if err != nil {
		return out, err
	}
	out.Bytes = len(b)
	text := bodyText(m, b)
	out.Preview = preview(text, maxChars)
	out.Truncated = len([]rune(text)) > maxChars
	ct, params, _ := mime.ParseMediaType(m.Headers.Get("Content-Type"))
	trim := strings.TrimSpace(text)
	switch {
	case descriptor != "" || message != "" || strings.Contains(ct, "protobuf") || strings.Contains(ct, "proto"):
		out.Format = "protobuf"
		if descriptor == "" || message == "" {
			out.Format = "binary"
			out.Preview = hex.EncodeToString(b[:min(len(b), maxChars/2)])
			out.Warnings = append(out.Warnings, "protobuf_descriptor_required: provide descriptor_path and message_type")
			return out, nil
		}
		raw, err := readFile(descriptor, MaxBodyBytes)
		if err != nil {
			return out, err
		}
		var set descriptorpb.FileDescriptorSet
		if err = proto.Unmarshal(raw, &set); err != nil {
			return out, err
		}
		files, err := protodesc.NewFiles(&set)
		if err != nil {
			return out, err
		}
		d, err := files.FindDescriptorByName(protoreflect.FullName(strings.TrimPrefix(message, ".")))
		if err != nil {
			return out, err
		}
		md, ok := d.(protoreflect.MessageDescriptor)
		if !ok {
			return out, fmt.Errorf("%q is not a message", message)
		}
		msg := dynamicpb.NewMessage(md)
		if err = proto.Unmarshal(b, msg); err != nil {
			return out, err
		}
		raw, err = (protojson.MarshalOptions{UseProtoNames: true}).Marshal(msg)
		if err != nil {
			return out, err
		}
		if err = json.Unmarshal(raw, &out.Value); err != nil {
			return out, err
		}
	case strings.Contains(ct, "json") || strings.HasPrefix(trim, "{") || strings.HasPrefix(trim, "["):
		out.Format = "json"
		if err = json.Unmarshal([]byte(text), &out.Value); err != nil {
			out.Format = "text"
			out.Warnings = append(out.Warnings, "json_decode_failed")
		}
	case ct == "application/x-www-form-urlencoded":
		out.Format = "form"
		v, err := url.ParseQuery(text)
		if err != nil {
			return out, err
		}
		out.Value = v
	case strings.HasPrefix(ct, "multipart/"):
		out.Format = "multipart"
		if params["boundary"] == "" {
			return out, fmt.Errorf("multipart boundary missing")
		}
		parts := []map[string]any{}
		reader := multipart.NewReader(bytes.NewReader(b), params["boundary"])
		for {
			p, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return out, err
			}
			data, err := readBounded(p, MaxBodyBytes)
			p.Close()
			if err != nil {
				return out, err
			}
			parts = append(parts, map[string]any{"name": p.FormName(), "filename": p.FileName(), "headers": p.Header, "byte_length": len(data), "preview": preview(string(data), maxChars), "truncated": len([]rune(string(data))) > maxChars})
			if len(parts) > 1000 {
				return out, fmt.Errorf("too many multipart parts")
			}
		}
		out.Value = parts
	default:
		out.Format = "text"
		if !utf8.Valid(b) && !strings.HasPrefix(ct, "text/") && m.Body.Charset == "" {
			out.Format = "binary"
			out.Preview = hex.EncodeToString(b[:min(len(b), maxChars/2)])
		}
	}
	if out.Value != nil {
		raw, _ := json.Marshal(out.Value)
		if len([]rune(string(raw))) > maxChars {
			out.Value = nil
			out.Preview = preview(string(raw), maxChars)
			out.Truncated = true
		}
	}
	return out, nil
}
func (s *Store) DecodeEntry(ctx context.Context, id, side, descriptor, message string, maxChars int) (DecodeResult, error) {
	e, err := s.Entry(ctx, id)
	if err != nil {
		return DecodeResult{}, err
	}
	var m Message
	switch side {
	case "request":
		m = e.Request
	case "response":
		m = e.Response
	default:
		return DecodeResult{}, fmt.Errorf("side must be request or response")
	}
	d, err := Decode(m, descriptor, message, maxChars)
	if err != nil {
		return d, err
	}
	d.ArtifactID, err = s.SaveArtifact(ctx, "decoded", id, map[string]any{"side": side, "decoded": d})
	return d, err
}
