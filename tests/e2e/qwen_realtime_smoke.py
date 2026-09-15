#!/usr/bin/env python3
"""Streams a mono PCM16/16 kHz WAV file to vLLM-Omni realtime."""

import argparse
import asyncio
import base64
import json
import os
import wave

import websockets


async def run(url: str, token: str, wav_path: str) -> None:
    with wave.open(wav_path, "rb") as source:
        if (source.getnchannels(), source.getsampwidth(), source.getframerate()) != (1, 2, 16000):
            raise SystemExit("input must be mono PCM16 at 16 kHz")
        pcm = source.readframes(source.getnframes())

    headers = {"Authorization": f"Bearer {token}"}
    async with websockets.connect(url, additional_headers=headers, max_size=64 * 1024 * 1024) as ws:
        await ws.send(json.dumps({"type": "session.update", "model": "Qwen/Qwen3-Omni-30B-A3B-Instruct"}))
        await ws.send(json.dumps({"type": "input_audio_buffer.commit", "final": False}))
        chunk_bytes = 6400  # 200 ms
        for offset in range(0, len(pcm), chunk_bytes):
            chunk = pcm[offset : offset + chunk_bytes]
            await ws.send(json.dumps({
                "type": "input_audio_buffer.append",
                "audio": base64.b64encode(chunk).decode("ascii"),
            }))
            await asyncio.sleep(0.2)
        await ws.send(json.dumps({"type": "input_audio_buffer.commit", "final": True}))

        audio_bytes = 0
        transcript = []
        while True:
            event = json.loads(await asyncio.wait_for(ws.recv(), timeout=120))
            event_type = event.get("type", "")
            if event_type == "error":
                raise RuntimeError(json.dumps(event, ensure_ascii=False))
            if event_type.endswith("output_audio.delta") or event_type.endswith("audio.delta"):
                encoded = event.get("delta") or event.get("audio") or ""
                audio_bytes += len(base64.b64decode(encoded))
            if event_type.endswith("output_text.delta") or event_type.endswith("transcription.delta"):
                transcript.append(event.get("delta", ""))
            if event_type.endswith("response.done") or event_type.endswith("audio.done"):
                break

        if audio_bytes == 0:
            raise RuntimeError("server returned no audio")
        print(json.dumps({"audioBytes": audio_bytes, "text": "".join(transcript)}, ensure_ascii=False))


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("wav", help="mono PCM16 16 kHz WAV input")
    parser.add_argument("--url", default=os.environ.get("STS_QWEN_REALTIME_URL"))
    parser.add_argument("--token", default=os.environ.get("STS_QWEN_API_KEY"))
    args = parser.parse_args()
    if not args.url or not args.token:
        parser.error("set --url/--token or STS_QWEN_REALTIME_URL/STS_QWEN_API_KEY")
    asyncio.run(run(args.url, args.token, args.wav))


if __name__ == "__main__":
    main()

