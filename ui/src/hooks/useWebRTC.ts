"use client";

import { useState, useRef, useCallback, useEffect } from "react";
import { authFetch } from "@/lib/auth";
import type { CallState } from "@/lib/console-types";

const GATEWAY = process.env.NEXT_PUBLIC_GATEWAY_URL || "http://localhost:8080";

export interface RTCStats {
  rtt: number;
  jitter: number;
  packetsLost: number;
  audioLevel: number;
  remoteAudioLevel: number;
  codec: string;
  bytesReceived: number;
  bytesSent: number;
}

export interface UseWebRTCReturn {
  callState: CallState;
  callId: string;
  dial: (number: string, agentId?: string) => Promise<void>;
  bridge: (siprecCallId: string, agentId?: string) => Promise<void>;
  answer: (callId: string) => Promise<void>;
  hangup: () => void;
  mute: () => void;
  unmute: () => void;
  isMuted: boolean;
  sendDTMF: (digit: string) => void;
  stats: RTCStats;
  error: string | null;
}

const defaultStats: RTCStats = {
  rtt: 0, jitter: 0, packetsLost: 0,
  audioLevel: 0, remoteAudioLevel: 0,
  codec: "", bytesReceived: 0, bytesSent: 0,
};

export function useWebRTC(): UseWebRTCReturn {
  const [callState, setCallState] = useState<CallState>("idle");
  const [callId, setCallId] = useState("");
  const [isMuted, setIsMuted] = useState(false);
  const [stats, setStats] = useState<RTCStats>(defaultStats);
  const [error, setError] = useState<string | null>(null);

  const pcRef = useRef<RTCPeerConnection | null>(null);
  const localStreamRef = useRef<MediaStream | null>(null);
  const remoteAudioRef = useRef<HTMLAudioElement | null>(null);
  const statsIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const dtmfSenderRef = useRef<RTCDTMFSender | null>(null);

  // Create hidden audio element for remote playback
  useEffect(() => {
    if (!remoteAudioRef.current) {
      const audio = document.createElement("audio");
      audio.autoplay = true;
      audio.style.display = "none";
      document.body.appendChild(audio);
      remoteAudioRef.current = audio;
    }
    return () => {
      if (remoteAudioRef.current) {
        remoteAudioRef.current.remove();
        remoteAudioRef.current = null;
      }
    };
  }, []);

  const cleanup = useCallback(() => {
    if (statsIntervalRef.current) {
      clearInterval(statsIntervalRef.current);
      statsIntervalRef.current = null;
    }
    if (localStreamRef.current) {
      localStreamRef.current.getTracks().forEach(t => t.stop());
      localStreamRef.current = null;
    }
    if (pcRef.current) {
      pcRef.current.close();
      pcRef.current = null;
    }
    dtmfSenderRef.current = null;
    setStats(defaultStats);
  }, []);

  const collectStats = useCallback(async () => {
    const pc = pcRef.current;
    if (!pc) return;

    try {
      const report = await pc.getStats();
      let rtt = 0, jitter = 0, packetsLost = 0;
      let audioLevel = 0, remoteAudioLevel = 0;
      let codec = "", bytesReceived = 0, bytesSent = 0;

      report.forEach(stat => {
        if (stat.type === "candidate-pair" && stat.state === "succeeded") {
          rtt = stat.currentRoundTripTime ? stat.currentRoundTripTime * 1000 : 0;
        }
        if (stat.type === "inbound-rtp" && stat.kind === "audio") {
          jitter = stat.jitter ? stat.jitter * 1000 : 0;
          packetsLost = stat.packetsLost || 0;
          bytesReceived = stat.bytesReceived || 0;
          remoteAudioLevel = stat.audioLevel ? stat.audioLevel * 100 : 0;
        }
        if (stat.type === "outbound-rtp" && stat.kind === "audio") {
          bytesSent = stat.bytesSent || 0;
        }
        if (stat.type === "codec" && stat.mimeType?.includes("opus")) {
          codec = stat.mimeType;
        }
        if (stat.type === "media-source" && stat.kind === "audio") {
          audioLevel = stat.audioLevel ? stat.audioLevel * 100 : 0;
        }
      });

      setStats({ rtt, jitter, packetsLost, audioLevel, remoteAudioLevel, codec, bytesReceived, bytesSent });
    } catch {}
  }, []);

  const dial = useCallback(async (target: string, agentId?: string) => {
    setError(null);
    setCallState("dialing");

    try {
      // Get microphone
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      localStreamRef.current = stream;

      // Create peer connection
      const pc = new RTCPeerConnection({
        iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
      });
      pcRef.current = pc;

      // Add local audio track
      const audioTrack = stream.getAudioTracks()[0];
      const sender = pc.addTrack(audioTrack, stream);
      dtmfSenderRef.current = sender.dtmf || null;

      // Handle remote audio
      pc.ontrack = (event) => {
        if (remoteAudioRef.current && event.streams[0]) {
          remoteAudioRef.current.srcObject = event.streams[0];
        }
      };

      pc.oniceconnectionstatechange = () => {
        if (pc.iceConnectionState === "connected") {
          setCallState("connected");
        } else if (pc.iceConnectionState === "failed" || pc.iceConnectionState === "disconnected") {
          setCallState("disconnected");
          cleanup();
        }
      };

      // Create and send offer
      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);

      // Wait for ICE gathering
      await new Promise<void>((resolve) => {
        if (pc.iceGatheringState === "complete") { resolve(); return; }
        pc.onicegatheringstatechange = () => {
          if (pc.iceGatheringState === "complete") resolve();
        };
        setTimeout(resolve, 3000);
      });

      const res = await fetch(`${GATEWAY}/api/webrtc/offer`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sdp: pc.localDescription?.sdp,
          type: pc.localDescription?.type,
          agent_id: agentId || "console",
          target,
        }),
      });

      if (!res.ok) {
        throw new Error(`Offer failed: ${res.status}`);
      }

      const answer = await res.json();
      setCallId(answer.call_id);

      await pc.setRemoteDescription({
        type: answer.type as RTCSdpType,
        sdp: answer.sdp,
      });

      // Start stats collection
      statsIntervalRef.current = setInterval(collectStats, 2000);

    } catch (err: any) {
      setError(err.message || "Call failed");
      setCallState("idle");
      cleanup();
    }
  }, [cleanup, collectStats]);

  const bridge = useCallback(async (siprecCallId: string, agentId?: string) => {
    setError(null);
    setCallState("dialing");

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      localStreamRef.current = stream;

      const pc = new RTCPeerConnection({
        iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
      });
      pcRef.current = pc;

      const audioTrack = stream.getAudioTracks()[0];
      pc.addTrack(audioTrack, stream);

      pc.ontrack = (event) => {
        if (remoteAudioRef.current && event.streams[0]) {
          remoteAudioRef.current.srcObject = event.streams[0];
        }
      };

      pc.oniceconnectionstatechange = () => {
        if (pc.iceConnectionState === "connected") {
          setCallState("connected");
        } else if (pc.iceConnectionState === "failed" || pc.iceConnectionState === "disconnected") {
          setCallState("disconnected");
          cleanup();
        }
      };

      const offer = await pc.createOffer();
      await pc.setLocalDescription(offer);

      await new Promise<void>((resolve) => {
        if (pc.iceGatheringState === "complete") { resolve(); return; }
        pc.onicegatheringstatechange = () => {
          if (pc.iceGatheringState === "complete") resolve();
        };
        setTimeout(resolve, 3000);
      });

      const res = await fetch(`${GATEWAY}/api/webrtc/bridge`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          sdp: pc.localDescription?.sdp,
          type: pc.localDescription?.type,
          agent_id: agentId || "console",
          siprec_call_id: siprecCallId,
        }),
      });

      if (!res.ok) throw new Error(`Bridge failed: ${res.status}`);

      const answer = await res.json();
      setCallId(answer.call_id);

      await pc.setRemoteDescription({
        type: answer.type as RTCSdpType,
        sdp: answer.sdp,
      });

      statsIntervalRef.current = setInterval(collectStats, 2000);
    } catch (err: any) {
      setError(err.message || "Bridge failed");
      setCallState("idle");
      cleanup();
    }
  }, [cleanup, collectStats]);

  const answer = useCallback(async (incomingCallId: string) => {
    setError(null);
    setCallId(incomingCallId);
    await dial("answer", "console");
  }, [dial]);

  const hangup = useCallback(() => {
    if (callId) {
      fetch(`${GATEWAY}/api/webrtc/hangup`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ call_id: callId }),
      }).catch(() => {});
    }
    setCallState("idle");
    setCallId("");
    cleanup();
  }, [callId, cleanup]);

  const mute = useCallback(() => {
    if (localStreamRef.current) {
      localStreamRef.current.getAudioTracks().forEach(t => { t.enabled = false; });
      setIsMuted(true);
    }
  }, []);

  const unmute = useCallback(() => {
    if (localStreamRef.current) {
      localStreamRef.current.getAudioTracks().forEach(t => { t.enabled = true; });
      setIsMuted(false);
    }
  }, []);

  const sendDTMF = useCallback((digit: string) => {
    if (dtmfSenderRef.current && dtmfSenderRef.current.canInsertDTMF) {
      dtmfSenderRef.current.insertDTMF(digit, 100, 70);
    }
  }, []);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      cleanup();
    };
  }, [cleanup]);

  return {
    callState, callId, dial, bridge, answer, hangup,
    mute, unmute, isMuted, sendDTMF,
    stats, error,
  };
}
