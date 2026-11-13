from __future__ import annotations

import os
import unittest
from unittest.mock import patch

import dns.flags
import dns.message
import dns.query
import dns.rdatatype
import dns.rrset

import monitor


class ParseTargetsTests(unittest.TestCase):
    def test_parses_named_ipv4_targets_and_custom_port(self) -> None:
        with patch.dict(os.environ, {"TARGETS": "home=192.168.1.2,remote=10.0.0.53:5353"}):
            self.assertEqual(
                monitor.parse_targets(),
                [
                    monitor.Target("home", "192.168.1.2", 53),
                    monitor.Target("remote", "10.0.0.53", 5353),
                ],
            )

    def test_parses_bracketed_ipv6(self) -> None:
        with patch.dict(os.environ, {"TARGETS": "office=[2001:db8::53]:5353"}):
            self.assertEqual(
                monitor.parse_targets(),
                [monitor.Target("office", "2001:db8::53", 5353)],
            )

    def test_rejects_hostnames(self) -> None:
        with patch.dict(os.environ, {"TARGETS": "home=adguard.local"}):
            with self.assertRaisesRegex(ValueError, "IP addresses"):
                monitor.parse_targets()

    def test_rejects_duplicate_endpoints(self) -> None:
        with patch.dict(os.environ, {"TARGETS": "one=192.168.1.2,two=192.168.1.2"}):
            with self.assertRaisesRegex(ValueError, "Duplicate"):
                monitor.parse_targets()


class DnsCheckTests(unittest.TestCase):
    def test_sends_recursive_query_directly_to_target(self) -> None:
        target = monitor.Target("home", "192.168.1.2", 53)

        def successful_reply(request, address, port, timeout):
            self.assertEqual(address, target.address)
            self.assertEqual(port, target.port)
            self.assertEqual(timeout, 2)
            self.assertTrue(request.flags & dns.flags.RD)
            response = dns.message.make_response(request)
            response.answer.append(
                dns.rrset.from_text(
                    "example.com.", 60, "IN", "A", "93.184.216.34"
                )
            )
            return response

        with patch("monitor.dns.query.udp", side_effect=successful_reply):
            self.assertEqual(
                monitor.check_dns(target, "example.com", 2),
                (True, "DNS query successful"),
            )

    def test_rejects_dns_servfail(self) -> None:
        target = monitor.Target("home", "192.168.1.2", 53)

        def servfail(request, *args, **kwargs):
            response = dns.message.make_response(request)
            response.set_rcode(dns.rcode.SERVFAIL)
            return response

        with patch("monitor.dns.query.udp", side_effect=servfail):
            healthy, detail = monitor.check_dns(target, "example.com", 2)

        self.assertFalse(healthy)
        self.assertIn("SERVFAIL", detail)

    def test_reports_dns_timeout_as_failure(self) -> None:
        target = monitor.Target("home", "192.168.1.2", 53)
        with patch("monitor.dns.query.udp", side_effect=dns.exception.Timeout):
            healthy, detail = monitor.check_dns(target, "example.com", 2)

        self.assertFalse(healthy)
        self.assertIn("Timeout", detail)


if __name__ == "__main__":
    unittest.main()
