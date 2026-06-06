// simcall simulates a FreeSWITCH mod_audio_fork connection to the gateway.
// It sends a metadata frame + a synthetic speech tone (1kHz sine wave)
// followed by silence, then listens for TTS audio coming back.
//
// Usage: go run simcall.go [ws://gateway-url/ws]
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

const (
	sampleRate = 16000
	frameMs    = 20
	frameSize  = sampleRate * 2 * frameMs / 1000 // 640 bytes per frame
)

func main() {
	url := "ws://localhost:8080/ws"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	fmt.Println("connecting to", url)
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		fmt.Println("dial error:", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Send mod_audio_fork metadata frame
	meta := map[string]any{
		"type":       "start",
		"callId":     "test-call-001",
		"streamId":   "test-stream",
		"sampleRate": sampleRate,
		"channels":   1,
	}
	metaJSON, _ := json.Marshal(meta)
	conn.WriteMessage(websocket.TextMessage, metaJSON)
	fmt.Println("sent metadata")

	// Generate 3 seconds of 1kHz sine wave (simulates speech energy)
	speechFrames := 3000 / frameMs // 150 frames = 3 seconds
	fmt.Printf("sending %d frames of speech tone (3s)...\n", speechFrames)
	for i := 0; i < speechFrames; i++ {
		frame := generateTone(1000, sampleRate, frameMs, i)
		conn.WriteMessage(websocket.BinaryMessage, frame)
		time.Sleep(time.Duration(frameMs) * time.Millisecond)
	}

	// Send 2 seconds of silence to trigger VAD flush
	silenceFrames := 2000 / frameMs
	fmt.Printf("sending %d frames of silence (2s) to trigger VAD...\n", silenceFrames)
	silence := make([]byte, frameSize)
	for i := 0; i < silenceFrames; i++ {
		conn.WriteMessage(websocket.BinaryMessage, silence)
		time.Sleep(time.Duration(frameMs) * time.Millisecond)
	}

	fmt.Println("waiting for TTS audio response (up to 30s)...")
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))

	totalBytes := 0
	frames := 0
	for {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			if totalBytes > 0 {
				fmt.Printf("\nreceived %d audio frames (%d bytes, %.1fs of audio)\n",
					frames, totalBytes, float64(totalBytes)/float64(sampleRate*2))
				fmt.Println("SUCCESS — full pipeline working")
			} else {
				fmt.Println("no audio received:", err)
			}
			break
		}
		if mt == websocket.BinaryMessage {
			totalBytes += len(data)
			frames++
			if frames == 1 {
				fmt.Print("receiving audio")
			}
			fmt.Print(".")
		} else if mt == websocket.TextMessage {
			fmt.Println("\ntext frame:", string(data))
		}
	}

	// Send stop event
	stop, _ := json.Marshal(map[string]string{"type": "stop", "callId": "test-call-001"})
	conn.WriteMessage(websocket.TextMessage, stop)
}

func generateTone(freqHz, rate, durationMs, frameIndex int) []byte {
	samples := rate * durationMs / 1000
	buf := make([]byte, samples*2)
	startSample := frameIndex * samples
	for i := 0; i < samples; i++ {
		t := float64(startSample+i) / float64(rate)
		sample := int16(8000 * math.Sin(2*math.Pi*float64(freqHz)*t))
		binary.LittleEndian.PutUint16(buf[i*2:i*2+2], uint16(sample))
	}
	return buf
}
