from datetime import datetime, timezone
import unittest

from nova_bridge.server import calendar_context


class CalendarContextTests(unittest.TestCase):
    def test_reference_date_uses_sao_paulo_not_utc(self):
        prompt = calendar_context(datetime(2026, 9, 18, 1, 0, tzinfo=timezone.utc))
        self.assertIn("2026-09-17T22:00:00-03:00", prompt)
        self.assertIn("America/Sao_Paulo", prompt)
        self.assertIn("RFC3339", prompt)
        self.assertIn("Confirmo agendar", prompt)
        self.assertIn("esclareça o ano", prompt)
