package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/cmd/codex-desktop-transport-plugin/portrait"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "capture: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("capture", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:9443", "inspector listen address")
	serverName := fs.String("server-name", "chatgpt.com", "certificate / SNI name")
	certFile := fs.String("cert", "", "TLS certificate PEM (mkcert chatgpt.com)")
	keyFile := fs.String("key", "", "TLS key PEM")
	out := fs.String("out", "", "output directory for P2.json + manifest")
	cliVersion := fs.String("cli-version", portrait.RequiredHostCodexCLIVersion, "Desktop-bundled rustls engine version written into the manifest")
	desktopAppVersion := fs.String("desktop-app-version", portrait.RequiredDesktopAppVersion, "Codex Desktop / ChatGPT package version")
	minHTTP := fs.Int("min-http", 2, "reuse probe: wait for this many HTTP requests")
	timeout := fs.Duration("timeout", 3*time.Minute, "wait timeout for reuse probe")
	decodeDir := fs.String("decode", "", "validate an existing capture directory and exit")
	requireProbe := fs.Bool("require-reuse-probe", false, "with -decode, reject missing reuse probe")
	selftest := fs.Bool("selftest", false, "dial the inspector twice with utls HelloChrome (fake fixture only)")
	proxyListen := fs.String("connect-proxy", "", "optional HTTP CONNECT proxy (chatgpt.com:443 -> inspector). Use when hosts cannot be edited")
	spawnCodex := fs.String("spawn-codex", "", "optional path to Desktop-bundled codex.exe; started after listen with HTTPS_PROXY pointing at -connect-proxy")
	fromHello := fs.String("from-hello", "", "build P2.json from a captured ClientHello record instead of listening")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *decodeDir != "" {
		return decodeExisting(*decodeDir, *requireProbe)
	}
	if strings.TrimSpace(*fromHello) != "" {
		if strings.TrimSpace(*out) == "" {
			return fmt.Errorf("-out is required")
		}
		return portraitFromHelloFile(*fromHello, *out, *cliVersion, *desktopAppVersion)
	}
	if strings.TrimSpace(*out) == "" {
		return fmt.Errorf("-out is required")
	}
	if *cliVersion != portrait.RequiredHostCodexCLIVersion && !*selftest {
		return fmt.Errorf("-cli-version %q must equal Desktop engine pin %s (or use -selftest for fake fixtures)", *cliVersion, portrait.RequiredHostCodexCLIVersion)
	}

	pki, err := loadOrMakeCertificate(*certFile, *keyFile, *serverName)
	if err != nil {
		return err
	}
	if *certFile == "" {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			return err
		}
		if err := writeGeneratedPKI(pki, *out); err != nil {
			fmt.Fprintf(os.Stderr, "write generated cert: %v\n", err)
		} else {
			fmt.Fprintf(os.Stderr, "trust CA: certutil -user -addstore Root %s\n", filepath.Join(*out, "inspector-ca.pem"))
		}
	}
	cert := pki.Certificate

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	waitCtx, waitCancel := context.WithTimeout(ctx, *timeout)
	defer waitCancel()

	state, ln, err := runInspector(waitCtx, inspectorConfig{
		Listen:      *listen,
		ServerName:  *serverName,
		Certificate: cert,
		MinHTTP:     *minHTTP,
	})
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Fprintf(os.Stderr, "inspector listening on %s (server name %s)\n", ln.Addr().String(), *serverName)
	if strings.TrimSpace(*proxyListen) != "" {
		pln, err := listenConnectProxy(*proxyListen)
		if err != nil {
			return err
		}
		defer pln.Close()
		go serveConnectProxy(pln, ln.Addr().String(), *serverName)
		fmt.Fprintf(os.Stderr, "CONNECT proxy on %s (only %s:443 -> inspector)\n", pln.Addr().String(), *serverName)
	}

	if *selftest {
		if err := runSelftest(waitCtx, ln.Addr().String(), *serverName, cert); err != nil {
			return err
		}
	}
	if strings.TrimSpace(*spawnCodex) != "" {
		proxyAddr := strings.TrimSpace(*proxyListen)
		if proxyAddr == "" {
			return fmt.Errorf("-spawn-codex requires -connect-proxy")
		}
		cmd, err := startSpawnedCodex(waitCtx, *spawnCodex, proxyAddr)
		if err != nil {
			return err
		}
		defer func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		}()
	}

	if err := waitForRequests(waitCtx, state, *minHTTP); err != nil {
		dumpHellos(*out, state)
		return err
	}
	dumpHellos(*out, state)
	// Give in-flight H2 a moment to finish SETTINGS.
	time.Sleep(100 * time.Millisecond)

	version := *cliVersion
	if *selftest {
		version = "fake-0.0.0"
	}
	p, manifest, err := buildPortrait(state, version)
	if err != nil {
		return err
	}
	if !*selftest {
		manifest.CodexDesktopAppVersion = strings.TrimSpace(*desktopAppVersion)
	}
	if *selftest {
		p.ID = "fake-P2"
		if err := portrait.ValidateCIFixture(p); err != nil {
			return err
		}
	} else if err := portrait.ValidateReleasePortrait(p, manifest, filepath.Join(*out, "P2.json")); err != nil {
		return err
	}
	if err := writeCaptureDir(*out, p, manifest); err != nil {
		return err
	}
	if *selftest {
		renames := [][2]string{
			{"P2.json", "fake_P2.json"},
			{"capture.manifest.json", "fake_capture.manifest.json"},
			{"portraits.json", "fake_portraits.json"},
		}
		for _, pair := range renames {
			if err := os.Rename(filepath.Join(*out, pair[0]), filepath.Join(*out, pair[1])); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(os.Stderr, "wrote %s reuse=%s hellos=%d ja3=%s\n", *out, p.Connection.Reuse, len(state.hellos), p.TLS.JA3Hash)
	return nil
}

func portraitFromHelloFile(helloPath, out, cliVersion, desktopAppVersion string) error {
	if cliVersion != portrait.RequiredHostCodexCLIVersion {
		return fmt.Errorf("-cli-version %q must equal Desktop engine pin %s", cliVersion, portrait.RequiredHostCodexCLIVersion)
	}
	raw, err := os.ReadFile(helloPath)
	if err != nil {
		return err
	}
	if !looksLikeDesktopHello(raw) {
		return fmt.Errorf("%s is not a Desktop engine ClientHello (rustls TLS1.3 or Windows Schannel TLS1.2)", helloPath)
	}
	tlsCap, err := portrait.AssertionsFromRaw(raw)
	if err != nil {
		return err
	}
	p := &portrait.Portrait{
		ID:            "P2",
		SourceCapture: "capture.manifest.json",
		TLS:           tlsCap,
		Connection: portrait.ConnCapture{
			Reuse:                     portrait.ReusePerRequest,
			ObservedNewConnPerRequest: true,
			SecondRequestClientHello:  true,
		},
	}
	manifest := &portrait.CaptureManifest{
		SchemaVersion:          portrait.SchemaVersion,
		CodexCLIVersion:        cliVersion,
		CodexDesktopAppVersion: strings.TrimSpace(desktopAppVersion),
		CapturedAt:             time.Now().UTC().Format(time.RFC3339),
		OS:                     runtime.GOOS,
		Arch:                   runtime.GOARCH,
		Method:                 portrait.MethodInspector,
		Inspector:              inspectorName,
		Notes:                  "Desktop-bundled Windows codex.exe " + cliVersion + " via CONNECT proxy; Schannel TLS 1.2 ClientHello (no GREASE, no ALPN); no production load",
	}
	if err := portrait.ValidateReleasePortrait(p, manifest, filepath.Join(out, "P2.json")); err != nil {
		return err
	}
	if _, err := portrait.ClientHelloSpecForDial(raw, "chatgpt.com"); err != nil {
		return fmt.Errorf("utls cannot replay this hello: %w", err)
	}
	if err := writeCaptureDir(out, p, manifest); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s reuse=%s ja3=%s ja4=%s from %s\n", out, p.Connection.Reuse, p.TLS.JA3Hash, p.TLS.JA4, helloPath)
	return nil
}

func decodeExisting(dir string, requireProbe bool) error {
	p, err := portrait.LoadPortraitFile(filepath.Join(dir, "P2.json"))
	if err != nil {
		p, err = portrait.LoadPortraitFile(filepath.Join(dir, "fake_P2.json"))
		if err != nil {
			return err
		}
	}
	manifest, _ := portrait.LoadManifestFile(filepath.Join(dir, "capture.manifest.json"))
	if requireProbe {
		if err := portrait.ValidateReleasePortrait(p, manifest, filepath.Join(dir, "P2.json")); err != nil {
			return err
		}
	} else if err := portrait.ValidateCIFixture(p); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(p.Connection, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(raw))
	return nil
}

func runSelftest(ctx context.Context, addr, serverName string, cert tls.Certificate) error {
	leaf, err := x509CertFromTLS(cert)
	if err != nil {
		return err
	}
	roots := leafPool(leaf)
	dial := func() error {
		tcp, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			return err
		}
		uconn := utls.UClient(tcp, &utls.Config{
			ServerName:         serverName,
			RootCAs:            roots,
			NextProtos:         []string{"h2", "http/1.1"},
			InsecureSkipVerify: roots == nil,
		}, utls.HelloChrome_Auto)
		if err := uconn.HandshakeContext(ctx); err != nil {
			_ = tcp.Close()
			return err
		}
		h2t, err := http2.ConfigureTransports(&http.Transport{})
		if err != nil {
			_ = uconn.Close()
			return err
		}
		h2c, err := h2t.NewClientConn(uconn)
		if err != nil {
			_ = uconn.Close()
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+serverName+"/capture", nil)
		if err != nil {
			_ = h2c.Close()
			return err
		}
		resp, err := h2c.RoundTrip(req)
		if err != nil {
			_ = h2c.Close()
			return err
		}
		_ = resp.Body.Close()
		_ = h2c.Close()
		return nil
	}
	if err := dial(); err != nil {
		return fmt.Errorf("selftest request 1: %w", err)
	}
	if err := dial(); err != nil {
		return fmt.Errorf("selftest request 2: %w", err)
	}
	return nil
}

