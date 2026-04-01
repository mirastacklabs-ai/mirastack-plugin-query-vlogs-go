package main

import "github.com/mirastacklabs-ai/mirastack-sdk-go"

func main() {
	mirastack.Serve(&QueryVLogsPlugin{})
}
