from __future__ import annotations

import asyncio
import base64
import json
import uuid
from collections.abc import Awaitable, Callable
from typing import Any

from aws_sdk_bedrock_runtime.client import (
    BedrockRuntimeClient,
    InvokeModelWithBidirectionalStreamOperationInput,
)
from aws_sdk_bedrock_runtime.config import Config
from aws_sdk_bedrock_runtime.models import (
    BidirectionalInputPayloadPart,
    InvokeModelWithBidirectionalStreamInputChunk,
)
from smithy_aws_core.identity.environment import EnvironmentCredentialsResolver


EventSink = Callable[[dict[str, Any]], Awaitable[None]]


class NovaSession:
    def __init__(
        self,
        region: str,
        model_id: str,
        voice_id: str,
        system_prompt: str,
        event_sink: EventSink,
    ) -> None:
        self.region = region
        self.model_id = model_id
        self.voice_id = voice_id
        self.system_prompt = system_prompt
        self.event_sink = event_sink
        self.prompt_name = str(uuid.uuid4())
        self.system_content_name = str(uuid.uuid4())
        self.audio_content_name = str(uuid.uuid4())
        self.stream: Any | None = None
        self.response_task: asyncio.Task[None] | None = None
        self.active = False

    async def start(self, tools: list[dict[str, Any]] | None = None) -> None:
        client = BedrockRuntimeClient(
            config=Config(
                endpoint_uri=f"https://bedrock-runtime.{self.region}.amazonaws.com",
                region=self.region,
                aws_credentials_identity_resolver=EnvironmentCredentialsResolver(),
            )
        )
        self.stream = await client.invoke_model_with_bidirectional_stream(
            InvokeModelWithBidirectionalStreamOperationInput(model_id=self.model_id)
        )
        self.active = True
        await self._send({
            "event": {"sessionStart": {"inferenceConfiguration": {
                "maxTokens": 1024, "topP": 0.9, "temperature": 0.7
            }}}
        })
        prompt_start: dict[str, Any] = {
            "promptName": self.prompt_name,
            "textOutputConfiguration": {"mediaType": "text/plain"},
            "audioOutputConfiguration": {
                "mediaType": "audio/lpcm",
                "sampleRateHertz": 24000,
                "sampleSizeBits": 16,
                "channelCount": 1,
                "voiceId": self.voice_id,
                "encoding": "base64",
                "audioType": "SPEECH",
            },
        }
        if tools:
            prompt_start["toolUseOutputConfiguration"] = {"mediaType": "application/json"}
            prompt_start["toolConfiguration"] = {"tools": tools}
        await self._send({"event": {"promptStart": prompt_start}})
        await self._send({"event": {"contentStart": {
            "promptName": self.prompt_name,
            "contentName": self.system_content_name,
            "type": "TEXT",
            "interactive": False,
            "role": "SYSTEM",
            "textInputConfiguration": {"mediaType": "text/plain"},
        }}})
        await self._send({"event": {"textInput": {
            "promptName": self.prompt_name,
            "contentName": self.system_content_name,
            "content": self.system_prompt,
        }}})
        await self._send({"event": {"contentEnd": {
            "promptName": self.prompt_name,
            "contentName": self.system_content_name,
        }}})
        await self._send({"event": {"contentStart": {
            "promptName": self.prompt_name,
            "contentName": self.audio_content_name,
            "type": "AUDIO",
            "interactive": True,
            "role": "USER",
            "audioInputConfiguration": {
                "mediaType": "audio/lpcm",
                "sampleRateHertz": 16000,
                "sampleSizeBits": 16,
                "channelCount": 1,
                "audioType": "SPEECH",
                "encoding": "base64",
            },
        }}})
        self.response_task = asyncio.create_task(self._receive())

    async def send_audio(self, encoded_audio: str) -> None:
        base64.b64decode(encoded_audio, validate=True)
        await self._send({"event": {"audioInput": {
            "promptName": self.prompt_name,
            "contentName": self.audio_content_name,
            "content": encoded_audio,
        }}})

    async def send_tool_result(self, tool_use_id: str, content: str) -> None:
        content_name = str(uuid.uuid4())
        await self._send({"event": {"contentStart": {
            "promptName": self.prompt_name,
            "contentName": content_name,
            "type": "TOOL",
            "interactive": False,
            "role": "TOOL",
            "toolResultInputConfiguration": {
                "toolUseId": tool_use_id,
                "type": "TEXT",
                "textInputConfiguration": {"mediaType": "text/plain"},
            },
        }}})
        await self._send({"event": {"textInput": {
            "promptName": self.prompt_name,
            "contentName": content_name,
            "content": content,
        }}})
        await self._send({"event": {"contentEnd": {
            "promptName": self.prompt_name,
            "contentName": content_name,
        }}})

    async def close(self) -> None:
        if not self.active or self.stream is None:
            return
        self.active = False
        await self._send({"event": {"contentEnd": {
            "promptName": self.prompt_name,
            "contentName": self.audio_content_name,
        }}})
        await self._send({"event": {"promptEnd": {"promptName": self.prompt_name}}})
        await self._send({"event": {"sessionEnd": {}}})
        await self.stream.input_stream.close()
        if self.response_task:
            self.response_task.cancel()
            await asyncio.gather(self.response_task, return_exceptions=True)

    async def _send(self, event: dict[str, Any]) -> None:
        if self.stream is None:
            raise RuntimeError("Nova stream has not started")
        payload = json.dumps(event, separators=(",", ":")).encode()
        chunk = InvokeModelWithBidirectionalStreamInputChunk(
            value=BidirectionalInputPayloadPart(bytes_=payload)
        )
        await self.stream.input_stream.send(chunk)

    async def _receive(self) -> None:
        assert self.stream is not None
        while self.active:
            output = await self.stream.await_output()
            result = await output[1].receive()
            if result.value and result.value.bytes_:
                event = json.loads(result.value.bytes_.decode("utf-8"))
                await self.event_sink(event)

