package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/pion/rtp"
)

type RTPListener struct {
	conn  *net.UDPConn
	port  int
	codec string
}

func NewRTPListener(port int, codec string) (*RTPListener, error) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("rtp listen on :%d: %w", port, err)
	}

	return &RTPListener{
		conn:  conn,
		port:  port,
		codec: codec,
	}, nil
}

func (r *RTPListener) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
}

func (r *RTPListener) ReceiveAndDecode(ctx context.Context, pcmCh chan []byte, role, callID string) {
	log := slog.With("call_id", callID, "role", role, "rtp_port", r.port)
	log.Info("RTP listener started", "codec", r.codec)

	buf := make([]byte, 1500)
	pkt := &rtp.Packet{}
	frameCount := 0

	for {
		select {
		case <-ctx.Done():
			log.Info("RTP listener stopped")
			return
		default:
		}

		n, _, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Debug("rtp read", "err", err)
			continue
		}

		if err := pkt.Unmarshal(buf[:n]); err != nil {
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
			)
		}

		select {
		case pcmCh <- pcm:
		default:
		}
	}
}
