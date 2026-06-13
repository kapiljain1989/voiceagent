package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/pion/sdp/v3"
)

type SIPServer struct {
	gw       *gateway
	ua       *sipgo.UserAgent
	server   *sipgo.Server
	addr     string
	rtpBase  int
	rtpMu    sync.Mutex
	rtpNext  int
	sessions map[string]*siprecRTPSession
	sessMu   sync.Mutex
}

type siprecRTPSession struct {
	callID      string
	fromTag     string
	toTag       string
	remoteAddr  string
	remotePort  int
	codec       string
	localPort   int
	listener    *RTPListener
	copilot     *siprecSession
	cancelFunc  context.CancelFunc
}

func NewSIPServer(gw *gateway, listenAddr string) (*SIPServer, error) {
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("VoiceAgent/1.0"))
	if err != nil {
		return nil, fmt.Errorf("sipgo ua: %w", err)
	}

	server, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, fmt.Errorf("sipgo server: %w", err)
	}

	s := &SIPServer{
		gw:       gw,
		ua:       ua,
		server:   server,
		addr:     listenAddr,
		rtpBase:  30000,
		rtpNext:  30000,
		sessions: make(map[string]*siprecRTPSession),
	}

	server.OnInvite(s.handleInvite)
	server.OnAck(func(req *sip.Request, tx sip.ServerTransaction) {})
	server.OnBye(s.handleBye)
	server.OnOptions(s.handleOptions)

	return s, nil
}

func (s *SIPServer) Start() error {
	slog.Info("SIP server starting", "addr", s.addr)

	go func() {
		if err := s.server.ListenAndServe(context.Background(), "udp", s.addr); err != nil {
			slog.Error("SIP UDP server", "err", err)
		}
	}()

	go func() {
		if err := s.server.ListenAndServe(context.Background(), "tcp", s.addr); err != nil {
			slog.Error("SIP TCP server", "err", err)
		}
	}()

	slog.Info("SIP server listening", "addr", s.addr, "rtp_range", fmt.Sprintf("%d-%d", s.rtpBase, s.rtpBase+100))
	return nil
}

func (s *SIPServer) allocateRTPPort() int {
	s.rtpMu.Lock()
	defer s.rtpMu.Unlock()
	port := s.rtpNext
	s.rtpNext += 2
	if s.rtpNext > s.rtpBase+100 {
		s.rtpNext = s.rtpBase
	}
	return port
}

func (s *SIPServer) handleOptions(req *sip.Request, tx sip.ServerTransaction) {
	res := sip.NewResponseFromRequest(req, 200, "OK", nil)
	tx.Respond(res)
}

