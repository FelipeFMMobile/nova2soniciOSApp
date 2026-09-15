import asyncio
import json
import unittest
from unittest.mock import patch

from nova_bridge.session import NovaSession


class FakeInputStream:
    def __init__(self) -> None:
        self.events: list[dict] = []
        self.closed = False

    async def send(self, chunk) -> None:
        self.events.append(json.loads(chunk.value.bytes_.decode("utf-8")))

    async def close(self) -> None:
        self.closed = True


class FakeStream:
    def __init__(self) -> None:
        self.input_stream = FakeInputStream()
        self.output_stream = FakeOutputStream()

    async def await_output(self):
        return object(), self.output_stream


class FakeOutputStream:
    async def receive(self):
        await asyncio.Future()


class FakeClient:
    stream = FakeStream()

    def __init__(self, config) -> None:
        self.config = config

    async def invoke_model_with_bidirectional_stream(self, request):
        self.request = request
        return self.stream


class FakeCredentials:
    access_key = "test-access-key"
    secret_key = "test-secret-key"
    token = "test-session-token"

    def get_frozen_credentials(self):
        return self


class FakeBotoSession:
    def __init__(self, profile_name=None) -> None:
        self.profile_name = profile_name

    def get_credentials(self):
        return FakeCredentials()


class NovaSessionTests(unittest.IsolatedAsyncioTestCase):
    async def test_session_uses_pt_br_voice_and_audio_contract(self) -> None:
        FakeClient.stream = FakeStream()
        session = NovaSession(
            region="us-east-1",
            model_id="amazon.nova-2-sonic-v1:0",
            voice_id="carolina",
            system_prompt="Responda em português brasileiro.",
            event_sink=self._ignore,
        )

        with (
            patch("nova_bridge.session.BedrockRuntimeClient", FakeClient),
            patch("nova_bridge.session.boto3.Session", FakeBotoSession),
        ):
            await session.start()
            events = FakeClient.stream.input_stream.events
            prompt = events[1]["event"]["promptStart"]
            audio = events[5]["event"]["contentStart"]["audioInputConfiguration"]
            self.assertEqual(prompt["audioOutputConfiguration"]["voiceId"], "carolina")
            self.assertEqual(prompt["audioOutputConfiguration"]["sampleRateHertz"], 24000)
            self.assertEqual(audio["sampleRateHertz"], 16000)

            await session.send_audio("AAAAAA==")
            self.assertIn("audioInput", events[-1]["event"])
            await session.close()
            self.assertTrue(FakeClient.stream.input_stream.closed)

    async def test_rejects_invalid_base64_audio(self) -> None:
        session = NovaSession(
            region="us-east-1",
            model_id="amazon.nova-2-sonic-v1:0",
            voice_id="carolina",
            system_prompt="pt-BR",
            event_sink=self._ignore,
        )
        session.stream = FakeStream()
        session.active = True
        with self.assertRaises(ValueError):
            await session.send_audio("not-base64")

    async def _ignore(self, event) -> None:
        return None


if __name__ == "__main__":
    unittest.main()
