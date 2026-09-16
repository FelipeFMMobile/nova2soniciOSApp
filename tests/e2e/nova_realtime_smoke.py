#!/usr/bin/env python3
"""Streams a WAV file through the local Nova bridge and requires audio output."""

import argparse
import asyncio
import base64
import json
import os
import wave
from pathlib import Path

import websockets


async def run(url: str, wav_path: str, output_path: str) -> None:
    with wave.open(wav_path, "rb") as source:
        if (source.getnchannels(), source.getsampwidth(), source.getframerate()) != (1, 2, 16000):
            raise SystemExit("input must be mono PCM16 at 16 kHz")
        pcm = source.readframes(source.getnframes())

    async with websockets.connect(url, max_size=8 * 1024 * 1024) as socket:
        await socket.send(json.dumps({
            "type": "session.start",
            "voiceId": "carolina",
            "systemPrompt": "Responda sempre em português brasileiro de forma breve.",
        }))
        startup_events: list[str] = []
        while True:
            ready = json.loads(await asyncio.wait_for(socket.recv(), timeout=30))
            if ready.get("type") == "session.ready":
                break
            if ready.get("type") == "error":
                raise RuntimeError(ready.get("message"))
            payload = ready.get("payload", {}).get("event", {})
            if ready.get("type") == "nova.event" and payload:
                startup_events.extend(payload.keys())
                continue
            raise RuntimeError(
                f"unexpected event before session.ready: {ready}; "
                f"startup events: {startup_events}"
            )

        # Nova expects roughly 32 ms of real-time audio per frame. At 16 kHz,
        # mono PCM16, that is 512 samples / 1024 bytes.
        chunk_bytes = 1024
        trailing_silence = bytes(16000 * 2)
        realtime_audio = pcm + trailing_silence
        for offset in range(0, len(realtime_audio), chunk_bytes):
            await socket.send(json.dumps({
                "type": "audio.append",
                "audio": base64.b64encode(
                    realtime_audio[offset : offset + chunk_bytes]
                ).decode("ascii"),
            }))
            await asyncio.sleep(0.032)

        output_audio = bytearray()
        audio_finished = False
        received_events: list[str] = []
        while not audio_finished:
            try:
                message = json.loads(await asyncio.wait_for(socket.recv(), timeout=60))
            except TimeoutError as error:
                raise RuntimeError(
                    f"Nova returned no audio; received events: {received_events}"
                ) from error
            if message.get("type") == "error":
                raise RuntimeError(message.get("message"))
            payload = message.get("payload", {}).get("event", {})
            received_events.extend(payload.keys())
            if "bridgeError" in payload:
                raise RuntimeError(payload["bridgeError"]["message"])
            if "audioOutput" in payload:
                output_audio.extend(base64.b64decode(payload["audioOutput"]["content"]))
            if payload.get("contentEnd", {}).get("type") == "AUDIO":
                audio_finished = True
            if "completionEnd" in payload and not output_audio:
                raise RuntimeError(
                    f"Nova completed without audio; received events: {received_events}"
                )

        await socket.send(json.dumps({"type": "session.stop"}))
        destination = Path(output_path).expanduser().resolve()
        destination.parent.mkdir(parents=True, exist_ok=True)
        with wave.open(str(destination), "wb") as output:
            output.setnchannels(1)
            output.setsampwidth(2)
            output.setframerate(24000)
            output.writeframes(output_audio)
        print(json.dumps({
            "audioBytes": len(output_audio),
            "durationSeconds": round(len(output_audio) / (24000 * 2), 3),
            "output": str(destination),
        }))


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("wav")
    parser.add_argument("--url", default=os.getenv("STS_NOVA_BRIDGE_URL", "ws://127.0.0.1:8091"))
    parser.add_argument("--output", default="/private/tmp/sts-nova-response.wav")
    args = parser.parse_args()
    asyncio.run(run(args.url, args.wav, args.output))


if __name__ == "__main__":
    main()
