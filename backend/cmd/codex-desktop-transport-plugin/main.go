package main

import pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"

func main() {
	pluginv1.Serve(newTransportService())
}
