// simcopilot simulates a two-party call to test the SIPREC co-pilot feature.
//
// Connects two WebSocket "legs" (caller + agent) to /siprec, sends synthetic
// speech tones, and subscribes to /siprec/events to display real-time
// transcripts and suggestions.
//
// Usage: go run simcopilot.go [http://localhost:8080]
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

const (
	sr       = 16000
	frameMs  = 20
	frameSz  = sr * 2 * frameMs / 1000
	callID   = "test-copilot-001"
)

func main() {
	base := "localhost:8080"
	if len(os.Args) > 1 {
		base = os.Args[1]
	}

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║       SIPREC Co-Pilot Test                   ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	// Subscribe to SSE events in background
	go subscribeEvents(base)
	time.Sleep(500 * time.Millisecond)

	// Connect caller leg
	callerURL := fmt.Sprintf("ws://%s/siprec?role=caller&call_id=%s", base, callID)
	fmt.Println("Connecting caller leg...")
	callerConn, _, err := websocket.DefaultDialer.Dial(callerURL, nil)
	if err != nil {
		fmt.Println("caller dial error:", err)
		os.Exit(1)
	}
	defer callerConn.Close()

	// Connect agent leg
	agentURL := fmt.Sprintf("ws://%s/siprec?role=agent&call_id=%s", base, callID)
	fmt.Println("Connecting agent leg...")
	agentConn, _, err := websocket.DefaultDialer.Dial(agentURL, nil)
	if err != nil {
		fmt.Println("agent dial error:", err)
		os.Exit(1)
	}
	defer agentConn.Close()

	fmt.Println("Both legs connected. Simulating conversation...\n")

	// Simulate caller speaking (3s tone)
	fmt.Println("[Caller speaking for 3 seconds...]")
	sendTone(callerConn, 1000, 3000)

	// Silence to trigger VAD flush
	fmt.Println("[Silence 2s — VAD flush...]")
	sendSilence(callerConn, 2000)

	// Wait for transcription + suggestion
	time.Sleep(5 * time.Second)

	// Simulate agent speaking (2s tone)
	fmt.Println("[Agent speaking for 2 seconds...]")
	sendTone(agentConn, 800, 2000)
	sendSilence(agentConn, 2000)

	time.Sleep(5 * time.Second)

	// Simulate another caller utterance
	fmt.Println("[Caller speaking again for 3 seconds...]")
	sendTone(callerConn, 1200, 3000)
	sendSilence(callerConn, 2000)

	time.Sleep(5 * time.Second)

	// Hang up — trigger summary
	fmt.Println("\n[Hanging up — triggering call summary...]")
	stop, _ := json.Marshal(map[string]string{"type": "stop", "callId": callID})
	callerConn.WriteMessage(websocket.TextMessage, stop)
	agentConn.WriteMessage(websocket.TextMessage, stop)

	// Wait for summary
	time.Sleep(10 * time.Second)
	fmt.Println("\nDone.")
}

func subscribeEvents(base string) {
	url := fmt.Sprintf("http://%s/siprec/events?call_id=%s", base, callID)

	// Retry until the session exists
	for i := 0; i < 10; i++ {
		resp, err := http.Get(url)
		if err != nil || resp.StatusCode != 200 {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		fmt.Println("SSE connected — listening for events:\n")
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if len(line) > 6 && line[:6] == "data: " {
				data := line[6:]
				var evt map[string]any
				if json.Unmarshal([]byte(data), &evt) == nil {
					switch evt["type"] {
					case "transcript":
						fmt.Printf("  [%s]: %s\n", evt["speaker"], evt["text"])
					case "suggestion":
						fmt.Printf("  >>> SUGGESTION (%s): %s\n", evt["category"], evt["suggestion"])
					case "summary":
						fmt.Printf("\n  === CALL SUMMARY ===\n")
						fmt.Printf("  %s\n", evt["summary"])
						if items, ok := evt["action_items"].([]any); ok {
							for _, item := range items {
								fmt.Printf("  - %s\n", item)
							}
						}
						fmt.Printf("  Sentiment: %s\n", evt["sentiment"])
					}
				}
			}
		}
		resp.Body.Close()
		return
	}
	fmt.Println("Could not connect to SSE endpoint")
}

func sendTone(conn *websocket.Conn, freqHz, durationMs int) {
	frames := durationMs / frameMs
	for i := 0; i < frames; i++ {
		frame := make([]byte, frameSz)
		for j := 0; j < frameSz/2; j++ {
			t := float64(i*frameSz/2+j) / float64(sr)
			sample := int16(8000 * math.Sin(2*math.Pi*float64(freqHz)*t))
			binary.LittleEndian.PutUint16(frame[j*2:j*2+2], uint16(sample))
		}
		conn.WriteMessage(websocket.BinaryMessage, frame)
		time.Sleep(time.Duration(frameMs) * time.Millisecond)
	}
}

func sendSilence(conn *websocket.Conn, durationMs int) {
	frames := durationMs / frameMs
	silence := make([]byte, frameSz)
	for i := 0; i < frames; i++ {
		conn.WriteMessage(websocket.BinaryMessage, silence)
		time.Sleep(time.Duration(frameMs) * time.Millisecond)
	}
}
