package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Minimal rtpproxy-compatible UDP relay for NAT traversal.
// Implements the rtpproxy control protocol (cookie-prefixed commands).
// Kamailio's rtpproxy module sends: "<cookie> <command> <args...>\n"
// Response must be: "<cookie> <result>\n"

const (
	listenIP   = "0.0.0.0"
	basePort   = 40000
	maxPorts   = 100
	sessionTTL = 5 * time.Minute
)

type session struct {
	callerPort int
	gwPort     int
	callerConn *net.UDPConn
	gwConn     *net.UDPConn
	callerAddr *net.UDPAddr
	gwAddr     *net.UDPAddr
	created    time.Time
	gotOffer   bool // first U = offer (caller side), second U = answer (gw side)
}

var (
	sessions = make(map[string]*session)
	mu       sync.Mutex
	nextPort = basePort
	extIP    string
)

func main() {
	extIP = os.Getenv("EXTERNAL_IP")
	if extIP == "" {
		extIP = "192.168.1.156"
	}
	ctrlAddr := "0.0.0.0:7722"
	if len(os.Args) > 1 {
		ctrlAddr = os.Args[1]
	}

	conn, err := net.ListenPacket("udp4", ctrlAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Printf("RTP relay control=%s media=%s ports=%d-%d", ctrlAddr, extIP, basePort, basePort+maxPorts)

	buf := make([]byte, 4096)
	for {
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			continue
		}
		cmd := strings.TrimSpace(string(buf[:n]))
		resp := handleCommand(cmd)
		if resp != "" {
			conn.WriteTo([]byte(resp+"\n"), addr)
		}
	}
}

func handleCommand(raw string) string {
	// Protocol: "<cookie> <command> [args...]"
	parts := strings.Fields(raw)
	if len(parts) < 1 {
		return ""
	}

	// Single-word command (no cookie) — legacy
	if len(parts) == 1 {
		switch parts[0] {
		case "V":
			return "20040107"
		case "VF":
			return "1"
		default:
			return "E1"
		}
	}

	cookie := parts[0]
	cmdFull := parts[1]
	cmd := cmdFull[0] // first char is the command letter

	switch cmd {
	case 'V':
		if len(cmdFull) > 1 && cmdFull[1] == 'F' {
			return cookie + " 1"
		}
		return cookie + " 20040107"
	case 'U', 'u':
		return handleUpdate(cookie, parts[2:])
	case 'D', 'd':
		return handleDelete(cookie, parts[2:])
	case 'L', 'l':
		return handleLookup(cookie, parts[2:])
	case 'I':
		mu.Lock()
		n := len(sessions)
		mu.Unlock()
		return fmt.Sprintf("%s sessions: %d", cookie, n)
	case 'Q':
		return cookie + " 0"
	default:
		log.Printf("unknown cmd: %c full: %s", cmd, raw)
		return cookie + " E1"
	}
}

func handleUpdate(cookie string, args []string) string {
	if len(args) < 2 {
		return cookie + " E1"
	}
	callID := args[0]
	// args[1] = addr, args[2] = port, args[3] = from-tag, args[4] = to-tag (optional)

	mu.Lock()
	defer mu.Unlock()

	sess, exists := sessions[callID]
	if !exists {
		// First U (offer) — allocate caller-side port
		callerPort := allocPort()
		gwPort := allocPort()

		callerConn := listenUDP(callerPort)
		gwConn := listenUDP(gwPort)
		if callerConn == nil || gwConn == nil {
			return cookie + " E7"
		}

		sess = &session{
			callerPort: callerPort,
			gwPort:     gwPort,
			callerConn: callerConn,
			gwConn:     gwConn,
			created:    time.Now(),
			gotOffer:   true,
		}
		sessions[callID] = sess

		go relayLoop(sess.callerConn, sess, true, callID)
		go relayLoop(sess.gwConn, sess, false, callID)

		log.Printf("NEW session %s caller=:%d gw=:%d", callID, callerPort, gwPort)
		return fmt.Sprintf("%s %d %s", cookie, callerPort, extIP)
	}

	// Second U (answer) — return gw-side port
	log.Printf("UPDATE session %s → gw port %d", callID, sess.gwPort)
	return fmt.Sprintf("%s %d %s", cookie, sess.gwPort, extIP)
}

func handleDelete(cookie string, args []string) string {
	if len(args) < 1 {
		return cookie + " E1"
	}
	callID := args[0]

	mu.Lock()
	sess, exists := sessions[callID]
	if exists {
		sess.callerConn.Close()
		sess.gwConn.Close()
		delete(sessions, callID)
		log.Printf("DELETE session %s", callID)
	}
	mu.Unlock()

	return cookie + " 0"
}

func handleLookup(cookie string, args []string) string {
	if len(args) < 1 {
		return cookie + " E8"
	}
	callID := args[0]

	mu.Lock()
	sess, exists := sessions[callID]
	mu.Unlock()

	if !exists {
		return cookie + " E8"
	}
	return fmt.Sprintf("%s %d %s", cookie, sess.gwPort, extIP)
}

func listenUDP(port int) *net.UDPConn {
	addr, _ := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", listenIP, port))
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		log.Printf("listen :%d failed: %v", port, err)
		return nil
	}
	return conn
}

func relayLoop(conn *net.UDPConn, sess *session, isCallerSide bool, callID string) {
	buf := make([]byte, 1500)
	count := 0
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		count++

		if isCallerSide {
			if sess.callerAddr == nil {
				sess.callerAddr = addr
				log.Printf("session %s: caller RTP from %s", callID, addr)
			}
			if sess.gwAddr != nil {
				sess.gwConn.WriteTo(buf[:n], sess.gwAddr)
			}
		} else {
			if sess.gwAddr == nil {
				sess.gwAddr = addr
				log.Printf("session %s: gateway RTP from %s", callID, addr)
			}
			if sess.callerAddr != nil {
				sess.callerConn.WriteTo(buf[:n], sess.callerAddr)
			}
		}
	}
}

func allocPort() int {
	p := nextPort
	nextPort += 2
	if nextPort >= basePort+maxPorts {
		nextPort = basePort
	}
	return p
}
