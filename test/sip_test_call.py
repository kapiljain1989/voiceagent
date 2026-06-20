#!/usr/bin/env python3
"""
Simple SIP+RTP test caller — calls the gateway, sends audio, receives audio.

Usage:
    python3 test/sip_test_call.py [gateway_ip:port] [duration_seconds] [dialed_number]

Example:
    python3 test/sip_test_call.py 127.0.0.1:5062 30
    python3 test/sip_test_call.py gateway:5062 60 +18005550100
"""

import socket
import struct
import threading
import time
import sys
import math
import random
import re

GW_HOST = "127.0.0.1"
GW_PORT = 5062
DURATION = 30
DIALED_NUMBER = "2001"
LOCAL_RTP_PORT = 16000
LOCAL_SIP_PORT = 15060

if len(sys.argv) > 1:
    parts = sys.argv[1].split(":")
    GW_HOST = parts[0]
    if len(parts) > 1:
        GW_PORT = int(parts[1])
if len(sys.argv) > 2:
    DURATION = int(sys.argv[2])
if len(sys.argv) > 3:
    DIALED_NUMBER = sys.argv[3]


def generate_tone(freq, sample_rate=8000, duration_ms=20):
    """Generate a sine wave tone as G.711 μ-law."""
    samples = int(sample_rate * duration_ms / 1000)
    pcm = []
    for i in range(samples):
        t = i / sample_rate
        val = int(16000 * math.sin(2 * math.pi * freq * t))
        pcm.append(val)
    return pcm_to_ulaw(pcm)


def pcm_to_ulaw(samples):
    """Convert 16-bit PCM samples to G.711 μ-law."""
    BIAS = 0x84
    CLIP = 32635
    result = bytearray(len(samples))
    for i, sample in enumerate(samples):
        sign = 0
        if sample < 0:
            sign = 0x80
            sample = -sample
        if sample > CLIP:
            sample = CLIP
        sample += BIAS
        exponent = 7
        mask = 0x4000
        while (sample & mask) == 0 and exponent > 0:
            exponent -= 1
            mask >>= 1
        mantissa = (sample >> (exponent + 3)) & 0x0F
        result[i] = ~(sign | (exponent << 4) | mantissa) & 0xFF
    return bytes(result)


def rtp_sender(sock, remote_addr, remote_port, duration, stop_event):
    """Send RTP PCMU packets with a 440Hz tone."""
    print(f"  RTP TX → {remote_addr}:{remote_port} (440Hz tone, {duration}s)")
    ssrc = random.randint(1000000, 9999999)
    seq = 0
    ts = 0
    tone = generate_tone(440)  # 20ms of 440Hz

    start = time.time()
    while time.time() - start < duration and not stop_event.is_set():
        seq += 1
        ts += 160  # 20ms at 8kHz
        hdr = struct.pack("!BBHII", 0x80, 0, seq & 0xFFFF, ts, ssrc)
        sock.sendto(hdr + tone, (remote_addr, remote_port))
        time.sleep(0.02)

    print(f"  RTP TX done: {seq} packets sent")


def rtp_receiver(sock, duration, stop_event):
    """Receive and count RTP packets."""
    print(f"  RTP RX listening on :{LOCAL_RTP_PORT}")
    sock.settimeout(1)
    count = 0
    start = time.time()
    while time.time() - start < duration and not stop_event.is_set():
        try:
            data, addr = sock.recvfrom(1500)
            count += 1
            if count == 1:
                print(f"  RTP RX: first packet from {addr}, {len(data)} bytes")
            if count % 250 == 0:
                print(f"  RTP RX: {count} packets received")
        except socket.timeout:
            continue
    print(f"  RTP RX done: {count} packets received")


