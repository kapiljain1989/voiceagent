package main

import (
	"context"
	"crypto/tls"
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
	security *SIPSecurity
	// Registration store: username → contact address (host:port)
	registrations map[string]string
	regMu         sync.RWMutex
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
	// SIP dialog state for sending BYE
	sipFrom     string
	sipTo       string
	sipCallID   string
	sipSource   string // where to send BYE
	sipConn     *net.UDPConn // outbound call's UDP connection (reuse for BYE)
	isOutbound  bool
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

	// Initialize SIP security (IP whitelist, digest auth)
	var sipSec *SIPSecurity
	if database != nil {
		sipSec = NewSIPSecurity(database.DB())
	}

	s := &SIPServer{
		gw:       gw,
		ua:       ua,
		server:   server,
		addr:     listenAddr,
		rtpBase:  30000,
		rtpNext:  30000,
		sessions:      make(map[string]*siprecRTPSession),
		security:      sipSec,
		registrations: make(map[string]string),
	}

	server.OnInvite(s.handleInvite)
	server.OnAck(func(req *sip.Request, tx sip.ServerTransaction) {})
	server.OnBye(s.handleBye)
	server.OnOptions(s.handleOptions)
	server.OnRegister(func(req *sip.Request, tx sip.ServerTransaction) {
		user := req.From().Address.User
		contact := req.Contact()
		slog.Info("SIP REGISTER", "from", user, "contact", contact)

		// Store registration: extract contact host:port
		if contact != nil {
			contactAddr := fmt.Sprintf("%s:%d", contact.Address.Host, contact.Address.Port)
			if contact.Address.Port == 0 {
				contactAddr = contact.Address.Host + ":5060"
			}

			s.regMu.Lock()
			s.registrations[user] = contactAddr
			s.regMu.Unlock()
			slog.Info("SIP registration stored", "user", user, "contact", contactAddr)
		}

		res := sip.NewResponseFromRequest(req, 200, "OK", nil)
		res.AppendHeader(sip.NewHeader("Expires", "3600"))
		tx.Respond(res)
	})

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

	// TLS listener (SIP over TLS / SIPS)
	certFile := os.Getenv("SIP_TLS_CERT")
	keyFile := os.Getenv("SIP_TLS_KEY")
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			slog.Error("SIP TLS cert load failed", "err", err, "cert", certFile, "key", keyFile)
		} else {
			tlsConf := &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}
			tlsAddr := os.Getenv("SIP_TLS_ADDR")
			if tlsAddr == "" {
				tlsAddr = ":5061"
			}
			go func() {
				slog.Info("SIP TLS server starting", "addr", tlsAddr)
				if err := s.server.ListenAndServeTLS(context.Background(), "tls", tlsAddr, tlsConf); err != nil {
					slog.Error("SIP TLS server", "err", err)
				}
			}()
			slog.Info("SIP TLS listener enabled", "addr", tlsAddr, "cert", certFile)
		}
	}

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
	sourceIP := extractSourceIP(req)
	slog.Info("SIP INVITE received", "call_id", callID, "from", req.From().Address.User, "source", sourceIP)

	// Security: authenticate the request against configured trunks
	if s.security != nil {
		trunk, code, err := s.security.AuthenticateRequest(req)
		if err != nil {
			if code == 401 {
				// Send digest auth challenge
				params := parseDigestParams("nonce=" + err.Error())
				nonce := params["nonce"]
				realm := params["realm"]
				if realm == "" {
					realm = "voiceagent"
				}
				challenge := sip.NewResponseFromRequest(req, 401, "Unauthorized", nil)
				challenge.AppendHeader(sip.NewHeader("WWW-Authenticate",
					fmt.Sprintf(`Digest realm="%s", nonce="%s", algorithm=MD5, qop="auth"`, realm, nonce)))
				tx.Respond(challenge)
				return
			}
			resp := sip.NewResponseFromRequest(req, code, "Forbidden", nil)
			tx.Respond(resp)
			return
		}
		slog.Info("SIP trunk authenticated", "trunk", trunk.Name, "type", trunk.TrunkType, "policy", trunk.SecurityPolicy)
	}

	// Determine trunk type from authenticated trunk or auto-detect from SIPREC metadata
	trunkType := "direct"
	if s.security != nil {
		trunk, _, _ := s.security.AuthenticateRequest(req)
		if trunk != nil && trunk.TrunkType != "" {
			trunkType = trunk.TrunkType
		}
	}
	// Auto-detect SIPREC from multipart/mixed content with recording metadata (RFC 7866)
	contentType := req.ContentType()
	if contentType != nil && strings.Contains(contentType.Value(), "multipart") {
		if strings.Contains(string(req.Body()), "recording") {
			trunkType = "siprec"
		}
	}

	slog.Info("SIP call mode", "call_id", callID, "trunk_type", trunkType)

	// Send 100 Trying
	trying := sip.NewResponseFromRequest(req, 100, "Trying", nil)
	tx.Respond(trying)

	// Parse body — expect SDP (and optionally SIPREC XML metadata)
	body := string(req.Body())
	var sdpBody string
	var siprecXML string

	if contentType == nil {
		contentType = req.ContentType()
	}
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

	// For Direct trunks: set remote addr for sending agent audio back
	// For SIPREC trunks: only receive (passive observer)
	if trunkType == "direct" {
		listener.SetRemoteAddr(remoteAddr, remotePort)
		copilot.rtpSession = sess
	}

	// Store SIP dialog info for sending BYE later
	if from := req.From(); from != nil {
		sess.sipFrom = from.Value()
		if from.Params != nil {
			if tag, ok := from.Params.Get("tag"); ok {
				sess.fromTag = tag
			}
		}
	}
	if to := req.To(); to != nil {
		sess.sipTo = to.Value()
	}
	sess.sipCallID = callID
	sess.sipSource = req.Source()

	s.sessMu.Lock()
	s.sessions[callID+"_"+role] = sess
	s.sessMu.Unlock()

	// Start RTP receiver → feed into copilot pipeline + audio taps
	pcmCh := copilot.pcmCaller
	if role == "agent" {
		pcmCh = copilot.pcmAgent
	}

	go listener.ReceiveAndDecode(ctx, pcmCh, role, callID, copilot)

	// DID routing — determine which queue/agent based on dialed number
	dialedNumber := ""
	if to := req.To(); to != nil {
		dialedNumber = to.Address.User
	}
	queueName := "Support"
	if role == "caller" && s.gw.didRouter != nil {
		trunkIDForRouting := ""
		if s.security != nil {
			if trunk, _, _ := s.security.AuthenticateRequest(req); trunk != nil {
				trunkIDForRouting = trunk.ID
			}
		}
		destType, destValue, _, ivrID := s.gw.didRouter.MatchDID(dialedNumber, trunkIDForRouting)

		// Run IVR if configured for this DID
		if ivrID != "" && copilot != nil {
			ivrCtx, ivrCancel := context.WithTimeout(context.Background(), 60*time.Second)
			ivrFlow, err := LoadIVRFlow(ivrCtx, ivrID)
			if err == nil && ivrFlow != nil {
				slog.Info("IVR starting", "dialed", dialedNumber, "ivr", ivrFlow.Name)
				ivrDestType, ivrDestValue := RunIVR(ivrCtx, ivrFlow, copilot, s.gw.announcer, slog.With("call_id", callID, "ivr", ivrFlow.Name))
				if ivrDestType == "queue" && ivrDestValue != "" {
					destType = "queue"
					destValue = ivrDestValue
				} else if ivrDestType == "agent" && ivrDestValue != "" {
					destType = "agent"
					destValue = ivrDestValue
				} else if ivrDestType == "hangup" {
					ivrCancel()
					return
				}
			}
			ivrCancel()
		}

		if destType == "queue" && destValue != "" {
			queueName = destValue
			slog.Info("DID routed", "dialed", dialedNumber, "queue", queueName)
		} else if destType == "agent" && destValue != "" {
			slog.Info("DID routed to agent", "dialed", dialedNumber, "agent", destValue)
			queueName = "Direct"
		} else {
			slog.Info("DID no match, using default", "dialed", dialedNumber, "queue", queueName)
		}
	}

	// Add to queue for Console visibility
	if role == "caller" && s.gw.queueMgr != nil {
		callerNum := copilot.callerNumber
		if callerNum == "" {
			callerNum = callID[:12]
		}
		reason := "Incoming call"
		if trunkType == "siprec" {
			reason = "SIPREC observation"
		}
		s.gw.queueMgr.AddCaller(queueName, queueEntry{
			ID:       fmt.Sprintf("q-%d", time.Now().UnixNano()),
			CallID:   callID,
			Number:   callerNum,
			Reason:   reason,
			Priority: "normal",
		})

		// Start queue announcements (TTS position/wait to caller via RTP)
		if trunkType == "direct" && s.gw.announcer != nil {
			s.gw.announcer.StartAnnouncements(callID, queueName, copilot)
		}
	}

	slog.Info("SIP session started",
		"call_id", callID,
		"role", role,
		"type", trunkType,
		"remote", fmt.Sprintf("%s:%d", remoteAddr, remotePort),
		"local_rtp", localPort,
		"codec", codec,
	)

	// Build SDP answer — recvonly for SIPREC, sendrecv for Direct
	sdpAnswer := buildSDPAnswerWithMode(localIP, localPort, codec, trunkType)

	slog.Info("SDP answer", "sdp", sdpAnswer)

	// Send 200 OK with SDP
	okResp := sip.NewResponseFromRequest(req, 200, "OK", []byte(sdpAnswer))
	okResp.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
	okResp.AppendHeader(sip.NewHeader("Contact", fmt.Sprintf("<sip:%s:%s;transport=udp>", localIP, s.addr[1:])))
	okResp.AppendHeader(sip.NewHeader("Allow", "INVITE, ACK, BYE, CANCEL, OPTIONS"))
	okResp.AppendHeader(sip.NewHeader("Supported", "timer"))

	// Capture the To tag generated by sipgo for BYE later
	if to := okResp.To(); to != nil {
		if to.Params != nil {
			if tag, ok := to.Params.Get("tag"); ok {
				sess.toTag = tag
			}
		}
	}

	if err := tx.Respond(okResp); err != nil {
		slog.Error("failed to send 200 OK", "err", err, "call_id", callID)
	} else {
		slog.Info("200 OK sent", "call_id", callID)
	}
}

