package service

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

var errInvalidBPSProxy = errors.New("invalid BPS login proxy")

// NormalizeBPSLoginProxy accepts username:password@hostname:port and an optional
// http, https, socks5, or socks5h scheme. A value without a scheme uses http.
func NormalizeBPSLoginProxy(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n") {
		return "", errInvalidBPSProxy
	}
	if strings.Contains(raw, "://") {
		return normalizeBPSProxyURL(raw)
	}
	at := strings.LastIndex(raw, "@")
	if at <= 0 || at == len(raw)-1 {
		return "", errInvalidBPSProxy
	}
	user, password, ok := splitBPSProxyUserinfo(raw[:at])
	if !ok {
		return "", errInvalidBPSProxy
	}
	host, port, ok := splitBPSProxyHost(raw[at+1:])
	if !ok {
		return "", errInvalidBPSProxy
	}
	u := &url.URL{Scheme: "http", Host: net.JoinHostPort(host, port), User: url.UserPassword(user, password)}
	return u.String(), nil
}

func normalizeBPSProxyURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return "", errInvalidBPSProxy
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return "", errInvalidBPSProxy
	}
	password, ok := parsed.User.Password()
	if !ok || parsed.User.Username() == "" || password == "" || parsed.Hostname() == "" || parsed.Port() == "" {
		return "", errInvalidBPSProxy
	}
	if _, _, ok = splitBPSProxyHost(net.JoinHostPort(parsed.Hostname(), parsed.Port())); !ok {
		return "", errInvalidBPSProxy
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errInvalidBPSProxy
	}
	return parsed.String(), nil
}

func splitBPSProxyUserinfo(userinfo string) (string, string, bool) {
	colon := strings.Index(userinfo, ":")
	if colon <= 0 || colon == len(userinfo)-1 {
		return "", "", false
	}
	return userinfo[:colon], userinfo[colon+1:], true
}

func splitBPSProxyHost(hostport string) (string, string, bool) {
	host, port, err := net.SplitHostPort(hostport)
	if err != nil || host == "" || strings.Contains(host, "@") {
		return "", "", false
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", "", false
	}
	return host, port, true
}