def sip_call():
    """Make a SIP call to the gateway with RTP audio."""
    # Detect own IP (use 0.0.0.0 for bind, resolve actual IP for SDP)
    local_bind = "0.0.0.0"
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect((GW_HOST, GW_PORT))
        local_ip = s.getsockname()[0]
        s.close()
    except Exception:
        local_ip = "127.0.0.1"

    call_id = f"test-{int(time.time())}@{local_ip}"
    from_tag = f"tag-{random.randint(10000, 99999)}"
    branch = f"z9hG4bK-{random.randint(100000, 999999)}"

    # Create SIP socket
    sip_sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sip_sock.bind((local_bind, LOCAL_SIP_PORT))
    sip_sock.settimeout(10)

    # Create RTP socket
    rtp_sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    rtp_sock.bind((local_bind, LOCAL_RTP_PORT))

    print(f"\n{'='*60}")
    print(f"  SIP Test Call to {GW_HOST}:{GW_PORT}")
    print(f"  Local SIP: {local_ip}:{LOCAL_SIP_PORT}")
    print(f"  Local RTP: {local_ip}:{LOCAL_RTP_PORT}")
    print(f"  Duration:  {DURATION}s")
    print(f"{'='*60}\n")

    # Build INVITE
    sdp = (
        f"v=0\r\n"
        f"o=testcaller 1234 1234 IN IP4 {local_ip}\r\n"
        f"s=Test Call\r\n"
        f"c=IN IP4 {local_ip}\r\n"
        f"t=0 0\r\n"
        f"m=audio {LOCAL_RTP_PORT} RTP/AVP 0 101\r\n"
        f"a=rtpmap:0 PCMU/8000\r\n"
        f"a=rtpmap:101 telephone-event/8000\r\n"
        f"a=fmtp:101 0-16\r\n"
        f"a=sendrecv\r\n"
        f"a=ptime:20\r\n"
    )

    invite = (
        f"INVITE sip:{DIALED_NUMBER}@{GW_HOST}:{GW_PORT} SIP/2.0\r\n"
        f"Via: SIP/2.0/UDP {local_ip}:{LOCAL_SIP_PORT};rport;branch={branch}\r\n"
        f"From: <sip:testcaller@{local_ip}>;tag={from_tag}\r\n"
        f"To: <sip:{DIALED_NUMBER}@{GW_HOST}>\r\n"
        f"Call-ID: {call_id}\r\n"
        f"CSeq: 1 INVITE\r\n"
        f"Contact: <sip:testcaller@{local_ip}:{LOCAL_SIP_PORT}>\r\n"
        f"Max-Forwards: 70\r\n"
        f"Content-Type: application/sdp\r\n"
        f"Content-Length: {len(sdp)}\r\n"
        f"\r\n"
        f"{sdp}"
    )

    # Send INVITE
    print("→ INVITE sent")
    sip_sock.sendto(invite.encode(), (GW_HOST, GW_PORT))

    # Wait for responses
    to_tag = None
    remote_rtp_port = None
    remote_rtp_ip = None
    got_200 = False

    for _ in range(10):
        try:
            data, addr = sip_sock.recvfrom(4096)
            msg = data.decode(errors="replace")
            first_line = msg.split("\r\n")[0]
            print(f"← {first_line}")

            if "200 OK" in first_line:
                got_200 = True
                # Extract To tag
                to_match = re.search(r"To:.*?;tag=([^\r\n;]+)", msg)
                if to_match:
                    to_tag = to_match.group(1)

                # Extract RTP port from SDP
                m_match = re.search(r"m=audio (\d+)", msg)
                if m_match:
                    remote_rtp_port = int(m_match.group(1))

                c_match = re.search(r"c=IN IP4 (\S+)", msg)
                if c_match:
                    remote_rtp_ip = c_match.group(1)

                print(f"  Remote RTP: {remote_rtp_ip}:{remote_rtp_port}")

                # Send ACK
                ack_branch = f"z9hG4bK-{random.randint(100000, 999999)}"
                to_hdr = f"<sip:{DIALED_NUMBER}@{GW_HOST}>"
                if to_tag:
                    to_hdr += f";tag={to_tag}"

                ack = (
                    f"ACK sip:{DIALED_NUMBER}@{GW_HOST}:{GW_PORT} SIP/2.0\r\n"
                    f"Via: SIP/2.0/UDP {local_ip}:{LOCAL_SIP_PORT};rport;branch={ack_branch}\r\n"
                    f"From: <sip:testcaller@{local_ip}>;tag={from_tag}\r\n"
                    f"To: {to_hdr}\r\n"
                    f"Call-ID: {call_id}\r\n"
                    f"CSeq: 1 ACK\r\n"
                    f"Max-Forwards: 70\r\n"
                    f"Content-Length: 0\r\n"
                    f"\r\n"
                )
                sip_sock.sendto(ack.encode(), (GW_HOST, GW_PORT))
                print("→ ACK sent")
                break

            elif "100" in first_line:
                continue
            else:
                print(f"  Unexpected: {first_line}")

        except socket.timeout:
            print("  Timeout waiting for response")
            break

    if not got_200:
        print("\n✗ Call failed — no 200 OK received")
        sip_sock.close()
        rtp_sock.close()
        return

    print(f"\n✓ Call established! Streaming audio for {DURATION}s...")
    print(f"  Open http://localhost:3000/console and click PICK\n")

    # Start RTP
    stop = threading.Event()
    tx = threading.Thread(target=rtp_sender, args=(rtp_sock, remote_rtp_ip, remote_rtp_port, DURATION, stop))
    rx = threading.Thread(target=rtp_receiver, args=(rtp_sock, DURATION, stop))
    tx.start()
    rx.start()

    try:
        tx.join()
        stop.set()
        rx.join()
    except KeyboardInterrupt:
        print("\n  Interrupted")
        stop.set()

    # Send BYE
    bye_branch = f"z9hG4bK-{random.randint(100000, 999999)}"
    to_hdr = f"<sip:{DIALED_NUMBER}@{GW_HOST}>"
    if to_tag:
        to_hdr += f";tag={to_tag}"

    bye = (
        f"BYE sip:{DIALED_NUMBER}@{GW_HOST}:{GW_PORT} SIP/2.0\r\n"
        f"Via: SIP/2.0/UDP {local_ip}:{LOCAL_SIP_PORT};rport;branch={bye_branch}\r\n"
        f"From: <sip:testcaller@{local_ip}>;tag={from_tag}\r\n"
        f"To: {to_hdr}\r\n"
        f"Call-ID: {call_id}\r\n"
        f"CSeq: 2 BYE\r\n"
        f"Max-Forwards: 70\r\n"
        f"Content-Length: 0\r\n"
        f"\r\n"
    )
    sip_sock.sendto(bye.encode(), (GW_HOST, GW_PORT))
    print("\n→ BYE sent")

    try:
        data, _ = sip_sock.recvfrom(4096)
        first_line = data.decode(errors="replace").split("\r\n")[0]
        print(f"← {first_line}")
    except:
        pass

    print("\n✓ Call ended")
    sip_sock.close()
    rtp_sock.close()


if __name__ == "__main__":
    sip_call()