func (s *SIPServer) handleBye(req *sip.Request, tx sip.ServerTransaction) {
	callID := req.CallID().Value()
	slog.Info("SIP BYE received", "call_id", callID)

	// Broadcast call_ended via SSE BEFORE closing (so Console receives it)
	s.gw.broadcastCallState(callID, "ended")

	s.sessMu.Lock()
	for key, sess := range s.sessions {
		if strings.HasPrefix(key, callID) {
			sess.cancelFunc()
			if sess.listener != nil {
				sess.listener.Close()
			}
			if sess.copilot != nil {
				sess.copilot.cancel()
			}
			delete(s.sessions, key)
		}
	}
	s.sessMu.Unlock()

	// Close WebRTC bridge (so Console shows disconnected)
	bridgeCallID := "bridge-" + callID
	if s.gw.webrtcMgr != nil {
		s.gw.webrtcMgr.mu.Lock()
		if sess, ok := s.gw.webrtcMgr.sessions[bridgeCallID]; ok {
			slog.Info("closing WebRTC bridge on caller BYE", "call_id", bridgeCallID)
			if s.gw.acd != nil && sess.agentID != "" {
				s.gw.acd.OnCallEnd(sess.agentID)
			}
			sess.close()
			delete(s.gw.webrtcMgr.sessions, bridgeCallID)
		}
		s.gw.webrtcMgr.mu.Unlock()
	}

	// Stop announcements
	if s.gw.announcer != nil {
		s.gw.announcer.StopAnnouncements(callID)
	}

	// Remove from queue
	if s.gw.queueMgr != nil {
		s.gw.queueMgr.RemoveCallerByCallID(callID)
	}

	// Clear ACD ringing state
	if s.gw.acd != nil {
		s.gw.acd.mu.Lock()
		delete(s.gw.acd.ringing, callID)
		s.gw.acd.mu.Unlock()
	}

	okResp := sip.NewResponseFromRequest(req, 200, "OK", nil)
	tx.Respond(okResp)
}

