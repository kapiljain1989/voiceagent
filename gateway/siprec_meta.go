package main

import (
	"encoding/xml"
	"time"
)

// -------------------------------------------------------------------
// Layer 2: Native SIPREC Metadata Parser (RFC 7866)
//
// Parses the binary XML metadata block from a SIPREC SIP INVITE.
// The SBC sends recording metadata describing the session participants,
// their media streams, and the recording context.
//
// This is the specific telecom protocol knowledge that generic AI
// startups lack — parsing these XML blocks correctly is required
// to anchor the dual RTP streams and label speakers properly.
// -------------------------------------------------------------------

// RecordingSession represents RFC 7866 SIPREC metadata.
type RecordingSession struct {
	XMLName      xml.Name               `xml:"recording"`
	DataMode     string                 `xml:"datamode,attr,omitempty"`
	Group        *SessionGroup          `xml:"group,omitempty"`
	Session      *SessionInfo           `xml:"session"`
	Participants []SIPRECParticipant     `xml:"participant"`
	Streams      []SIPRECStream         `xml:"stream"`
}

type SessionGroup struct {
	ID string `xml:"id,attr"`
}

type SessionInfo struct {
	ID        string `xml:"session-id,attr"`
	StartTime string `xml:"start-time,omitempty"`
}

type SIPRECParticipant struct {
	ID        string   `xml:"participant-id,attr"`
	Name      string   `xml:"nameID>aor,omitempty"`
	AOR       string   `xml:"nameID>name,omitempty"`
	SendRecv  string   `xml:"send,omitempty"`
	StartTime string   `xml:"start-time,omitempty"`
}

type SIPRECStream struct {
	ID            string `xml:"stream-id,attr"`
	SessionID     string `xml:"session-id,attr"`
	ParticipantID string `xml:"participant-id,attr"`
	Label         string `xml:"label,omitempty"`
	Mode          string `xml:"mode,omitempty"` // sendonly, recvonly, sendrecv
}

// ParseSIPRECMetadata parses the XML body from a SIPREC SIP INVITE.
func ParseSIPRECMetadata(xmlData []byte) (*RecordingSession, error) {
	var session RecordingSession
	if err := xml.Unmarshal(xmlData, &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// StreamInfo represents a parsed and labeled media stream.
type StreamInfo struct {
	StreamID      string
	ParticipantID string
	Label         string // "caller" or "agent"
	SSRC          uint32
	PayloadType   int
	Codec         string
}

// ExtractStreams converts SIPREC metadata into labeled stream info
// for anchoring the dual RTP streams.
func (rs *RecordingSession) ExtractStreams() []StreamInfo {
	var streams []StreamInfo

	participantNames := make(map[string]string)
	for _, p := range rs.Participants {
		name := p.Name
		if name == "" {
			name = p.AOR
		}
		participantNames[p.ID] = name
	}

	for i, s := range rs.Streams {
		label := s.Label
		if label == "" {
			// Infer label from position: first stream = caller, second = agent
			if i == 0 {
				label = "caller"
			} else {
				label = "agent"
			}
		}

		streams = append(streams, StreamInfo{
			StreamID:      s.ID,
			ParticipantID: s.ParticipantID,
			Label:         label,
		})
	}

	return streams
}

// SIPRECSessionContext holds all the context needed for a recording session.
type SIPRECSessionContext struct {
	SessionID    string
	StartTime    time.Time
	Participants map[string]string // participantID → name/AOR
	Streams      []StreamInfo
	CallerStream *StreamInfo
	AgentStream  *StreamInfo
}

// BuildContext creates a fully resolved session context from metadata.
func (rs *RecordingSession) BuildContext() *SIPRECSessionContext {
	ctx := &SIPRECSessionContext{
		Participants: make(map[string]string),
	}

	if rs.Session != nil {
		ctx.SessionID = rs.Session.ID
		if t, err := time.Parse(time.RFC3339, rs.Session.StartTime); err == nil {
			ctx.StartTime = t
		}
	}

	for _, p := range rs.Participants {
		name := p.Name
		if name == "" {
			name = p.AOR
		}
		ctx.Participants[p.ID] = name
	}

	ctx.Streams = rs.ExtractStreams()

	for i := range ctx.Streams {
		switch ctx.Streams[i].Label {
		case "caller":
			ctx.CallerStream = &ctx.Streams[i]
		case "agent":
			ctx.AgentStream = &ctx.Streams[i]
		}
	}

	return ctx
}