func (s *SIPServer) handleInvite(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	slog.Info("SIPREC INVITE received", "call_id", callID, "from", req.From().Address.User)

	// Send 100 Trying
	trying := sip.NewResponseFromRequest(req, 100, "Trying", nil)
	tx.Respond(trying)

	// Parse body — expect SDP (and optionally SIPREC XML metadata)
	body := string(req.Body())
	var sdpBody string
	var siprecXML string

	contentType := req.ContentType()
	if contentType != nil && strings.Contains(contentType.Value(), "multipart") {
		sdpBody, siprecXML = parseMultipartBody(body, contentType.Value())
	} else {
		sdpBody = body
	}

	// Parse SDP to extract remote RTP port and codec
	remoteAddr, remotePort, codec, err := parseSDPOffer(sdpBody)
	if err != nil {
		slog.Error("SDP parse failed", "err", err, "call_id", callID)
		errResp := sip.NewResponseFromRequest(req, 488, "Not Acceptable Here", nil)
		tx.Respond(errResp)
		return
	}

	// If remoteAddr is 0.0.0.0, use the SIP source IP
	if remoteAddr == "0.0.0.0" {
		src := req.Source()
		if host, _, err := net.SplitHostPort(src); err == nil {
			remoteAddr = host
		}
	}

	// Parse SIPREC metadata if present
	var role string
	if siprecXML != "" {
		meta, err := ParseSIPRECMetadata([]byte(siprecXML))
		if err == nil && len(meta.Streams) > 0 {
			role = meta.Streams[0].Label
			slog.Info("SIPREC metadata parsed", "call_id", callID, "streams", len(meta.Streams), "role", role)
		}
	}
	if role == "" {
		role = "caller"
	}

	// Allocate local RTP port
	localPort := s.allocateRTPPort()
	localIP := getLocalIP()
	slog.Info("SDP answer IP", "local_ip", localIP, "local_rtp", localPort)

	// Create RTP listener
	ctx, cancel := context.WithCancel(context.Background())
	listener, err := NewRTPListener(localPort, codec)
	if err != nil {
		slog.Error("RTP listener failed", "err", err, "port", localPort)
		cancel()
		errResp := sip.NewResponseFromRequest(req, 500, "RTP Setup Failed", nil)
		tx.Respond(errResp)
		return
	}

	// Get or create copilot session
	copilot := getOrCreateSIPRECSession(s.gw, callID)

	// Extract caller/agent identity from SIP headers and SIPREC metadata
	if from := req.From(); from != nil && from.Address.User != "" {
		copilot.callerNumber = from.Address.User
	}
	if to := req.To(); to != nil && to.Address.User != "" {
		copilot.agentNumber = to.Address.User
	}
	if siprecXML != "" {
		if meta, err := ParseSIPRECMetadata([]byte(siprecXML)); err == nil {
			for _, p := range meta.Participants {
				if p.AOR != "" {
					for _, st := range meta.Streams {
						if st.ParticipantID == p.ID {
							if st.Label == "caller" || st.Label == "" {
								copilot.callerNumber = p.AOR
							} else {
								copilot.agentNumber = p.AOR
							}
						}
					}
				}
			}
		}
	}

	// Store session
	sess := &siprecRTPSession{
		callID:     callID,
		remoteAddr: remoteAddr,
		remotePort: remotePort,
		codec:      codec,
		localPort:  localPort,
		listener:   listener,
		copilot:    copilot,
		cancelFunc: cancel,
	}

	// Set remote RTP address for sending agent audio back
	listener.SetRemoteAddr(remoteAddr, remotePort)

	// Store RTP session reference on copilot for WebRTC bridge
	copilot.rtpSession = sess

	s.sessMu.Lock()
	s.sessions[callID+"_"+role] = sess
	s.sessMu.Unlock()

	// Start RTP receiver → feed into copilot pipeline + audio taps
	pcmCh := copilot.pcmCaller
	if role == "agent" {
		pcmCh = copilot.pcmAgent
	}

	go listener.ReceiveAndDecode(ctx, pcmCh, role, callID, copilot)

	// Auto-add to queue for Console visibility
	if role == "caller" && s.gw.queueMgr != nil {
		callerNum := copilot.callerNumber
		if callerNum == "" {
			callerNum = callID[:12]
		}
		s.gw.queueMgr.AddCaller("Support", queueEntry{
			ID:       fmt.Sprintf("q-%d", time.Now().UnixNano()),
			CallID:   callID,
			Number:   callerNum,
			Reason:   "Incoming call",
			Priority: "normal",
		})
	}

	slog.Info("SIP session started",
		"call_id", callID,
		"role", role,
		"remote", fmt.Sprintf("%s:%d", remoteAddr, remotePort),
		"local_rtp", localPort,
		"codec", codec,
	)

	// Build SDP answer
	sdpAnswer := buildSDPAnswer(localIP, localPort, codec)

	slog.Info("SDP answer", "sdp", sdpAnswer)

	// Send 200 OK with SDP
	okResp := sip.NewResponseFromRequest(req, 200, "OK", []byte(sdpAnswer))
	okResp.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
	if err := tx.Respond(okResp); err != nil {
		slog.Error("failed to send 200 OK", "err", err, "call_id", callID)
	} else {
		slog.Info("200 OK sent", "call_id", callID)
	}
}

