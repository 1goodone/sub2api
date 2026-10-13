FROM mcr.microsoft.com/playwright/python:v1.63.0-noble@sha256:96b39581c89131729a7ecb8d532314af54c7f9bcc7a61fe15c7a9e77602acf59

WORKDIR /opt/sub2api/prism-adapter

COPY requirements.txt ./

ARG PIP_FIND_LINKS
ARG PIP_NO_INDEX=0

RUN python -m pip install --no-cache-dir --only-binary=:all: -r requirements.txt \
    && chromium="$(python -c 'from playwright.sync_api import sync_playwright; p = sync_playwright().start(); print(p.chromium.executable_path); p.stop()')" \
    && test -x "$chromium" \
    && ln -s "$chromium" /usr/local/bin/prism-chromium \
    && chromium_dir="$(dirname "$chromium")" \
    && sandbox="$(find "$chromium_dir" -maxdepth 1 -type f \( -name chrome_sandbox -o -name chrome-sandbox \) -print -quit)" \
    && test -n "$sandbox" \
    && chmod 755 "$chromium_dir" \
    && install -o root -g root -m 4755 "$sandbox" "$chromium_dir/chrome-sandbox" \
    && ln -s "$chromium_dir/chrome-sandbox" /usr/local/sbin/chrome-devel-sandbox \
    && groupadd --gid 10001 prism \
    && useradd --uid 10001 --gid 10001 --home-dir /var/lib/sub2api-prism --create-home --shell /usr/sbin/nologin prism \
    && chmod 700 /var/lib/sub2api-prism

COPY server.py browser_gate.py image_inputs.py model_selection.py multiplex_browser.py multiplex_runtime.py response_events.py tool_bridge.py tool_state.py tool_validation.py ./

ENV HOME=/var/lib/sub2api-prism \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PRISM_ADAPTER_CHROME=/usr/local/bin/prism-chromium \
    CHROME_DEVEL_SANDBOX=/usr/local/sbin/chrome-devel-sandbox \
    PRISM_ADAPTER_STATE_DIR=/var/lib/sub2api-prism \
    PRISM_ADAPTER_PORT=8319

USER prism

HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=4 \
    CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8319/health', timeout=3).read()"]

CMD ["python", "server.py"]