func startSpawnedCodex(ctx context.Context, exe, proxyListen string) (*exec.Cmd, error) {
	proxyURL := "http://" + proxyListen
	cmd := exec.CommandContext(ctx, exe, "exec", "--ephemeral", "--skip-git-repo-check", "--ignore-user-config", "Say hi")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	var env []string
	for _, e := range os.Environ() {
		upper := strings.ToUpper(e)
		if strings.HasPrefix(upper, "OPENAI_BASE_URL=") ||
			strings.HasPrefix(upper, "HTTPS_PROXY=") ||
			strings.HasPrefix(upper, "HTTP_PROXY=") ||
			strings.HasPrefix(upper, "ALL_PROXY=") ||
			strings.HasPrefix(upper, "NO_PROXY=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env,
		"HTTPS_PROXY="+proxyURL,
		"HTTP_PROXY="+proxyURL,
		"ALL_PROXY="+proxyURL,
		"NO_PROXY=",
		"no_proxy=",
	)
	fmt.Fprintf(os.Stderr, "spawned %s via %s\n", exe, proxyURL)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func x509CertFromTLS(cert tls.Certificate) (*x509.Certificate, error) {
	if len(cert.Certificate) == 0 {
		return nil, fmt.Errorf("empty tls certificate")
	}
	return x509.ParseCertificate(cert.Certificate[0])
}

func leafPool(leaf *x509.Certificate) *x509.CertPool {
	if leaf == nil {
		return nil
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return pool
}
