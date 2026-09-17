import unittest
from unittest.mock import AsyncMock, patch

from nova_bridge.server import CALENDAR_FORMAT_INSTRUCTIONS, handle


class EmptyConnection:
    async def recv(self):
        return '{"type":"session.start","systemPrompt":"CUSTOM"}'

    send = AsyncMock()

    def __aiter__(self):
        return self

    async def __anext__(self):
        raise StopAsyncIteration


class CalendarFormatTests(unittest.IsolatedAsyncioTestCase):
    async def test_instructions_reach_session_even_with_custom_prompt(self):
        with patch("nova_bridge.server.NovaSession") as session:
            session.return_value.start = AsyncMock()
            session.return_value.close = AsyncMock()
            await handle(EmptyConnection())
            prompt = session.call_args.kwargs["system_prompt"]
            self.assertEqual(prompt, "CUSTOM" + CALENDAR_FORMAT_INSTRUCTIONS)
            self.assertIn("2026-09-18T10:00:00-03:00", prompt)
            self.assertIn("2026-09-18T11:00:00-03:00", prompt)
            self.assertIn("grava diretamente", prompt)
            self.assertIn("agenda.create_event e notes.create", prompt)
            self.assertIn("sucessos e falhas", prompt)
            self.assertIn("não são uma transação atômica", prompt)
            session.return_value.start.assert_awaited_once()
