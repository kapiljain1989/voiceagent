package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"sync"

	"github.com/pion/rtp"
)

type RTPListener struct {
	conn  *net.UDPConn
	port  int
	codec string

	// RTP send state (agent → caller)
	remoteAddr *net.UDPAddr
	sendMu     sync.Mutex
	sendSeq    uint16
	sendTS     uint32
	sendSSRC   uint32
}

func NewRTPListener(port int, codec string) (*RTPListener, error) {
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("rtp listen on :%d: %w", port, err)
	}

	return &RTPListener{
		conn:     conn,
		port:     port,
		codec:    codec,
		sendSSRC: rand.Uint32(),
	}, nil
}

func (r *RTPListener) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
}

func (r *RTPListener) RemoteAddrStr() string {
	if r.remoteAddr == nil {
		return "<nil>"
	}
	return r.remoteAddr.String()
}

func (r *RTPListener) SetRemoteAddr(host string, port int) error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return err
	}
	r.remoteAddr = addr
	return nil
}

// ReceiveAndDecode reads RTP from the caller, decodes G.711→PCM, feeds copilot + audio taps.
func (r *RTPListener) ReceiveAndDecode(ctx context.Context, pcmCh chan []byte, role, callID string, copilot *siprecSession) {
	log := slog.With("call_id", callID, "role", role, "rtp_port", r.port)
	log.Info("RTP listener started", "codec", r.codec)

	buf := make([]byte, 1500)
	pkt := &rtp.Packet{}
	frameCount := 0

	for {
		select {
		case <-ctx.Done():
			log.Info("RTP listener stopped", "frames", frameCount)
			return
		default:
		}

		n, sender, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Debug("rtp read", "err", err)
			continue
		}

		// Learn/update the remote address from the first packet (NAT may differ from SDP)
		if sender != nil && (r.remoteAddr == nil || frameCount == 0) {
			if r.remoteAddr != nil && r.remoteAddr.String() != sender.String() {
				log.Info("RTP remote address updated (NAT)", "sdp", r.remoteAddr.String(), "actual", sender.String())
			}
			r.remoteAddr = sender
			if frameCount == 0 {
				log.Info("RTP remote address learned", "addr", sender.String())
			}
		}

		if err := pkt.Unmarshal(buf[:n]); err != nil {
			continue
		}

		// Skip non-audio payloads (comfort noise PT 13, DTMF PT 101, proprietary PTs)
		expectedPT := PayloadTypeForCodec(r.codec)
		if pkt.PayloadType != expectedPT {
			if frameCount == 0 {
				log.Debug("skipping non-audio RTP", "pt", pkt.PayloadType, "expected", expectedPT)
			}
			continue
		}

		payload := pkt.Payload
		if len(payload) == 0 {
			continue
		}

		// Decode G.711 to 16-bit PCM and resample 8kHz→16kHz
		pcm := ResampleG711toL16(payload, r.codec)

		frameCount++
		if frameCount == 1 {
			log.Info("first RTP packet",
				"ssrc", pkt.SSRC,
				"pt", pkt.PayloadType,
				"seq", pkt.SequenceNumber,
				"payload_bytes", len(payload),
				"pcm_bytes", len(pcm),
				"remote", sender.String(),
			)
		}

		// Feed copilot STT pipeline
		select {
		case pcmCh <- pcm:
		default:
		}

		// Conference mode: feed mixer instead of direct taps
		if role == "caller" && copilot != nil && copilot.conference != nil {
			select {
			case copilot.conference.mixer.participants[0].pcmIn <- pcm:
			default:
			}
			continue
		}

		// Broadcast to audio taps (WebRTC bridge)
		if role == "caller" && copilot != nil {
			copilot.broadcastToTaps(pcm)
		}
	}
}

// SendPCM encodes 16kHz L16 PCM → G.711 μ-law, wraps in RTP, sends to the caller's SBC.
func (r *RTPListener) SendPCM(pcm16k []byte) error {
	if r.remoteAddr == nil {
		return fmt.Errorf("no remote address")
	}
	if r.conn == nil {
		return fmt.Errorf("connection closed")
	}

	// 16kHz L16 → 8kHz L16 → G.711 μ-law
	pcm8k := resample(pcm16k, 16000, 8000)
	ulaw := EncodeG711Ulaw(pcm8k)

	// Build RTP packet
	r.sendMu.Lock()
	r.sendSeq++
	r.sendTS += 160 // 20ms at 8kHz = 160 samples
	pkt := rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0, // PCMU
			SequenceNumber: r.sendSeq,
			Timestamp:      r.sendTS,
			SSRC:           r.sendSSRC,
		},
		Payload: ulaw,
	}
	r.sendMu.Unlock()

	data, err := pkt.Marshal()
	if err != nil {
		return err
	}

	_, err = r.conn.WriteTo(data, r.remoteAddr)
	return err
}

// SendG711 sends pre-encoded G.711 payload as RTP (for codec passthrough).
func (r *RTPListener) SendG711(payload []byte) error {
	if r.remoteAddr == nil || r.conn == nil {
		return fmt.Errorf("not ready")
	}

	r.sendMu.Lock()
	r.sendSeq++
	r.sendTS += uint32(len(payload)) // 1 byte = 1 sample at 8kHz
	pkt := rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0,
			SequenceNumber: r.sendSeq,
			Timestamp:      r.sendTS,
			SSRC:           r.sendSSRC,
		},
		Payload: payload,
	}
	r.sendMu.Unlock()

	data, err := pkt.Marshal()
	if err != nil {
		return err
	}

	_, err = r.conn.WriteTo(data, r.remoteAddr)
	return err
}

// PayloadTypeForCodec returns the RTP payload type number.
func PayloadTypeForCodec(codec string) uint8 {
	switch codec {
	case "PCMU", "pcmu", "ulaw":
		return 0
	case "PCMA", "pcma", "alaw":
		return 8
	default:
		return 0
	}
}

// FrameSize returns bytes for a given duration at 8kHz G.711.
func (r *RTPListener) FrameSize(durationMs int) int {
	return binary.Size(byte(0)) * 8 * durationMs // 8 samples/ms * 1 byte/sample
}
