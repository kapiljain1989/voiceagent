package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
)

// OutboundCall handles agent-initiated outbound calls through SIP trunks.

type outboundCallRequest struct {
	Number   string `json:"number"`
	TrunkID  string `json:"trunk_id"`
	CallerID string `json:"caller_id"`
	AgentID  string `json:"agent_id"`
}

// RegisterOutboundRoutes adds outbound call API
func (s *SIPServer) RegisterOutboundRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/call/outbound", s.handleOutboundCall)
}

func (s *SIPServer) handleOutboundCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req outboundCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Number == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "number required"})
		return
	}

	// Find outbound trunk
	trunk := s.findOutboundTrunk(req.TrunkID)
	if trunk == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no outbound trunk configured"})
		return
	}

	callerID := req.CallerID
	if callerID == "" {
		callerID = trunk.CallerID
	}
	if callerID == "" {
		callerID = "voiceagent"
	}

	callID := fmt.Sprintf("outbound-%d", time.Now().UnixMilli())
	log := slog.With("call_id", callID, "number", req.Number, "trunk", trunk.Name)
	log.Info("outbound call initiated")

	// Start the outbound call in background
	go s.makeOutboundCall(callID, req.Number, callerID, trunk, req.AgentID)

	writeJSON(w, http.StatusOK, map[string]string{
		"call_id": callID,
		"status":  "dialing",
	})
}

func (s *SIPServer) findOutboundTrunk(trunkID string) *SIPTrunk {
	if s.gw == nil || database == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var query string
	var args []any
	if trunkID != "" {
		query = `SELECT id, name, address, port, transport, caller_id, codecs FROM sip_trunks WHERE id=$1 AND status='active' LIMIT 1`
		args = []any{trunkID}
	} else {
		query = `SELECT id, name, address, port, transport, caller_id, codecs FROM sip_trunks WHERE status='active' LIMIT 1`
	}

	var t SIPTrunk
	err := database.DB().QueryRowContext(ctx, query, args...).Scan(
		&t.ID, &t.Name, &t.Address, &t.Port, &t.Transport, &t.CallerID, &t.Codecs)
	if err != nil {
		return nil
	}
	if t.Port == 0 {
		t.Port = 5060
	}
	return &t
}