func (s *SIPServer) handleBye(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	slog.Info("SIP BYE received", "call_id", callID)

	s.sessMu.Lock()
	for key, sess := range s.sessions {
		if strings.HasPrefix(key, callID) {
			sess.cancelFunc()
			if sess.listener != nil {
				sess.listener.Close()
			}
			// Cancel the copilot session
			if sess.copilot != nil {
				sess.copilot.cancel()
			}
			delete(s.sessions, key)
		}
	}
	s.sessMu.Unlock()

	// Remove from queue
	if s.gw.queueMgr != nil {
		s.gw.queueMgr.RemoveCallerByCallID(callID)
	}

	okResp := sip.NewResponseFromRequest(req, 200, "OK", nil)
	tx.Respond(okResp)
}

func (s *SIPServer) ActiveSessions() int {
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	return len(s.sessions)
}

// --- SDP Parsing ---

func parseSDPOffer(body string) (addr string, port int, codec string, err error) {
	sd := &sdp.SessionDescription{}
	if err := sd.UnmarshalString(body); err != nil {
		return "", 0, "", fmt.Errorf("sdp unmarshal: %w", err)
	}

	if len(sd.MediaDescriptions) == 0 {
		return "", 0, "", fmt.Errorf("no media descriptions in SDP")
	}

	media := sd.MediaDescriptions[0]
	port = media.MediaName.Port.Value

	// Get connection address
	if media.ConnectionInformation != nil {
		addr = media.ConnectionInformation.Address.Address
	} else if sd.ConnectionInformation != nil {
		addr = sd.ConnectionInformation.Address.Address
	}

	// Determine codec from first payload type
	codec = "PCMU"
	if len(media.MediaName.Formats) > 0 {
		switch media.MediaName.Formats[0] {
		case "0":
			codec = "PCMU"
		case "8":
			codec = "PCMA"
		}
	}

	return addr, port, codec, nil
}

func buildSDPAnswer(localIP string, localPort int, codec string) string {
	pt := "0"
	codecName := "PCMU"
	if codec == "PCMA" {
		pt = "8"
		codecName = "PCMA"
	}

	return fmt.Sprintf("v=0\r\n"+
		"o=VoiceAgent %d %d IN IP4 %s\r\n"+
		"s=VoiceAgent\r\n"+
		"c=IN IP4 %s\r\n"+
		"t=0 0\r\n"+
		"m=audio %d RTP/AVP %s 101\r\n"+
		"a=rtpmap:%s %s/8000\r\n"+
		"a=rtpmap:101 telephone-event/8000\r\n"+
		"a=fmtp:101 0-16\r\n"+
		"a=sendrecv\r\n"+
		"a=ptime:20\r\n",
		time.Now().UnixMilli(), time.Now().UnixMilli(), localIP,
		localIP, localPort, pt, pt, codecName)
}

func parseMultipartBody(body, contentType string) (sdpPart, xmlPart string) {
	// Extract boundary from content-type
	boundary := ""
	for _, p := range strings.Split(contentType, ";") {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "boundary=") {
			boundary = strings.Trim(strings.TrimPrefix(p, "boundary="), "\"")
			break
		}
	}

	if boundary == "" {
		return body, ""
	}

	parts := strings.Split(body, "--"+boundary)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "--" {
			continue
		}

		lower := strings.ToLower(part)
		// Split headers from body
		idx := strings.Index(part, "\r\n\r\n")
		if idx < 0 {
			idx = strings.Index(part, "\n\n")
		}
		if idx < 0 {
			continue
		}
		partBody := strings.TrimSpace(part[idx:])

		if strings.Contains(lower, "application/sdp") {
			sdpPart = partBody
		} else if strings.Contains(lower, "application/rs-metadata") || strings.Contains(lower, "xml") {
			xmlPart = partBody
		}
	}

	return sdpPart, xmlPart
}

func getLocalIP() string {
	// Prefer explicit external IP from env (required for Docker/NAT)
	if ext := os.Getenv("EXTERNAL_IP"); ext != "" {
		return ext
	}
	if ext := os.Getenv("EXT_IP"); ext != "" {
		return ext
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "0.0.0.0"
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "0.0.0.0"
}
