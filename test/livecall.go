// livecall — real-time voice conversation with the AI gateway.
//
// Captures audio from your microphone, streams it to the gateway,
// plays Claude's TTS response through your speaker, and prints
// the full conversation on the console.
//
// Usage:
//   go run livecall.go [ws://localhost:8080/ws]
//
// Press Ctrl+C to end the call.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
	"github.com/gorilla/websocket"
)

const (
	sampleRate  = 16000
	channels    = 1
	frameMs     = 20
	frameSamples = sampleRate * frameMs / 1000 // 320 samples per frame
)

func main() {
	url := "ws://localhost:8080/ws"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	if err := portaudio.Initialize(); err != nil {
		fmt.Println("portaudio init error:", err)
		os.Exit(1)
	}
	defer portaudio.Terminate()

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║       AI Voice Gateway — Live Call           ║")
	fmt.Println("║  Speak into your microphone. Press Ctrl+C    ║")
	fmt.Println("║  to hang up.                                 ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	// Connect to gateway
	fmt.Printf("Dialing %s ...\n", url)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Println("connection failed:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Send mod_audio_fork metadata
	meta := map[string]any{
		"type":       "start",
		"callId":     fmt.Sprintf("live-%d", time.Now().UnixMilli()),
		"streamId":   "mic",
		"sampleRate": sampleRate,
		"channels":   channels,
	}
	metaJSON, _ := json.Marshal(meta)
	conn.WriteMessage(websocket.TextMessage, metaJSON)
	fmt.Println("Connected. Listening...\n")

	// Playback buffer — audio frames from the gateway queue here
	playBuf := make(chan []int16, 100)

	// Signal handling
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	done := make(chan struct{})

	var wg sync.WaitGroup

	// --- Microphone capture goroutine ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		inBuf := make([]int16, frameSamples)
		stream, err := portaudio.OpenDefaultStream(channels, 0, float64(sampleRate), frameSamples, inBuf)
		if err != nil {
			fmt.Println("mic open error:", err)
			close(done)
			return
		}
		defer stream.Close()

		if err := stream.Start(); err != nil {
			fmt.Println("mic start error:", err)
			close(done)
			return
		}
		defer stream.Stop()

		for {
			select {
			case <-done:
				return
			default:
			}

			if err := stream.Read(); err != nil {
				fmt.Println("mic read error:", err)
				return
			}

			// Convert int16 samples to little-endian bytes
			frame := make([]byte, frameSamples*2)
			for i, s := range inBuf {
				frame[i*2] = byte(s)
				frame[i*2+1] = byte(s >> 8)
			}

			conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				return
			}
		}
	}()

	// --- Speaker playback goroutine ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		outBuf := make([]int16, frameSamples)
		stream, err := portaudio.OpenDefaultStream(0, channels, float64(sampleRate), frameSamples, outBuf)
		if err != nil {
			fmt.Println("speaker open error:", err)
			return
		}
		defer stream.Close()

		if err := stream.Start(); err != nil {
			fmt.Println("speaker start error:", err)
			return
		}
		defer stream.Stop()

		for {
			select {
			case <-done:
				return
			case samples, ok := <-playBuf:
				if !ok {
					return
				}
				// Write in frame-sized chunks
				for off := 0; off < len(samples); off += frameSamples {
					end := off + frameSamples
					if end > len(samples) {
						end = len(samples)
					}
					chunk := samples[off:end]
					// Pad if short
					if len(chunk) < frameSamples {
						padded := make([]int16, frameSamples)
						copy(padded, chunk)
						chunk = padded
					}
					copy(outBuf, chunk)
					if err := stream.Write(); err != nil {
						return
					}
				}
			}
		}
	}()

	// --- WebSocket reader goroutine (receives audio + events) ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(playBuf)

		for {
			select {
			case <-done:
				return
			default:
			}

			mt, data, err := conn.ReadMessage()
			if err != nil {
				select {
				case <-done:
				default:
					fmt.Println("\ndisconnected:", err)
				}
				return
			}

			if mt == websocket.TextMessage {
				var evt struct {
					Event string `json:"event"`
					Text  string `json:"text"`
				}
				if json.Unmarshal(data, &evt) == nil {
					switch evt.Event {
					case "transcript":
						fmt.Printf("\n  YOU:    %s\n", evt.Text)
					case "response":
						fmt.Printf("  CLAUDE: %s\n\n", evt.Text)
					}
				}
				continue
			}

			if mt == websocket.BinaryMessage && len(data) >= 2 {
				// Convert bytes to int16 samples for playback
				samples := make([]int16, len(data)/2)
				for i := range samples {
					samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
				}
				select {
				case playBuf <- samples:
				default:
				}
			}
		}
	}()

	// Wait for Ctrl+C
	select {
	case <-sig:
		fmt.Println("\n\nHanging up...")
	case <-done:
	}

	// Send stop event
	stop, _ := json.Marshal(map[string]string{"type": "stop", "callId": meta["callId"].(string)})
	conn.WriteMessage(websocket.TextMessage, stop)
	close(done)

	// Give goroutines time to clean up
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Call ended.")
}
