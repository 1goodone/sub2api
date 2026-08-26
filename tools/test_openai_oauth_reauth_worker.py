import json
import os
import threading
import time
import unittest
import base64
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest.mock import patch
from types import SimpleNamespace

from tools.openai_oauth_reauth_worker import (
    OTPURLProvider,
    WorkerConfig,
    WorkerError,
    _auth_session_flow,
    _email_submission_state,
    _has_passwordless_intent,
    _start_passwordless_login,
    _is_local_codex_callback,
    process_claim,
    _wait_for_otp,
    extract_otp,
    sanitize_error,
    validate_otp_url,
)


class OTPHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({"code": "482931", "subject": "ChatGPT login code"}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, _format, *_args):
        pass


class OpenAIOAuthReauthWorkerTest(unittest.TestCase):
    def test_auth_session_flow_exposes_only_safe_state(self):
        payload = {
            "email": "mail@example.com",
            "session_id": "secret-session",
            "email_verification_mode": "passwordless_login",
            "passwordless_disabled": False,
            "signup_mode": "email_login",
        }
        encoded = base64.urlsafe_b64encode(json.dumps(payload).encode()).decode().rstrip("=")
        cookie = SimpleNamespace(name="oai-client-auth-session", value=f"{encoded}.signature")
        session = SimpleNamespace(session=SimpleNamespace(cookies=SimpleNamespace(jar=[cookie])))

        self.assertEqual(
            _auth_session_flow(session),
            {
                "email_verification_mode": "passwordless_login",
                "passwordless_disabled": False,
                "signup_mode": "email_login",
            },
        )

    def test_email_submission_state_keeps_only_page_type_and_path(self):
        self.assertEqual(
            _email_submission_state(
                {
                    "continue_url": "https://auth.openai.com/log-in/password?secret=value",
                    "page": {"type": "login_password"},
                    "email": "mail@example.com",
                }
            ),
            ("login_password", "/log-in/password"),
        )

    def test_passwordless_intent_is_detected_without_reading_form_values(self):
        html = """
        <form method="post" action="/log-in/password">
          <input type="hidden" name="username" value="mail@example.com">
          <input type="password" name="current-password" value="must-not-submit">
          <button name="intent" value="passwordless_login_send_otp">Use code</button>
        </form>
        """
        self.assertTrue(_has_passwordless_intent(html))
        self.assertFalse(_has_passwordless_intent(html.replace("passwordless_login_send_otp", "password_login")))

    def test_passwordless_login_posts_empty_passwordless_request(self):
        calls = []

        class Response:
            def __init__(self, status_code, url, text=""):
                self.status_code = status_code
                self.url = url
                self.text = text

        class Session:
            def get_auth_navigate_headers(self, *, referer):
                return {"referer": referer}

            def get_auth_headers(self, *, referer):
                return {"referer": referer}

            def get(self, url, *, headers, allow_redirects):
                calls.append(("GET", url, dict(headers), allow_redirects))
                return Response(
                    200,
                    "https://auth.openai.com/log-in/password",
                    '<button name="intent" value="passwordless_login_send_otp">Use code</button>',
                )

            def post(self, url, *, headers, allow_redirects):
                calls.append(("POST", url, dict(headers), allow_redirects))
                response = Response(200, url)
                response.json = lambda: {"page": {"type": "email_otp_verification"}}
                return response

        path = _start_passwordless_login(Session(), "https://auth.openai.com/log-in/password")

        self.assertEqual(path, "/email-verification")
        self.assertEqual(
            [(call[0], call[1]) for call in calls],
            [
                ("GET", "https://auth.openai.com/log-in/password"),
                ("POST", "https://auth.openai.com/api/accounts/passwordless/send-otp"),
            ],
        )
        self.assertEqual(calls[1][2]["referer"], "https://auth.openai.com/log-in/password")
        self.assertEqual(calls[1][2]["content-type"], "application/json")
        self.assertFalse(calls[1][3])

    def test_process_claim_uses_codex_protocol_flow(self):
        events = []

        class FakeSession:
            def __init__(self):
                self.session = SimpleNamespace(close=lambda: events.append("close"))

        class FakeProtocol:
            class AccountUnusableError(Exception):
                pass

            ProtocolSession = lambda **_kwargs: FakeSession()

            @staticmethod
            def network_preflight(_session):
                events.append("preflight")

            @staticmethod
            def bootstrap_authorize(_session, state, auth_url=None):
                events.append(("bootstrap", state, auth_url))

            @staticmethod
            def submit_email(_session, email):
                events.append(("email", email))

            @staticmethod
            def resend_email_otp(_session):
                events.append("resend")

            @staticmethod
            def submit_email_otp(_session, code):
                events.append(("otp", code))

            @staticmethod
            def select_workspace_and_get_callback(_session, state):
                events.append(("workspace", state))
                return "http://localhost:1455/auth/callback?code=callback-code&state=state-1"

            @staticmethod
            def extract_code(callback, state):
                events.append(("extract", callback, state))
                return "callback-code"

        class FakeAPI:
            def progress(self, task_id, stage):
                events.append(("progress", task_id, stage))

            def callback(self, task_id, callback):
                events.append(("callback", task_id, callback))
                return {"status": "succeeded"}

        claim = {
            "task_id": 7,
            "account_id": 42,
            "login_email": "mail@example.com",
            "otp_url": "http://127.0.0.1:12345/code",
            "auth_url": "https://auth.openai.com/authorize?state=state-1",
        }
        with patch("tools.openai_oauth_reauth_worker._wait_for_otp", return_value="482931"):
            process_claim(FakeAPI(), FakeProtocol, claim)

        names = [item[0] if isinstance(item, tuple) else item for item in events]
        self.assertLess(names.index("preflight"), names.index("bootstrap"))
        self.assertLess(names.index("bootstrap"), names.index("email"))
        self.assertLess(names.index("email"), names.index("otp"))
        self.assertLess(names.index("otp"), names.index("workspace"))
        self.assertLess(names.index("workspace"), names.index("callback"))
        self.assertNotIn("providers", names)
        self.assertNotIn("csrf", names)
        self.assertIn("close", names)

    def test_process_claim_uses_email_otp_resend_after_mailbox_timeout(self):
        events = []

        class FakeSession:
            def __init__(self):
                self.session = SimpleNamespace(close=lambda: None)

        class FakeProtocol:
            class AccountUnusableError(Exception):
                pass

            ProtocolSession = lambda **_kwargs: FakeSession()
            network_preflight = staticmethod(lambda _session: None)
            bootstrap_authorize = staticmethod(lambda _session, _state, auth_url=None: None)
            submit_email = staticmethod(lambda _session, _email: events.append("email"))
            resend_email_otp = staticmethod(lambda _session: events.append("resend"))
            submit_email_otp = staticmethod(lambda _session, _code: events.append("otp"))
            select_workspace_and_get_callback = staticmethod(
                lambda _session, _state: "http://localhost:1455/auth/callback?code=callback-code&state=state-1"
            )
            extract_code = staticmethod(lambda _callback, _state: "callback-code")

        class FakeAPI:
            progress = staticmethod(lambda _task_id, _stage: None)
            callback = staticmethod(lambda _task_id, _callback: {"status": "succeeded"})

        claim = {
            "task_id": 7,
            "account_id": 42,
            "login_email": "mail@example.com",
            "otp_url": "http://127.0.0.1:12345/code",
            "auth_url": "https://auth.openai.com/authorize?state=state-1",
        }
        with patch(
            "tools.openai_oauth_reauth_worker._wait_for_otp",
            side_effect=(WorkerError("mailbox timeout"), "482931"),
        ):
            process_claim(FakeAPI(), FakeProtocol, claim)

        self.assertEqual(events, ["email", "resend", "otp"])

    def test_process_claim_switches_login_password_to_email_otp(self):
        events = []

        class FakeSession:
            def __init__(self):
                self.session = SimpleNamespace(close=lambda: None, cookies=SimpleNamespace(jar=[]))

        class FakeProtocol:
            class AccountUnusableError(Exception):
                pass

            ProtocolSession = lambda **_kwargs: FakeSession()
            network_preflight = staticmethod(lambda _session: None)
            bootstrap_authorize = staticmethod(lambda _session, _state, auth_url=None: None)
            submit_email = staticmethod(
                lambda _session, _email: {
                    "continue_url": "https://auth.openai.com/log-in/password",
                    "page": {"type": "login_password"},
                }
            )
            start_passwordless_login = staticmethod(
                lambda _session, _url: events.append("passwordless")
            )
            resend_email_otp = staticmethod(lambda _session: events.append("resend"))
            submit_email_otp = staticmethod(lambda _session, _code: events.append("otp"))
            select_workspace_and_get_callback = staticmethod(
                lambda _session, _state: "http://localhost:1455/auth/callback?code=callback-code&state=state-1"
            )
            extract_code = staticmethod(lambda _callback, _state: "callback-code")

        class FakeAPI:
            progress = staticmethod(lambda _task_id, _stage: None)
            callback = staticmethod(lambda _task_id, _callback: {"status": "succeeded"})

        claim = {
            "task_id": 7,
            "account_id": 42,
            "login_email": "mail@example.com",
            "otp_url": "http://127.0.0.1:12345/code",
            "auth_url": "https://auth.openai.com/authorize?state=state-1",
        }
        with patch("tools.openai_oauth_reauth_worker._wait_for_otp", return_value="482931"):
            process_claim(FakeAPI(), FakeProtocol, claim)

        self.assertEqual(events, ["passwordless", "otp"])

    def test_extracts_structured_and_html_otp(self):
        self.assertEqual(extract_otp('{"verification_code":"123456"}'), "123456")
        self.assertEqual(extract_otp("<p>Your ChatGPT verification code is <b>654321</b></p>"), "654321")

    def test_rejects_stale_structured_otp(self):
        old = time.time() - 60
        body = json.dumps({"code": "123456", "received_at": old})
        self.assertEqual(extract_otp(body, after_ts=time.time()), "")

    def test_fetches_otp_without_worker_credentials(self):
        server = ThreadingHTTPServer(("127.0.0.1", 0), OTPHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            provider = OTPURLProvider(f"http://127.0.0.1:{server.server_port}/code")
            self.assertEqual(provider("mail@example.com"), "482931")
        finally:
            server.shutdown()
            server.server_close()

    def test_revalidates_mailbox_address_before_every_request(self):
        server = ThreadingHTTPServer(("127.0.0.1", 0), OTPHandler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        url = f"http://127.0.0.1:{server.server_port}/code"
        try:
            with patch(
                "tools.openai_oauth_reauth_worker.validate_otp_url",
                wraps=validate_otp_url,
            ) as validate:
                provider = OTPURLProvider(url)
                self.assertEqual(provider("mail@example.com"), "482931")
                self.assertEqual(validate.call_count, 2)
        finally:
            server.shutdown()
            server.server_close()

    def test_rejects_short_worker_token(self):
        previous = dict(os.environ)
        try:
            os.environ["SUB2API_BASE_URL"] = "http://127.0.0.1:8080"
            os.environ["OPENAI_REAUTH_WORKER_TOKEN"] = "short"
            os.environ["CODEX_PROTOCOL_ROOT"] = os.getcwd()
            with self.assertRaises(WorkerError):
                WorkerConfig.from_env()
        finally:
            os.environ.clear()
            os.environ.update(previous)

    def test_rejects_private_https_and_non_loopback_http_mailboxes(self):
        with self.assertRaises(WorkerError):
            validate_otp_url("https://127.0.0.1/code")
        with self.assertRaises(WorkerError):
            validate_otp_url("http://192.168.1.10/code")

    def test_rejects_plain_http_remote_control_api(self):
        previous = dict(os.environ)
        try:
            os.environ["SUB2API_BASE_URL"] = "http://example.com"
            os.environ["OPENAI_REAUTH_WORKER_TOKEN"] = "x" * 32
            os.environ["CODEX_PROTOCOL_ROOT"] = os.getcwd()
            with self.assertRaises(WorkerError):
                WorkerConfig.from_env()
        finally:
            os.environ.clear()
            os.environ.update(previous)

    def test_sanitizes_urls_oauth_params_and_otps(self):
        raw = (
            'failed https://mail.test/code?key=secret&state=abc for user@example.com '
            'with OTP 123456 and {"refresh_token":"hidden"} Bearer eyJhbGciOiJub25l.eyJzdWIiOiIxIn0.sig'
        )
        safe = sanitize_error(raw)
        self.assertNotIn("secret", safe)
        self.assertNotIn("abc", safe)
        self.assertNotIn("123456", safe)
        self.assertNotIn("https://", safe)
        self.assertNotIn("user@example.com", safe)
        self.assertNotIn("hidden", safe)
        self.assertNotIn("eyJ", safe)

    def test_wait_for_otp_skips_a_code_that_already_failed(self):
        codes = iter(("123456", "654321"))

        def provider(_email, after_ts=None):
            return next(codes)

        self.assertEqual(
            _wait_for_otp(
                provider,
                "mail@example.com",
                time.time(),
                timeout=1,
                interval=0.001,
                excluded_codes={"123456"},
            ),
            "654321",
        )

    def test_codex_callback_requires_exact_redirect_uri(self):
        self.assertTrue(
            _is_local_codex_callback(
                "http://localhost:1455/auth/callback?code=test-code&state=test-state"
            )
        )
        for callback in (
            "https://localhost:1455/auth/callback?code=test&state=test",
            "http://127.0.0.1:1455/auth/callback?code=test&state=test",
            "http://[::1]:1455/auth/callback?code=test&state=test",
            "http://localhost:1456/auth/callback?code=test&state=test",
            "http://localhost:1455/other?code=test&state=test",
            "http://localhost:1455/auth/callback/extra?code=test&state=test",
            "http://user@localhost:1455/auth/callback?code=test&state=test",
            "http://localhost:1455/auth/callback?code=test&state=test#fragment",
        ):
            with self.subTest(callback=callback):
                self.assertFalse(_is_local_codex_callback(callback))


if __name__ == "__main__":
    unittest.main()
