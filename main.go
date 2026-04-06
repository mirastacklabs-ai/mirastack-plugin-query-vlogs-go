package main

import "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"

func main() {
	mirastack.Serve(&QueryVLogsPlugin{})
}