// SendBYE sends a SIP BYE to the caller when the agent hangs up from Console.
func (s *SIPServer) SendBYE(callID string) {
	s.sessMu.Lock()
	var sess *siprecRTPSession
	for key, se := range s.sessions {
		if strings.HasPrefix(key, callID) {
			sess = se
			break
		}
	}
	s.sessMu.Unlock()

	if sess == nil || sess.sipSource == "" {
		slog.Debug("SendBYE: no session or source", "call_id", callID)
		return
	}

	localAddr := getLocalIP() + s.addr

	// Build BYE with correct From/To based on call direction
	var byeFrom, byeTo string
	if sess.isOutbound {
		// Outbound: we are the caller — From/To same as INVITE
		byeFrom = sess.sipFrom
		byeTo = fmt.Sprintf("%s;tag=%s", sess.sipTo, sess.toTag)
	} else {
		// Inbound: we are the callee — From/To swapped vs INVITE
		byeFrom = fmt.Sprintf("%s;tag=%s", sess.sipTo, sess.toTag)
		byeTo = fmt.Sprintf("%s;tag=%s", sess.sipFrom, sess.fromTag)
	}

	bye := fmt.Sprintf(
		"BYE sip:%s SIP/2.0\r\n"+
			"Via: SIP/2.0/UDP %s;branch=z9hG4bK-%d\r\n"+
			"From: %s\r\n"+
			"To: %s\r\n"+
			"Call-ID: %s\r\n"+
			"CSeq: 2 BYE\r\n"+
			"Max-Forwards: 70\r\n"+
			"Content-Length: 0\r\n"+
			"\r\n",
		sess.sipSource,
		localAddr, time.Now().UnixNano(),
		byeFrom,
		byeTo,
		sess.sipCallID,
	)

	// Send BYE — use existing connection for outbound calls, new connection for inbound
	if sess.sipConn != nil {
		sess.sipConn.Write([]byte(bye))
		slog.Info("SIP BYE sent via outbound connection", "call_id", callID, "target", sess.sipSource)
	} else {
		addr, err := net.ResolveUDPAddr("udp4", sess.sipSource)
		if err != nil {
			slog.Debug("SendBYE: resolve addr", "err", err, "source", sess.sipSource)
			return
		}
		conn, err := net.DialUDP("udp4", nil, addr)
		if err != nil {
			slog.Debug("SendBYE: dial", "err", err)
			return
		}
		defer conn.Close()
		conn.Write([]byte(bye))
		slog.Info("SIP BYE sent to caller", "call_id", callID, "target", sess.sipSource)
	}
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
	return buildSDPAnswerWithMode(localIP, localPort, codec, "direct")
}

func buildSDPAnswerWithMode(localIP string, localPort int, codec, trunkType string) string {
	pt := "0"
	codecName := "PCMU"
	if codec == "PCMA" {
		pt = "8"
		codecName = "PCMA"
	}

	// SIPREC = passive observer (recvonly), Direct = two-way (sendrecv)
	mode := "sendrecv"
	if trunkType == "siprec" {
		mode = "recvonly"
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
		"a=%s\r\n"+
		"a=ptime:20\r\n",
		time.Now().UnixMilli(), time.Now().UnixMilli(), localIP,
		localIP, localPort, pt, pt, codecName, mode)
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