func (s *SIPServer) makeOutboundCall(callID, number, callerID string, trunk *SIPTrunk, agentID string) {
	log := slog.With("call_id", callID, "number", number)

	// Allocate local RTP port
	localPort := s.allocateRTPPort()
	localIP := getLocalIP()

	// Create RTP listener
	ctx, cancel := context.WithCancel(context.Background())
	listener, err := NewRTPListener(localPort, "PCMU")
	if err != nil {
		log.Error("RTP listener failed", "err", err)
		cancel()
		return
	}

	// Build SDP offer
	sdp := fmt.Sprintf("v=0\r\n"+
		"o=VoiceAgent %d %d IN IP4 %s\r\n"+
		"s=VoiceAgent\r\n"+
		"c=IN IP4 %s\r\n"+
		"t=0 0\r\n"+
		"m=audio %d RTP/AVP 0 101\r\n"+
		"a=rtpmap:0 PCMU/8000\r\n"+
		"a=rtpmap:101 telephone-event/8000\r\n"+
		"a=fmtp:101 0-16\r\n"+
		"a=sendrecv\r\n"+
		"a=ptime:20\r\n",
		time.Now().UnixMilli(), time.Now().UnixMilli(), localIP,
		localIP, localPort)

	// Build SIP INVITE
	trunkAddr := fmt.Sprintf("%s:%d", trunk.Address, trunk.Port)
	requestURI := fmt.Sprintf("sip:%s@%s", number, trunkAddr)
	fromURI := fmt.Sprintf("<sip:%s@%s>", callerID, localIP)
	toURI := fmt.Sprintf("<sip:%s@%s>", number, trunk.Address)
	contactURI := fmt.Sprintf("<sip:%s@%s%s>", callerID, localIP, s.addr)
	branch := fmt.Sprintf("z9hG4bK-%d", time.Now().UnixNano())
	fromTag := fmt.Sprintf("outbound-%d", time.Now().UnixMilli())

	invite := fmt.Sprintf(
		"INVITE %s SIP/2.0\r\n"+
			"Via: SIP/2.0/UDP %s%s;rport;branch=%s\r\n"+
			"From: %s;tag=%s\r\n"+
			"To: %s\r\n"+
			"Call-ID: %s\r\n"+
			"CSeq: 1 INVITE\r\n"+
			"Contact: %s\r\n"+
			"Max-Forwards: 70\r\n"+
			"Content-Type: application/sdp\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n%s",
		requestURI,
		localIP, s.addr, branch,
		fromURI, fromTag,
		toURI,
		callID,
		contactURI,
		len(sdp), sdp,
	)

	// Send INVITE via UDP
	addr, err := net.ResolveUDPAddr("udp4", trunkAddr)
	if err != nil {
		log.Error("resolve trunk address", "err", err)
		cancel()
		listener.Close()
		return
	}

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		log.Error("dial trunk", "err", err)
		cancel()
		listener.Close()
		return
	}

	log.Info("sending INVITE to trunk", "trunk", trunkAddr)
	conn.Write([]byte(invite))

	// Wait for response
	buf := make([]byte, 4096)
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		log.Error("no response from trunk", "err", err)
		cancel()
		listener.Close()
		conn.Close()
		return
	}

	response := string(buf[:n])
	firstLine := strings.SplitN(response, "\r\n", 2)[0]
	log.Info("trunk response", "response", firstLine)

	// Parse response
	if strings.Contains(firstLine, "100") || strings.Contains(firstLine, "180") || strings.Contains(firstLine, "183") {
		// Provisional — wait for final response
		for {
			conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err = conn.Read(buf)
			if err != nil {
				log.Error("timeout waiting for answer", "err", err)
				cancel()
				listener.Close()
				conn.Close()
				return
			}
			response = string(buf[:n])
			firstLine = strings.SplitN(response, "\r\n", 2)[0]
			log.Info("trunk response", "response", firstLine)

			if strings.Contains(firstLine, "200") {
				break
			}
			if strings.Contains(firstLine, "4") || strings.Contains(firstLine, "5") || strings.Contains(firstLine, "6") {
				log.Info("call rejected", "response", firstLine)
				cancel()
				listener.Close()
				conn.Close()
				return
			}
		}
	}

	if !strings.Contains(firstLine, "200") {
		log.Info("call not answered", "response", firstLine)
		cancel()
		listener.Close()
		conn.Close()
		return
	}

	// Parse remote RTP from response SDP
	remoteRTPAddr := ""
	remoteRTPPort := 0
	if idx := strings.Index(response, "m=audio"); idx >= 0 {
		fmt.Sscanf(response[idx:], "m=audio %d", &remoteRTPPort)
	}
	if idx := strings.Index(response, "c=IN IP4"); idx >= 0 {
		fmt.Sscanf(response[idx:], "c=IN IP4 %s", &remoteRTPAddr)
		remoteRTPAddr = strings.TrimSpace(remoteRTPAddr)
	}

	// Extract To tag for dialog
	toTag := ""
	if idx := strings.Index(response, "To:"); idx >= 0 {
		toLine := response[idx:]
		if tagIdx := strings.Index(toLine, "tag="); tagIdx >= 0 {
			toTag = strings.TrimSpace(strings.SplitN(toLine[tagIdx+4:], "\r\n", 2)[0])
			toTag = strings.Split(toTag, ";")[0]
			toTag = strings.Split(toTag, ">")[0]
		}
	}

	log.Info("outbound call answered",
		"remote_rtp", fmt.Sprintf("%s:%d", remoteRTPAddr, remoteRTPPort),
		"to_tag", toTag)

	// Send ACK
	ack := fmt.Sprintf(
		"ACK %s SIP/2.0\r\n"+
			"Via: SIP/2.0/UDP %s%s;rport;branch=z9hG4bK-%d\r\n"+
			"From: %s;tag=%s\r\n"+
			"To: %s;tag=%s\r\n"+
			"Call-ID: %s\r\n"+
			"CSeq: 1 ACK\r\n"+
			"Max-Forwards: 70\r\n"+
			"Content-Length: 0\r\n"+
			"\r\n",
		requestURI,
		localIP, s.addr, time.Now().UnixNano(),
		fromURI, fromTag,
		toURI, toTag,
		callID,
	)
	conn.Write([]byte(ack))

	// Set remote RTP address
	if remoteRTPAddr != "" && remoteRTPPort > 0 {
		listener.SetRemoteAddr(remoteRTPAddr, remoteRTPPort)
	}

	// Create copilot session
	copilot := getOrCreateSIPRECSession(s.gw, callID)
	copilot.callerNumber = number
	copilot.agentNumber = callerID

	// Store session
	sess := &siprecRTPSession{
		callID:     callID,
		fromTag:    fromTag,
		toTag:      toTag,
		remoteAddr: remoteRTPAddr,
		remotePort: remoteRTPPort,
		codec:      "PCMU",
		localPort:  localPort,
		listener:   listener,
		copilot:    copilot,
		cancelFunc: cancel,
		sipFrom:    fromURI + ";tag=" + fromTag,
		sipTo:      toURI,
		sipCallID:  callID,
		sipSource:  trunkAddr,
	}
	copilot.rtpSession = sess

	s.sessMu.Lock()
	s.sessions[callID+"_caller"] = sess
	s.sessMu.Unlock()

	// Start RTP receiver
	go listener.ReceiveAndDecode(ctx, copilot.pcmCaller, "caller", callID, copilot)

	// Add to queue so agent can pick on Console
	if s.gw.queueMgr != nil {
		s.gw.queueMgr.AddCaller("Outbound", queueEntry{
			ID:       fmt.Sprintf("q-%d", time.Now().UnixNano()),
			CallID:   callID,
			Number:   number,
			Reason:   "Outbound call",
			Priority: "high",
		})
	}

	// Ring the agent who initiated the call
	if agentID != "" && s.gw.acd != nil && s.gw.acd.agentMgr != nil {
		s.gw.acd.agentMgr.RingAgent(agentID, callID, number, "Outbound", 0)
	}

	log.Info("outbound call active, waiting for agent to bridge")

	// Keep connection alive for BYE
	go func() {
		defer conn.Close()
		defer listener.Close()
		defer cancel()

		buf := make([]byte, 4096)
		for {
			conn.SetReadDeadline(time.Now().Add(120 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				log.Info("outbound call connection ended")
				return
			}
			msg := string(buf[:n])
			if strings.Contains(msg, "BYE") {
				log.Info("remote BYE received on outbound call")
				// Send 200 OK for BYE
				byeOK := fmt.Sprintf(
					"SIP/2.0 200 OK\r\n"+
						"Via: %s\r\n"+
						"Call-ID: %s\r\n"+
						"CSeq: 2 BYE\r\n"+
						"Content-Length: 0\r\n"+
						"\r\n",
					extractHeader(msg, "Via"),
					callID,
				)
				conn.Write([]byte(byeOK))

				// Close WebRTC bridge
				bridgeCallID := "bridge-" + callID
				if s.gw.webrtcMgr != nil {
					s.gw.webrtcMgr.mu.Lock()
					if wSess, ok := s.gw.webrtcMgr.sessions[bridgeCallID]; ok {
						wSess.close()
						delete(s.gw.webrtcMgr.sessions, bridgeCallID)
					}
					s.gw.webrtcMgr.mu.Unlock()
				}

				// Clean up
				if s.gw.queueMgr != nil {
					s.gw.queueMgr.RemoveCallerByCallID(callID)
				}
				if copilot != nil {
					copilot.cancel()
				}
				return
			}
		}
	}()
}

func extractHeader(msg, header string) string {
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(line, header+":") || strings.HasPrefix(line, header+" :") {
			return strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		}
	}
	return ""
}

// Ensure sip import is used
var _ sip.Request
